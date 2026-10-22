package loginflow

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"workbuddy2api/internal/auth"
)

type fakeLoginServer struct {
	*httptest.Server
	mu             sync.Mutex
	paths          []string
	checkinHeaders http.Header
	globalRegister string
	trialBody      string
}

func newFakeLoginServer(t *testing.T) *fakeLoginServer {
	t.Helper()
	fake := &fakeLoginServer{
		globalRegister: `{"code":200,"msg":"register success"}`,
		trialBody:      `{"code":0,"msg":"ok"}`,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/plugin/auth/state", func(w http.ResponseWriter, r *http.Request) {
		fake.record(r.URL.Path)
		_, _ = io.WriteString(w, `{"code":0,"data":{"state":"s1","authUrl":"https://example.test/auth"}}`)
	})
	mux.HandleFunc("/v2/plugin/auth/token", func(w http.ResponseWriter, r *http.Request) {
		fake.record(r.URL.Path)
		_, _ = io.WriteString(w, `{"code":0,"data":{`+
			`"accessToken":"access-token-value","refreshToken":"refresh-token-value",`+
			`"expiresIn":3600,"domain":"www.codebuddy.cn"}}`)
	})
	mux.HandleFunc("/v2/plugin/login/account", func(w http.ResponseWriter, r *http.Request) {
		fake.record(r.URL.Path)
		_, _ = io.WriteString(w, `{"code":0,"data":{"uid":"u1","enterpriseId":"ent1","nickname":"Nick"}}`)
	})
	mux.HandleFunc("/v2/billing/meter/daily-checkin", func(w http.ResponseWriter, r *http.Request) {
		fake.record(r.URL.Path)
		fake.mu.Lock()
		fake.checkinHeaders = r.Header.Clone()
		fake.mu.Unlock()
		_, _ = io.WriteString(w, `{"code":0,"data":{"reward":1}}`)
	})
	mux.HandleFunc("/auth/realms/copilot/overseas/user/register", func(w http.ResponseWriter, r *http.Request) {
		fake.record(r.URL.Path)
		_, _ = io.WriteString(w, fake.globalRegister)
	})
	mux.HandleFunc("/billing/ide/trial", func(w http.ResponseWriter, r *http.Request) {
		fake.record(r.URL.Path)
		_, _ = io.WriteString(w, fake.trialBody)
	})
	fake.Server = httptest.NewServer(mux)
	t.Cleanup(fake.Close)
	return fake
}

func (f *fakeLoginServer) record(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths = append(f.paths, path)
}

func (f *fakeLoginServer) hasPath(path string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, got := range f.paths {
		if got == path {
			return true
		}
	}
	return false
}

func baseRunner(t *testing.T, server *fakeLoginServer, realm string) *Runner {
	t.Helper()
	return &Runner{
		In:             strings.NewReader("y\n"),
		Out:            &strings.Builder{},
		Err:            &strings.Builder{},
		StdinTTY:       false,
		Realm:          realm,
		RealmExplicit:  true,
		AuthDir:        filepath.Join(t.TempDir(), "auths"),
		OAuthStatePath: filepath.Join(t.TempDir(), "state.json"),
		OAuthBaseURL:   server.URL,
		OAuthOrigin:    server.URL,
		CNBaseURL:      server.URL,
		GlobalBaseURL:  server.URL,
		HTTPClient:     server.Client(),
		LookPath:       func(string) (string, error) { return "", errors.New("not found") },
		Now:            func() time.Time { return time.Unix(1_700_000_000, 0) },
		Getenv:         func(string) string { return "123" },
		LookupUID:      func(string) (int, error) { return 0, errors.New("not found") },
		LookupName:     func(int) (string, error) { return "tester", nil },
		FileOwner:      func(string) (int, error) { return 123, nil },
		RunCommand: func(context.Context, string, []string, io.Reader) (CommandOutput, error) {
			return CommandOutput{}, errors.New("not found")
		},
	}
}

func TestWriteAuthFileAtomicOverwriteAndMode(t *testing.T) {
	dir := t.TempDir()
	path := AuthFilePath(dir, "u1")
	if err := os.WriteFile(path, []byte(`old`), 0o644); err != nil {
		t.Fatal(err)
	}
	creds := Credentials{
		AccessToken:  "at",
		RefreshToken: "rt",
		ExpiresAt:    123,
		Domain:       "www.codebuddy.cn",
		Realm:        RealmCN,
		UID:          "u1",
		EnterpriseID: "ent",
		Nickname:     "Nick",
	}
	gotPath, err := WriteAuthFile(dir, creds)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != path {
		t.Errorf("path=%q want %q", gotPath, path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode=%o want 600", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(string(raw), "\n") {
		t.Error("auth file should not have trailing newline")
	}
	parsed, err := auth.Parse(raw)
	if err != nil {
		t.Fatalf("plugin auth parser rejected file: %v\n%s", err, raw)
	}
	if parsed.UID != "u1" || parsed.AccessToken != "at" || parsed.RealmStored() != RealmCN {
		t.Errorf("parsed=%+v", parsed)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".workbuddy-auth-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Errorf("temporary files left: %v", temps)
	}
}

func TestPrepareAuthDirReportsPermissionError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory mode checks")
	}
	dir := t.TempDir()
	authDir := filepath.Join(dir, "auths")
	if err := os.Mkdir(authDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(authDir, 0o700) })
	if err := PrepareAuthDir(authDir); err == nil {
		t.Fatal("PrepareAuthDir succeeded on read-only directory")
	}
}

func TestRunRejectsUnwritableAuthPathBeforeOAuth(t *testing.T) {
	fake := newFakeLoginServer(t)
	dir := t.TempDir()
	authPath := filepath.Join(dir, "auths")
	if err := os.WriteFile(authPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := baseRunner(t, fake, RealmCN)
	runner.AuthDir = authPath
	code := runner.Run(context.Background())
	if code != 1 {
		t.Fatalf("exit=%d want 1", code)
	}
	if fake.hasPath("/v2/plugin/auth/state") {
		t.Error("OAuth started despite unwritable auth path")
	}
	errText := runner.Err.(*strings.Builder).String()
	if !strings.Contains(errText, "无法写入") || !strings.Contains(errText, "chown -R") {
		t.Errorf("permission error missing actionable advice:\n%s", errText)
	}
}

func TestExpectedOwnerPriority(t *testing.T) {
	t.Run("environment wins", func(t *testing.T) {
		runner := &Runner{
			Getenv: func(string) string { return "321" },
			RunCommand: func(context.Context, string, []string, io.Reader) (CommandOutput, error) {
				t.Fatal("systemctl should not run")
				return CommandOutput{}, nil
			},
		}
		if got := runner.expectedOwnerUID(context.Background()); got != 321 {
			t.Errorf("uid=%d want 321", got)
		}
	})
	t.Run("systemd user", func(t *testing.T) {
		runner := &Runner{
			Getenv: func(string) string { return "" },
			RunCommand: func(context.Context, string, []string, io.Reader) (CommandOutput, error) {
				return CommandOutput{Stdout: []byte("gateway\n")}, nil
			},
			LookupUID: func(name string) (int, error) {
				if name != "gateway" {
					t.Errorf("name=%q", name)
				}
				return 456, nil
			},
		}
		if got := runner.expectedOwnerUID(context.Background()); got != 456 {
			t.Errorf("uid=%d want 456", got)
		}
	})
	t.Run("current user fallback", func(t *testing.T) {
		runner := &Runner{
			Getenv: func(string) string { return "" },
			RunCommand: func(context.Context, string, []string, io.Reader) (CommandOutput, error) {
				return CommandOutput{}, errors.New("systemctl missing")
			},
		}
		if got := runner.expectedOwnerUID(context.Background()); got != os.Getuid() {
			t.Errorf("uid=%d want %d", got, os.Getuid())
		}
	})
}

func TestRunCNBranchOverwritesExistingAccount(t *testing.T) {
	fake := newFakeLoginServer(t)
	runner := baseRunner(t, fake, RealmCN)
	if err := os.MkdirAll(runner.AuthDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := AuthFilePath(runner.AuthDir, "u1")
	if err := os.WriteFile(oldPath, []byte(`{"old":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runner.Run(context.Background()); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, runner.Err.(*strings.Builder).String())
	}
	if !fake.hasPath("/v2/billing/meter/daily-checkin") {
		t.Error("CN login did not run daily check-in")
	}
	if fake.hasPath("/auth/realms/copilot/overseas/user/register") ||
		fake.hasPath("/billing/ide/trial") {
		t.Error("CN login called global endpoints")
	}
	fake.mu.Lock()
	headers := fake.checkinHeaders.Clone()
	fake.mu.Unlock()
	if headers.Get("X-User-Id") != "u1" ||
		headers.Get("X-Enterprise-Id") != "ent1" ||
		headers.Get("X-Tenant-Id") != "ent1" ||
		headers.Get("X-Domain") != "www.codebuddy.cn" {
		t.Errorf("check-in headers=%v", headers)
	}
	raw, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := auth.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.AccessToken != "access-token-value" || parsed.ExpiresAt != 1_700_003_600 {
		t.Errorf("parsed=%+v", parsed)
	}
	if info, _ := os.Stat(oldPath); info.Mode().Perm() != 0o600 {
		t.Errorf("mode=%o", info.Mode().Perm())
	}
	out := runner.Out.(*strings.Builder).String()
	if !strings.Contains(out, "账号已存在") || !strings.Contains(out, "已保存（覆盖）") {
		t.Errorf("output=%s", out)
	}
}

func TestRunGlobalBranchSkipsCheckinAndHandlesTrialAlreadyClaimed(t *testing.T) {
	fake := newFakeLoginServer(t)
	fake.trialBody = `{"code":14051,"msg":"already claimed"}`
	runner := baseRunner(t, fake, RealmGlobal)
	if code := runner.Run(context.Background()); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, runner.Err.(*strings.Builder).String())
	}
	if fake.hasPath("/v2/billing/meter/daily-checkin") {
		t.Error("global login ran CN check-in")
	}
	if !fake.hasPath("/auth/realms/copilot/overseas/user/register") ||
		!fake.hasPath("/billing/ide/trial") {
		t.Errorf("global endpoints missing: %v", fake.paths)
	}
	if !strings.Contains(runner.Out.(*strings.Builder).String(), "已领取过") {
		t.Errorf("output=%s", runner.Out.(*strings.Builder).String())
	}
}

func TestGlobalFailureDoesNotRollbackSavedCredentials(t *testing.T) {
	fake := newFakeLoginServer(t)
	fake.globalRegister = `{"code":123,"msg":"register failed"}`
	fake.trialBody = `{"code":14052,"msg":"trial failed"}`
	runner := baseRunner(t, fake, RealmGlobal)
	if code := runner.Run(context.Background()); code != 0 {
		t.Fatalf("exit=%d", code)
	}
	raw, err := os.ReadFile(AuthFilePath(runner.AuthDir, "u1"))
	if err != nil {
		t.Fatalf("saved credential missing after global failure: %v", err)
	}
	parsed, err := auth.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Realm() != RealmGlobal {
		t.Errorf("realm=%q want global", parsed.Realm())
	}
	out := runner.Out.(*strings.Builder).String()
	if !strings.Contains(out, "注册激活: register failed") ||
		!strings.Contains(out, "trial failed（不阻断登录）") {
		t.Errorf("output=%s", out)
	}
}

func TestRunInteractiveRealmSelection(t *testing.T) {
	fake := newFakeLoginServer(t)
	runner := baseRunner(t, fake, RealmCN)
	runner.RealmExplicit = false
	runner.StdinTTY = true
	runner.In = strings.NewReader("2\ny\n")
	if code := runner.Run(context.Background()); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, runner.Err.(*strings.Builder).String())
	}
	if !fake.hasPath("/auth/realms/copilot/overseas/user/register") {
		t.Error("interactive global selection did not use global branch")
	}
	if !strings.Contains(runner.Err.(*strings.Builder).String(), "选择登录版本") {
		t.Errorf("stderr=%s", runner.Err.(*strings.Builder).String())
	}
}

func TestMarshalAuthFileRejectsMissingIdentity(t *testing.T) {
	if _, err := MarshalAuthFile(Credentials{AccessToken: "at"}); err == nil {
		t.Fatal("missing uid was accepted")
	}
	if _, err := MarshalAuthFile(Credentials{UID: "u1"}); err == nil {
		t.Fatal("missing access token was accepted")
	}
}

func TestPermissionAdviceHost(t *testing.T) {
	host := strings.Join(PermissionAdvice("auths", 123, func(int) (string, error) {
		return "alice", nil
	}), "\n")
	if !strings.Contains(host, "sudo chown -R alice:alice auths") {
		t.Errorf("host advice=%s", host)
	}
}

func TestStateFileHasRealmAndMode(t *testing.T) {
	fake := newFakeLoginServer(t)
	statePath := filepath.Join(t.TempDir(), "state.json")
	oauth := NewOAuthClient(fake.Client(), RealmGlobal, fake.URL, fake.URL, statePath)
	if _, err := oauth.AuthURL(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var state loginState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if state.State != "s1" || state.Realm != RealmGlobal {
		t.Errorf("state=%+v", state)
	}
	if info, _ := os.Stat(statePath); info.Mode().Perm() != 0o600 {
		t.Errorf("mode=%o", info.Mode().Perm())
	}
}
