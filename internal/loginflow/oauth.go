package loginflow

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"workbuddy2api/internal/auth"
)

const (
	RealmCN     = "cn"
	RealmGlobal = "global"

	BaseCN       = "https://copilot.tencent.com"
	BaseGlobal   = "https://www.workbuddy.ai"
	OriginCN     = "https://www.codebuddy.cn"
	OriginGlobal = "https://www.workbuddy.ai"

	ClientUserAgent = "CLI/2.63.2 CodeBuddy/2.63.2"
)

// StatePath is the default location shared by the low-level url and poll
// subcommands.
func StatePath() string {
	return filepath.Join(os.TempDir(), "wb2api-login-state.json")
}

// RealmEndpoints maps a realm to its OAuth base and browser Origin.
func RealmEndpoints(realm string) (base, origin string) {
	if realm == RealmGlobal {
		return BaseGlobal, OriginGlobal
	}
	return BaseCN, OriginCN
}

// CommonHeaders applies the headers used by both OAuth calls and the CN
// daily-checkin endpoint.
func CommonHeaders(origin string) func(*http.Request) {
	return func(req *http.Request) {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("Origin", origin)
		req.Header.Set("Referer", origin+"/")
		req.Header.Set("User-Agent", ClientUserAgent)
	}
}

// DoJSON wraps WorkBuddy's {code,msg,data} envelope.
func DoJSON(
	client *http.Client,
	method, fullURL string,
	headers func(*http.Request),
	body io.Reader,
) (json.RawMessage, int, error) {
	return doJSONContext(context.Background(), client, method, fullURL, headers, body)
}

func doJSONContext(
	ctx context.Context,
	client *http.Client,
	method, fullURL string,
	headers func(*http.Request),
	body io.Reader,
) (json.RawMessage, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return nil, 0, err
	}
	if headers != nil {
		headers(req)
	} else {
		CommonHeaders(OriginCN)(req)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, resp.StatusCode, fmt.Errorf("http_error: upstream %d", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return nil, resp.StatusCode, fmt.Errorf("http_error: upstream redirect %d", resp.StatusCode)
	}
	var env struct {
		Code int             `json:"code"`
		Msg  string          `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("parse failed: %w", err)
	}
	if env.Code != 0 {
		return nil, resp.StatusCode, fmt.Errorf("code=%d msg=%s", env.Code, env.Msg)
	}
	return env.Data, resp.StatusCode, nil
}

// OAuthClient owns one realm/state pairing.
type OAuthClient struct {
	HTTPClient *http.Client
	BaseURL    string
	Origin     string
	Realm      string
	StatePath  string
}

// NewOAuthClient creates a client from a realm and optional test overrides.
func NewOAuthClient(
	httpClient *http.Client,
	realm, baseOverride, originOverride, statePath string,
) *OAuthClient {
	base, origin := RealmEndpoints(realm)
	if strings.TrimSpace(baseOverride) != "" {
		base = baseOverride
	}
	if strings.TrimSpace(originOverride) != "" {
		origin = originOverride
	}
	if strings.TrimSpace(statePath) == "" {
		statePath = StatePath()
	}
	return &OAuthClient{
		HTTPClient: httpClient,
		BaseURL:    strings.TrimRight(base, "/"),
		Origin:     origin,
		Realm:      realm,
		StatePath:  statePath,
	}
}

type loginState struct {
	State string `json:"state"`
	Realm string `json:"realm,omitempty"`
}

// AuthURL requests one device authorization URL and persists its state with
// mode 0600 for the later poll call.
func (c *OAuthClient) AuthURL(ctx context.Context) (string, error) {
	data, _, err := doJSONContext(
		ctx,
		c.HTTPClient,
		http.MethodPost,
		c.BaseURL+"/v2/plugin/auth/state?platform=CLI",
		CommonHeaders(c.Origin),
		bytes.NewReader([]byte("{}")),
	)
	if err != nil {
		return "", fmt.Errorf("auth state failed: %w", err)
	}
	var state struct {
		State   string `json:"state"`
		AuthURL string `json:"authUrl"`
	}
	if err := json.Unmarshal(data, &state); err != nil || state.State == "" || state.AuthURL == "" {
		return "", errors.New("auth state: missing state or authUrl")
	}
	raw, err := json.Marshal(loginState{State: state.State, Realm: c.Realm})
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(c.StatePath, raw, 0o600); err != nil {
		return "", fmt.Errorf("write state: %w", err)
	}
	if err := os.Chmod(c.StatePath, 0o600); err != nil {
		return "", fmt.Errorf("chmod state: %w", err)
	}
	return state.AuthURL, nil
}

// Token is the OAuth token bundle returned by the token endpoint.
type Token struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresIn    int64  `json:"expiresIn"`
	Domain       string `json:"domain"`
}

// Account is the identity returned after a completed OAuth login.
type Account struct {
	UID          string `json:"uid"`
	EnterpriseID string `json:"enterpriseId"`
	Nickname     string `json:"nickname"`
}

// Result is one completed OAuth poll.
type Result struct {
	Token   Token
	Account Account
	Realm   string
}

// Poll reads and validates the state file, polls the token endpoint once, and
// fetches the account identity. The state file is removed after a successful
// token response, matching the existing CLI contract.
func (c *OAuthClient) Poll(ctx context.Context) (Result, error) {
	raw, err := os.ReadFile(c.StatePath)
	if err != nil {
		return Result{}, fmt.Errorf("read state: %w (先跑 login url)", err)
	}
	var state loginState
	if err := json.Unmarshal(raw, &state); err != nil {
		return Result{}, fmt.Errorf("parse state: %w", err)
	}
	if err := ValidateRealmMatch(state.Realm, c.Realm); err != nil {
		return Result{}, err
	}
	headers := CommonHeaders(c.Origin)
	tokenRaw, status, err := doJSONContext(
		ctx,
		c.HTTPClient,
		http.MethodGet,
		c.BaseURL+"/v2/plugin/auth/token?state="+state.State,
		headers,
		nil,
	)
	if err != nil {
		if status == 0 || status >= 500 {
			return Result{}, fmt.Errorf("token endpoint error: %w", err)
		}
		return Result{}, errors.New("登录未完成（waiting for login）。请确认已在浏览器完成登录再按 y")
	}
	var token Token
	if err := json.Unmarshal(tokenRaw, &token); err != nil || token.AccessToken == "" {
		return Result{}, errors.New("登录未完成（waiting for login）。请确认已在浏览器完成登录再按 y")
	}

	var account Account
	accountHeaders := func(req *http.Request) {
		headers(req)
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	}
	accountRaw, _, accountErr := doJSONContext(
		ctx,
		c.HTTPClient,
		http.MethodGet,
		c.BaseURL+"/v2/plugin/login/account?state="+state.State,
		accountHeaders,
		nil,
	)
	if accountErr == nil {
		_ = json.Unmarshal(accountRaw, &account)
	}
	_ = os.Remove(c.StatePath)
	return Result{
		Token:   token,
		Account: account,
		Realm:   auth.ResolveRealm(c.Realm, token.Domain),
	}, nil
}

// ResolveRealmInput normalizes the interactive login-version prompt.
func ResolveRealmInput(input string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "", "1", RealmCN:
		return RealmCN, true
	case "2", RealmGlobal:
		return RealmGlobal, true
	}
	return "", false
}

// PromptRealm reads one selection line and falls back to CN on invalid or EOF
// input, matching the shell prompt semantics.
func PromptRealm(in io.Reader, out io.Writer) string {
	fmt.Fprintln(out, "选择登录版本: 1) 国内版(cn) 2) 国际版(global) [默认 1/cn]: ")
	reader, ok := in.(*bufio.Reader)
	if !ok {
		reader = bufio.NewReader(in)
	}
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return RealmCN
	}
	if realm, ok := ResolveRealmInput(line); ok {
		return realm
	}
	fmt.Fprintln(out, "无效选择，默认国内版 cn")
	return RealmCN
}

// ValidateRealmMatch rejects a state file produced by a different realm. State
// files from older versions without a realm key remain accepted.
func ValidateRealmMatch(stateRealm, cliRealm string) error {
	if stateRealm != "" && stateRealm != cliRealm {
		return fmt.Errorf(
			"realm mismatch: state file realm=%q, command --realm=%q（url 与 poll 需同一 realm）",
			stateRealm,
			cliRealm,
		)
	}
	return nil
}
