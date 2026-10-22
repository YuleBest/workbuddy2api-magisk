package wbapicli

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsServiceAndTunnelBranches(t *testing.T) {
	project := t.TempDir()
	runner := &fakeRunner{responses: map[string]CommandResult{
		"schtasks /Run /TN wbapi-gateway": {ExitCode: 0, Stdout: "ok"},
		"schtasks /Run /TN wbapi-tunnel":  {ExitCode: 0, Stdout: "ok"},
		"tasklist /FI IMAGENAME eq cloudflared.exe": {
			ExitCode: 0,
			Stdout:   "cloudflared.exe 1234 Console 1 10,000 K",
		},
	}}
	platform := &fakePlatform{osName: "windows", alive: make(map[int]bool)}
	deps := Dependencies{
		Runner:   runner,
		Platform: platform,
		HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/healthz":
				return testResponse(200, `{"healthy":true,"total":1}`), nil
			case "/status":
				return testResponse(200, `{"healthy":1,"cooling":0,"disabled":0,"in_flight_full":0,"sticky_sessions":0}`), nil
			default:
				t.Fatalf("path=%s", req.URL.Path)
				return nil, nil
			}
		}),
	}
	var stdout, stderr bytes.Buffer
	run := func(args ...string) int {
		stdout.Reset()
		stderr.Reset()
		return Run(Options{
			Args: args, Env: Env{"WB2A_GATEWAY": "http://gateway.test"},
			Project: project, Stdout: &stdout, Stderr: &stderr, Deps: deps,
		})
	}
	if code := run("start"); code != 0 {
		t.Fatalf("start code=%d out=%s", code, stdout.String())
	}
	for _, want := range []string{"start wbapi-gateway: [✓] 完成", "start wbapi-tunnel: [✓] 完成"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("start missing %q:\n%s", want, stdout.String())
		}
	}
	if code := run("status"); code != 0 {
		t.Fatalf("status code=%d", code)
	}
	if !strings.Contains(stdout.String(), "隧道:   active") {
		t.Errorf("Windows tunnel probe not active:\n%s", stdout.String())
	}
	if code := run("tunnel", "stop"); code != 0 || !strings.Contains(stdout.String(), "[✓] 完成") {
		t.Fatalf("tunnel stop code=%d out=%s", code, stdout.String())
	}
	if code := run("tunnel", "unknown"); code != 0 || !strings.Contains(stdout.String(), "[✓] unknown 完成") {
		t.Fatalf("unknown tunnel should preserve historical success: code=%d out=%s", code, stdout.String())
	}
}

func TestWindowsLogAndConfigParseWarning(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "config.json"), []byte(`{broken`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "gateway.err.log"), []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	platform := &fakePlatform{osName: "windows", alive: make(map[int]bool)}
	var stdout bytes.Buffer
	code := Run(Options{
		Args: []string{"log", "2"}, Env: Env{}, Project: project, Stdout: &stdout,
		Deps: Dependencies{Platform: platform},
	})
	if code != 0 || !strings.Contains(stdout.String(), "配置文件解析失败") {
		t.Fatalf("code=%d out=%s", code, stdout.String())
	}
	if !strings.HasSuffix(stdout.String(), "two\nthree\n") {
		t.Errorf("log tail incorrect: %q", stdout.String())
	}
}

func TestDoctorLinuxHappyPath(t *testing.T) {
	project := t.TempDir()
	authDir := filepath.Join(project, "auths")
	if err := os.Mkdir(authDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "workbuddy-a.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	keysPath := filepath.Join(project, "keys.json")
	if err := os.WriteFile(keysPath, []byte(`{"keys":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(project, "wb2api")
	if err := os.WriteFile(binPath, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	unitPath := filepath.Join(project, "wbapi-gateway.service")
	if err := os.WriteFile(unitPath, []byte("ExecStart="+binPath+" -config config.json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := `{"api_key":"key","auth_dir":"auths","keys_file":"keys.json","admin":{"enabled":true}}`
	if err := os.WriteFile(filepath.Join(project, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{responses: map[string]CommandResult{
		"systemctl show -p User --value wbapi-gateway":         {Stdout: "tester\n"},
		"systemctl show -p FragmentPath --value wbapi-gateway": {Stdout: unitPath + "\n"},
		"systemctl is-active wbapi-gateway":                    {Stdout: "active\n"},
		"systemctl is-active wbapi-tunnel":                     {Stdout: "active\n"},
	}}
	platform := &fakePlatform{osName: "linux", alive: make(map[int]bool)}
	var stdout, stderr bytes.Buffer
	code := Run(Options{
		Args: []string{"doctor"}, Env: Env{"WB2A_GATEWAY": "http://gateway.test"},
		Project: project, Stdout: &stdout, Stderr: &stderr,
		Deps: Dependencies{
			Runner:   runner,
			Platform: platform,
			HTTPClient: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return testResponse(200, `{"healthy":1,"total":1,"realm_servable":{"cn":true}}`), nil
			}),
		},
	})
	if code != 0 {
		t.Fatalf("doctor code=%d out=%s err=%s", code, stdout.String(), stderr.String())
	}
	for _, want := range []string{
		"unit 已加载（User=tester）",
		"二进制存在：" + binPath,
		"凭证文件 1 个",
		"keys.json 权限 0o600",
		"可接活账号 1/1",
		"0 项待处理",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("doctor missing %q:\n%s", want, stdout.String())
		}
	}
}

func TestModelsMoreSkipsPythonFalsyUnknownFields(t *testing.T) {
	handler := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return testResponse(200, `{"data":[{"id":"cn:m","name":"模型","credits":"x0.21","context_length":1000,"max_output_tokens":20,"extra_zero":0,"extra_false":false,"extra_empty":[],"extra_list":["a","b"],"extra_bool":true}]}`), nil
	})
	code, stdout, _, _, _ := runTest(t, []string{"models", "--more"}, "", handler)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	for _, hidden := range []string{"extra_zero", "extra_false", "extra_empty"} {
		if strings.Contains(stdout, hidden) {
			t.Errorf("falsy field %s should be skipped:\n%s", hidden, stdout)
		}
	}
	for _, want := range []string{"extra_list", "a, b", "extra_bool", "是", "x0.21"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("more output missing %q:\n%s", want, stdout)
		}
	}
}

func TestBaseURLResolutionMatchesCLIContract(t *testing.T) {
	cases := []struct {
		name string
		env  Env
		cfg  string
		want string
	}{
		{"default", Env{}, ``, "http://127.0.0.1:7863"},
		{"loopback", Env{}, `{"listen":"127.0.0.1:9000"}`, "http://127.0.0.1:9000"},
		{"wildcard", Env{}, `{"listen":"0.0.0.0:9001"}`, "http://127.0.0.1:9001"},
		{"ipv6 wildcard", Env{}, `{"listen":"[::]:9002"}`, "http://127.0.0.1:9002"},
		{"port only", Env{}, `{"listen":"9003"}`, "http://127.0.0.1:9003"},
		{"gateway override", Env{"WB2A_GATEWAY": "http://example.test/"}, `{"listen":"127.0.0.1:9000"}`, "http://example.test"},
		{"empty gateway is unset", Env{"WB2A_GATEWAY": ""}, `{"listen":":9004"}`, "http://127.0.0.1:9004"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			if tc.cfg != "" {
				if err := os.WriteFile(filepath.Join(project, "config.json"), []byte(tc.cfg), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			app := newApp(Options{
				Env: tc.env, Project: project,
				Deps: Dependencies{Platform: &fakePlatform{osName: "linux", alive: make(map[int]bool)}},
			})
			if got := app.baseURL(); got != tc.want {
				t.Errorf("baseURL=%s want %s", got, tc.want)
			}
		})
	}
}
