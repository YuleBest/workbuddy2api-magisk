package loginflow

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"
)

// CommandOutput captures one external process invocation.
type CommandOutput struct {
	Stdout []byte
	Stderr []byte
}

// CommandFunc executes a system utility without a shell.
type CommandFunc func(ctx context.Context, name string, args []string, stdin io.Reader) (CommandOutput, error)

// ExecCommand is the production command runner.
func ExecCommand(
	ctx context.Context,
	name string,
	args []string,
	stdin io.Reader,
) (CommandOutput, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return CommandOutput{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

// Runner owns the complete high-level login flow.
type Runner struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer

	StdinTTY      bool
	Realm         string
	RealmExplicit bool

	AuthDir        string
	OAuthStatePath string

	OAuthBaseURL  string
	OAuthOrigin   string
	CNBaseURL     string
	GlobalBaseURL string
	HTTPClient    *http.Client
	RunCommand    CommandFunc
	LookPath      func(string) (string, error)
	Now           func() time.Time
	Getenv        func(string) string
	LookupUID     func(string) (int, error)
	LookupName    func(int) (string, error)
	FileOwner     func(string) (int, error)
}

// NewRunner creates the production high-level login runner.
func NewRunner() *Runner {
	jar, _ := cookiejar.New(nil)
	return &Runner{
		In:             os.Stdin,
		Out:            os.Stdout,
		Err:            os.Stderr,
		StdinTTY:       isCharacterDevice(os.Stdin),
		Realm:          RealmCN,
		AuthDir:        "auths",
		OAuthStatePath: StatePath(),
		CNBaseURL:      "https://www.codebuddy.cn",
		GlobalBaseURL:  "https://www.workbuddy.ai",
		HTTPClient:     &http.Client{Timeout: 30 * time.Second, Jar: jar},
		RunCommand:     ExecCommand,
		LookPath:       exec.LookPath,
		Now:            time.Now,
		Getenv:         os.Getenv,
		LookupUID:      lookupUID,
		LookupName:     lookupName,
		FileOwner:      fileOwnerUID,
	}
}

func isCharacterDevice(file *os.File) bool {
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func lookupUID(name string) (int, error) {
	account, err := user.Lookup(name)
	if err != nil {
		return 0, err
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return 0, err
	}
	return uid, nil
}

func lookupName(uid int) (string, error) {
	account, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return "", err
	}
	return account.Username, nil
}

func (base *Runner) withDefaults() *Runner {
	if base == nil {
		return NewRunner()
	}
	r := *base
	defaults := NewRunner()
	if r.In == nil {
		r.In = defaults.In
	}
	if r.Out == nil {
		r.Out = defaults.Out
	}
	if r.Err == nil {
		r.Err = defaults.Err
	}
	if strings.TrimSpace(r.Realm) == "" {
		r.Realm = RealmCN
	}
	if r.AuthDir == "" {
		r.AuthDir = defaults.AuthDir
	}
	if r.OAuthStatePath == "" {
		r.OAuthStatePath = defaults.OAuthStatePath
	}
	if r.CNBaseURL == "" {
		r.CNBaseURL = defaults.CNBaseURL
	}
	if r.GlobalBaseURL == "" {
		r.GlobalBaseURL = defaults.GlobalBaseURL
	}
	if r.HTTPClient == nil {
		r.HTTPClient = defaults.HTTPClient
	}
	if r.RunCommand == nil {
		r.RunCommand = defaults.RunCommand
	}
	if r.LookPath == nil {
		r.LookPath = defaults.LookPath
	}
	if r.Now == nil {
		r.Now = defaults.Now
	}
	if r.Getenv == nil {
		r.Getenv = defaults.Getenv
	}
	if r.LookupUID == nil {
		r.LookupUID = defaults.LookupUID
	}
	if r.LookupName == nil {
		r.LookupName = defaults.LookupName
	}
	if r.FileOwner == nil {
		r.FileOwner = defaults.FileOwner
	}
	return &r
}

// Run executes the complete login flow and returns the process exit code.
func (base *Runner) Run(ctx context.Context) int {
	r := base.withDefaults()
	realm := strings.ToLower(strings.TrimSpace(r.Realm))
	if realm != RealmCN && realm != RealmGlobal {
		fmt.Fprintf(r.Err, "login: invalid realm %q (want cn|global)\n", r.Realm)
		return 1
	}
	expectedUID := r.expectedOwnerUID(ctx)
	if err := PrepareAuthDir(r.AuthDir); err != nil {
		fmt.Fprintf(r.Err, "❌ 无法写入 %s（权限不足或属主不匹配）\n", r.AuthDir)
		fmt.Fprintf(r.Err, "    原因：%v\n", err)
		for _, line := range PermissionAdvice(r.AuthDir, expectedUID, r.LookupName) {
			fmt.Fprintln(r.Err, line)
		}
		fmt.Fprintln(r.Err)
		return 1
	}
	reader := bufio.NewReader(r.In)
	if !r.RealmExplicit {
		if r.StdinTTY {
			realm = PromptRealm(reader, r.Err)
		} else {
			realm = RealmCN
			fmt.Fprintln(r.Err, "（非交互 stdin，默认国内版 cn；可用 --realm=global 指定国际版）")
		}
	}

	fmt.Fprintln(r.Out, "============================================================")
	fmt.Fprintln(r.Out, "  WorkBuddy OAuth 登录")
	fmt.Fprintln(r.Out, "============================================================")
	fmt.Fprintln(r.Out)

	oauth := NewOAuthClient(
		r.HTTPClient,
		realm,
		r.OAuthBaseURL,
		r.OAuthOrigin,
		r.OAuthStatePath,
	)
	authURL, err := oauth.AuthURL(ctx)
	if err != nil {
		fmt.Fprintf(r.Err, "login: %v\n", err)
		return 1
	}
	fmt.Fprintln(r.Out, "请在浏览器中打开以下链接完成登录：")
	fmt.Fprintln(r.Out)
	fmt.Fprintf(r.Out, "  %s\n", authURL)
	fmt.Fprintln(r.Out)
	r.copyAuthURL(ctx, authURL)
	fmt.Fprintln(r.Out)
	fmt.Fprint(r.Out, "完成登录后按 y 继续: ")
	answer, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		fmt.Fprintln(r.Out, "已取消")
		return 1
	}
	fmt.Fprintln(r.Out)
	fmt.Fprintln(r.Out, "正在获取 token...")
	result, err := oauth.Poll(ctx)
	if err != nil {
		fmt.Fprintln(r.Out)
		fmt.Fprintln(r.Out, "获取 token 失败。可能原因：")
		fmt.Fprintln(r.Out, "  - 登录还没完成就按了 y（重新运行 ./login 再试）")
		fmt.Fprintln(r.Out, "  - 登录页报错（把报错截图发出来排查）")
		fmt.Fprintf(r.Err, "login: %v\n", err)
		return 1
	}
	if result.Account.UID == "" {
		fmt.Fprintln(r.Err, "无法获取 uid，请检查 token 是否有效")
		return 1
	}
	expiresAt := r.Now().Add(time.Duration(result.Token.ExpiresIn) * time.Second).Unix()
	if realm == RealmGlobal {
		fmt.Fprintln(r.Out, "签到: global realm 跳过（国际版签到端点未实测）")
	} else {
		r.runCheckin(ctx, result.Token.AccessToken, result.Account, result.Token.Domain)
	}

	authFile := AuthFilePath(r.AuthDir, result.Account.UID)
	action := "新增"
	if _, err := os.Stat(authFile); err == nil {
		action = "覆盖"
		fmt.Fprintf(r.Out, "账号已存在（uid=%s），将覆盖更新凭证\n", result.Account.UID)
	} else {
		fmt.Fprintf(r.Out, "新账号（uid=%s），新增 auth 文件\n", result.Account.UID)
	}
	creds := Credentials{
		AccessToken:  result.Token.AccessToken,
		RefreshToken: result.Token.RefreshToken,
		ExpiresAt:    expiresAt,
		Domain:       result.Token.Domain,
		Realm:        realm,
		UID:          result.Account.UID,
		EnterpriseID: result.Account.EnterpriseID,
		Nickname:     result.Account.Nickname,
	}
	authFile, err = WriteAuthFile(r.AuthDir, creds)
	if err != nil {
		fmt.Fprintf(r.Err, "❌ 写入失败：%v\n", err)
		for _, line := range PermissionAdvice(r.AuthDir, expectedUID, r.LookupName) {
			fmt.Fprintln(r.Err, line)
		}
		return 1
	}
	fmt.Fprintf(r.Out, "已保存（%s）: %s\n", action, authFile)
	r.warnOwnerMismatch(authFile, expectedUID)

	if realm == RealmGlobal {
		r.runGlobal(ctx, result.Token.AccessToken, result.Account.UID, reader)
	}

	fmt.Fprintln(r.Out)
	fmt.Fprintln(r.Out, "============================================================")
	fmt.Fprintln(r.Out, "  登录完成！")
	fmt.Fprintf(r.Out, "  UID: %s\n", result.Account.UID)
	fmt.Fprintf(r.Out, "  Realm: %s\n", realm)
	nickname := result.Account.Nickname
	if nickname == "" {
		nickname = "（未获取到）"
	}
	fmt.Fprintf(r.Out, "  Nickname: %s\n", nickname)
	fmt.Fprintf(r.Out, "  Token: %s...\n", tokenPrefix(result.Token.AccessToken, 30))
	fmt.Fprintf(r.Out, "  有效期: %s\n", time.Unix(expiresAt, 0).Format("2006-01-02 15:04"))
	fmt.Fprintln(r.Out, "============================================================")
	return 0
}

func tokenPrefix(token string, max int) string {
	runes := []rune(token)
	if len(runes) <= max {
		return token
	}
	return string(runes[:max])
}

func (r *Runner) expectedOwnerUID(ctx context.Context) int {
	if raw := strings.TrimSpace(r.Getenv("WB2A_EXPECT_UID")); raw != "" {
		if uid, err := strconv.Atoi(raw); err == nil && uid >= 0 {
			return uid
		}
	}
	commandCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, _ := r.RunCommand(
		commandCtx,
		"systemctl",
		[]string{"show", "-p", "User", "--value", "wbapi-gateway"},
		nil,
	)
	name := strings.TrimSpace(string(out.Stdout))
	if name != "" && name != "root" {
		if uid, err := r.LookupUID(name); err == nil {
			return uid
		}
	}
	if uid := os.Getuid(); uid >= 0 {
		return uid
	}
	return -1
}

func (r *Runner) warnOwnerMismatch(path string, expectedUID int) {
	actualUID, err := r.FileOwner(path)
	if err != nil {
		return
	}
	if actualUID == expectedUID {
		return
	}
	fmt.Fprintf(r.Out, "\n⚠️  权限警告：%s 属主 uid=%d，网关进程（uid=%d）可能读不到\n", path, actualUID, expectedUID)
	for _, line := range PermissionAdvice(r.AuthDir, expectedUID, r.LookupName) {
		fmt.Fprintln(r.Out, line)
	}
	fmt.Fprintln(r.Out)
}

func (r *Runner) copyAuthURL(ctx context.Context, authURL string) {
	if path, err := r.LookPath("xclip"); err == nil {
		_, runErr := r.RunCommand(
			ctx,
			path,
			[]string{"-selection", "clipboard"},
			strings.NewReader(authURL),
		)
		if runErr == nil {
			fmt.Fprintln(r.Out, "(已复制到剪贴板)")
		}
		return
	}
	if path, err := r.LookPath("xsel"); err == nil {
		_, runErr := r.RunCommand(
			ctx,
			path,
			[]string{"--clipboard"},
			strings.NewReader(authURL),
		)
		if runErr == nil {
			fmt.Fprintln(r.Out, "(已复制到剪贴板)")
		}
	}
}

func decodeMap(raw []byte) (map[string]any, error) {
	var body map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&body); err != nil {
		return nil, err
	}
	return body, nil
}

func numericCode(value any) (int, bool) {
	switch code := value.(type) {
	case json.Number:
		number, err := code.Int64()
		return int(number), err == nil
	case float64:
		return int(code), true
	case int:
		return code, true
	}
	return 0, false
}

func responseMessage(body map[string]any, fallback string) string {
	if msg, ok := body["msg"].(string); ok && msg != "" {
		return msg
	}
	if code, ok := numericCode(body["code"]); ok {
		return fmt.Sprintf("code=%d", code)
	}
	return fallback
}

func shortJSON(value any, max int) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	text := string(raw)
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max])
}
