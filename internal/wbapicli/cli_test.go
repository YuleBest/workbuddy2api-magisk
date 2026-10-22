package wbapicli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) { return f(req) }

type runnerCall struct {
	Name string
	Args []string
}

type fakeRunner struct {
	mu        sync.Mutex
	responses map[string]CommandResult
	calls     []runnerCall
}

func (f *fakeRunner) Run(name string, args ...string) CommandResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, runnerCall{Name: name, Args: append([]string(nil), args...)})
	key := name + " " + strings.Join(args, " ")
	if result, ok := f.responses[key]; ok {
		return result
	}
	return CommandResult{ExitCode: 0}
}

type fakeProcess struct {
	pid   int
	owner *fakePlatform
}

func (p *fakeProcess) PID() int { return p.pid }
func (p *fakeProcess) Signal(os.Signal) error {
	p.owner.mu.Lock()
	defer p.owner.mu.Unlock()
	p.owner.alive[p.pid] = false
	return nil
}
func (p *fakeProcess) Release() error { return nil }

type fakePlatform struct {
	mu         sync.Mutex
	osName     string
	alive      map[int]bool
	nextPID    int
	terminated []int
	sleeps     []time.Duration
}

func (f *fakePlatform) GOOS() string { return f.osName }
func (f *fakePlatform) ProcessAlive(pid int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive[pid]
}
func (f *fakePlatform) Terminate(pid int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.terminated = append(f.terminated, pid)
	f.alive[pid] = false
	return nil
}
func (f *fakePlatform) StartProcess(string, []string, string, io.Writer, io.Writer) (ProcessHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.nextPID == 0 {
		f.nextPID = 1234
	}
	pid := f.nextPID
	f.nextPID++
	f.alive[pid] = true
	return &fakeProcess{pid: pid, owner: f}, nil
}
func (f *fakePlatform) UserID(string) (int, error)       { return 1000, nil }
func (f *fakePlatform) FileOwnerUID(string) (int, error) { return 1000, nil }
func (f *fakePlatform) Readable(string) bool             { return true }
func (f *fakePlatform) Sleep(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sleeps = append(f.sleeps, d)
}

func testResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func runTest(t *testing.T, args []string, cfg string, handler roundTripFunc) (int, string, string, *fakeRunner, *fakePlatform) {
	t.Helper()
	project := t.TempDir()
	if cfg != "" {
		if err := os.WriteFile(filepath.Join(project, "config.json"), []byte(cfg), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	runner := &fakeRunner{responses: map[string]CommandResult{
		"systemctl is-active wbapi-gateway": {Stdout: "active\n"},
		"systemctl is-active wbapi-tunnel":  {Stdout: "active\n"},
	}}
	platform := &fakePlatform{osName: "linux", alive: make(map[int]bool)}
	deps := Dependencies{
		HTTPClient: handler,
		Runner:     runner,
		Platform:   platform,
		Now:        func() time.Time { return time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC) },
		Random:     bytes.NewReader(bytes.Repeat([]byte{0xab}, 64)),
	}
	code := Run(Options{
		Args:    args,
		Env:     Env{"WB2A_GATEWAY": "http://gateway.test"},
		Project: project,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Deps:    deps,
	})
	return code, stdout.String(), stderr.String(), runner, platform
}

func TestHealthcheckOnlyAcceptsHTTP200(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		err    error
		want   int
	}{
		{"ok", 200, nil, 0},
		{"unhealthy", 503, nil, 1},
		{"network", 0, errors.New("dial failed"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, stdout, _, _, _ := runTest(t, []string{"healthcheck"}, "", func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/healthz" {
					t.Errorf("path=%s", req.URL.Path)
				}
				if tc.err != nil {
					return nil, tc.err
				}
				return testResponse(tc.status, `not-json`), nil
			})
			if code != tc.want {
				t.Fatalf("code=%d want %d", code, tc.want)
			}
			if stdout != "" {
				t.Errorf("healthcheck should be silent, got %q", stdout)
			}
		})
	}
}

func TestStatusAndAccountsUseExpectedTokensAndFormatting(t *testing.T) {
	cfg := `{"api_key":"cfg-key"}`
	var statusAuth string
	code, stdout, _, _, _ := runTest(t, nil, cfg, func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/healthz":
			return testResponse(200, `{"healthy":true,"total":2,"realm_servable":{"global":true,"cn":false}}`), nil
		case "/status":
			statusAuth = req.Header.Get("Authorization")
			return testResponse(200, `{
				"healthy":1,"cooling":1,"disabled":0,"in_flight_full":0,"sticky_sessions":2,
				"accounts":[
					{"uid":"u-global","realm":"global","nickname":"国际号","credits":12,"cooling":true,"cool_kind":"soft_rate","cool_remaining_sec":62,"rate_limited_models":[{"model":"glm"}]},
					{"uid":"u-cn","realm":"cn","nickname":"国内号","credits":3,"disabled":true,"disabled_reason":"12153"}
				]
			}`), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	})
	if code != 0 {
		t.Fatalf("status code=%d\n%s", code, stdout)
	}
	for _, want := range []string{
		"网关:   active",
		"健康:   healthy=True  账号数=2  可接活域=global",
		"池:     健康 1 / 冷却 1 / 禁用 0 / 在途占满 0 / 粘性 2",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("status output missing %q:\n%s", want, stdout)
		}
	}
	if statusAuth != "Bearer cfg-key" {
		t.Errorf("status auth=%q", statusAuth)
	}

	code, stdout, _, _, _ = runTest(t, []string{"accounts"}, cfg, func(req *http.Request) (*http.Response, error) {
		return testResponse(200, `{
			"accounts":[
				{"uid":"u-global","realm":"global","nickname":"国际号","credits":12,"cooling":true,"cool_kind":"soft_rate","cool_remaining_sec":62,"rate_limited_models":[{"model":"glm"}],"last_success":"2026-10-20T10:11:12Z"},
				{"uid":"u-cn","realm":"cn","nickname":"国内号","credits":3,"disabled":true,"disabled_reason":"12153"}
			]
		}`), nil
	})
	if code != 0 {
		t.Fatalf("accounts code=%d", code)
	}
	for _, want := range []string{
		"账号池（2 个，域 2 个）:",
		"  [cn]",
		"  [global]",
		"国际号",
		"限流冷却，剩 1m02s",
		"系统禁用（12153）",
		"最近成功 2026-10-20 10:11:12",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("accounts output missing %q:\n%s", want, stdout)
		}
	}
}

func TestKeyLifecycleAndFileMode(t *testing.T) {
	project := t.TempDir()
	cfg := `{"keys_file":"keys.json"}`
	if err := os.WriteFile(filepath.Join(project, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	platform := &fakePlatform{osName: "linux", alive: make(map[int]bool)}
	deps := Dependencies{
		HTTPClient: roundTripFunc(func(*http.Request) (*http.Response, error) { return testResponse(200, `{}`), nil }),
		Runner:     &fakeRunner{responses: map[string]CommandResult{}},
		Platform:   platform,
		Now:        func() time.Time { return time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC) },
		Random:     bytes.NewReader(bytes.Repeat([]byte{0x01}, 16)),
	}
	run := func(args ...string) int {
		stdout.Reset()
		stderr.Reset()
		return Run(Options{Args: args, Env: Env{}, Project: project, Stdout: &stdout, Stderr: &stderr, Deps: deps})
	}
	if code := run("key", "new", "部署"); code != 0 {
		t.Fatalf("new code=%d out=%s err=%s", code, stdout.String(), stderr.String())
	}
	const key = "sk-01010101010101010101010101010101"
	if !strings.Contains(stdout.String(), key) {
		t.Fatalf("new output missing key: %s", stdout.String())
	}
	path := filepath.Join(project, "keys.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode=%#o want 0600", mode)
	}
	raw, _ := os.ReadFile(path)
	if strings.Index(string(raw), `"key"`) > strings.Index(string(raw), `"note"`) ||
		strings.Index(string(raw), `"note"`) > strings.Index(string(raw), `"added"`) {
		t.Fatalf("keys field order changed: %s", raw)
	}
	var doc struct {
		Keys []struct {
			Key   string `json:"key"`
			Note  string `json:"note"`
			Added string `json:"added"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Keys) != 1 || doc.Keys[0].Key != key || doc.Keys[0].Note != "部署" || doc.Keys[0].Added != "2026-10-20" {
		t.Fatalf("unexpected keys doc: %s", raw)
	}
	if code := run("key", "list"); code != 0 || !strings.Contains(stdout.String(), "sk-01010…0101") {
		t.Fatalf("masked list code=%d out=%s", code, stdout.String())
	}
	if code := run("key", "revoke", "sk-0101"); code != 0 {
		t.Fatalf("revoke code=%d out=%s", code, stdout.String())
	}
	if code := run("key", "list"); code != 0 || !strings.Contains(stdout.String(), "里没有 key") {
		t.Fatalf("list after revoke code=%d out=%s", code, stdout.String())
	}
}

func TestModelsTaskAndStatsResetRoutes(t *testing.T) {
	cfg := `{"api_key":"data-key","admin":{"token":"admin-key"}}`
	modelsJSON := `{"data":[
		{"id":"global:gpt-x","name":"GPT X","credits":"x0.34 credits","context_length":1000,"max_output_tokens":100,"supports_images":true},
		{"id":"cn:glm","name":"","credits":"x0.00","context_length":2000,"max_output_tokens":0}
	]}`
	var gotTaskURL string
	var gotTaskAuth string
	handler := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/v1/models":
			return testResponse(200, modelsJSON), nil
		case req.URL.Path == "/api/admin/tasks/checkin":
			gotTaskURL = req.URL.RequestURI()
			gotTaskAuth = req.Header.Get("Authorization")
			return testResponse(200, `{"message":"签到完成","waited":true,"outcomes":[{"status":"ok","nickname":"甲","uid":"u1","credits":5}]}`), nil
		case req.URL.Path == "/api/admin/stats/reset":
			return testResponse(404, `{"error":{"message":"not found"}}`), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	})
	code, stdout, _, _, _ := runTest(t, []string{"models", "--realm", "global"}, cfg, handler)
	if code != 0 || !strings.Contains(stdout, "模型（1 个，域 global）") ||
		!strings.Contains(stdout, "x0.34") || !strings.Contains(stdout, "识图") {
		t.Fatalf("models code=%d out=%s", code, stdout)
	}

	code, stdout, _, _, _ = runTest(t, []string{"task", "checkin"}, cfg, handler)
	if code != 0 || gotTaskURL != "/api/admin/tasks/checkin?wait=1" || gotTaskAuth != "Bearer admin-key" {
		t.Fatalf("task code=%d url=%s auth=%s out=%s", code, gotTaskURL, gotTaskAuth, stdout)
	}
	if !strings.Contains(stdout, "✓ 签到成功") || !strings.Contains(stdout, "甲") {
		t.Errorf("task outcome not rendered: %s", stdout)
	}

	code, stdout, _, _, _ = runTest(t, []string{"stats", "--reset"}, cfg, handler)
	if code != 1 || !strings.Contains(stdout, "管理面未开启") {
		t.Fatalf("stats reset code=%d out=%s", code, stdout)
	}
}

func TestQuotaTestRequestBodyPreservesFieldOrder(t *testing.T) {
	var body string
	handler := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path=%s", req.URL.Path)
		}
		raw, _ := io.ReadAll(req.Body)
		body = string(raw)
		return testResponse(200, `{"choices":[{"message":{"content":"ok"}}]}`), nil
	})
	code, stdout, _, _, _ := runTest(t, []string{"quota", "test", "glm"}, `{"api_key":"key"}`, handler)
	if code != 0 || !strings.Contains(stdout, "可用 1/1") {
		t.Fatalf("code=%d out=%s", code, stdout)
	}
	want := `{"model":"glm","messages":[{"role":"user","content":"hi"}],"max_tokens":16}`
	if body != want {
		t.Errorf("request body=%s want %s", body, want)
	}
}

func TestConfigFallbackRedactsAndServiceCommands(t *testing.T) {
	cfg := `{"listen":":7863","api_key":"secret","upstash":{"url":"redis://u","token":"tok"},"admin":{"token":"adm"},"upstream":{"device_token":"dev"}}`
	handler := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return testResponse(401, `{"error":{"message":"unauthorized"}}`), nil
	})
	code, stdout, _, _, _ := runTest(t, []string{"config"}, cfg, handler)
	if code != 0 {
		t.Fatalf("config code=%d", code)
	}
	for _, secret := range []string{"secret", "redis://u", `"tok"`, `"adm"`, `"dev"`} {
		if strings.Contains(stdout, secret) {
			t.Errorf("config leaked %q:\n%s", secret, stdout)
		}
	}
	if strings.Count(stdout, `"***"`) != 5 {
		t.Errorf("redaction count mismatch:\n%s", stdout)
	}
	if strings.Index(stdout, `"listen"`) > strings.Index(stdout, `"api_key"`) ||
		strings.Index(stdout, `"api_key"`) > strings.Index(stdout, `"upstash"`) {
		t.Errorf("local config order was not preserved:\n%s", stdout)
	}

	code, stdout, _, runner, _ := runTest(t, []string{"start"}, cfg, handler)
	if code != 0 || !strings.Contains(stdout, "start wbapi-gateway: ok") ||
		!strings.Contains(stdout, "start wbapi-tunnel: ok") {
		t.Fatalf("start code=%d out=%s", code, stdout)
	}
	if len(runner.calls) != 2 {
		t.Errorf("runner calls=%+v", runner.calls)
	}
}

func TestConfigRemotePreservesServerObjectOrder(t *testing.T) {
	handler := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return testResponse(200, `{"config":{"z":1,"listen":":9000","admin":{"enabled":true,"token_set":true}}}`), nil
	})
	code, stdout, _, _, _ := runTest(t, []string{"config"}, `{"api_key":"x"}`, handler)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if strings.Index(stdout, `"z"`) > strings.Index(stdout, `"listen"`) ||
		strings.Index(stdout, `"listen"`) > strings.Index(stdout, `"admin"`) {
		t.Errorf("remote config order was not preserved:\n%s", stdout)
	}
}

func TestBoolValuesRenderLikePython(t *testing.T) {
	code, stdout, _, _, _ := runTest(t, []string{"stats"}, "", func(req *http.Request) (*http.Response, error) {
		return testResponse(200, `{"enabled":true,"total":{"requests":1,"success":1,"failed":0,"total_tokens":2},"models":[]}`), nil
	})
	if code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(stdout, "请求 1") {
		t.Fatalf("unexpected stats output: %s", stdout)
	}
}
