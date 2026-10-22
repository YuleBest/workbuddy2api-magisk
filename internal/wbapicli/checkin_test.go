package wbapicli

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"workbuddy2api/internal/auth"
	"workbuddy2api/internal/upstream"
)

type fakeSigninClient struct {
	checkinErr error
	remain     int64
}

func (f *fakeSigninClient) RefreshToken(*auth.Auth) error { return nil }
func (f *fakeSigninClient) DailyCheckin(*auth.Auth) error { return f.checkinErr }
func (f *fakeSigninClient) UserResource(*auth.Auth) (int64, error) {
	return f.remain, nil
}

func TestRunCheckinStatusHeaderTableAndVerboseFailures(t *testing.T) {
	project := t.TempDir()
	authDir := filepath.Join(project, "auths")
	if err := os.Mkdir(authDir, 0o700); err != nil {
		t.Fatal(err)
	}
	valid := `{"auth":{"accessToken":"at","refreshToken":"rt","expiresAt":4102444800,"domain":""},"account":{"uid":"uid-123456789","nickname":"测试号"}}`
	if err := os.WriteFile(filepath.Join(authDir, "workbuddy-good.json"), []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "workbuddy-bad.json"), []byte(`{bad`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "config.json"), []byte(`{"listen":"127.0.0.1:9999","api_key":"checkin-key"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var statusAuth string
	httpClient := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/status" {
			t.Fatalf("path=%s", req.URL.Path)
		}
		statusAuth = req.Header.Get("Authorization")
		return testResponse(200, `{"total":2,"healthy":1,"cooling":1,"disabled":0}`), nil
	})
	var stdout, stderr bytes.Buffer
	code := RunCheckin(CheckinOptions{
		Args:    []string{"-v"},
		Env:     Env{"WB2A_URL": "http://checkin.test"},
		Project: project,
		Stdout:  &stdout,
		Stderr:  &stderr,
		Deps: Dependencies{
			HTTPClient: httpClient,
			Platform:   &fakePlatform{osName: "linux", alive: make(map[int]bool)},
		},
		Client: &fakeSigninClient{
			checkinErr: &upstream.Error{Kind: upstream.ErrClient, Status: 409, Msg: "code=10001 msg=今天已签到"},
			remain:     42,
		},
	})
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if statusAuth != "Bearer checkin-key" {
		t.Errorf("status auth=%q", statusAuth)
	}
	out := stdout.String()
	for _, want := range []string{
		"── 网关状态",
		"网关在线：total=2 healthy=1 cooling=1 disabled=0",
		"提示：定时签到已在网关侧运行",
		"uid-123456789",
		"测试号",
		"ALREADY",
		"total=2 ok=0 already=1 fail=1",
		"── 问题明细（仅 fail/错误，已签到不计）──",
		"LOAD_ERR",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	failureTail := out[strings.Index(out, "── 问题明细"):]
	if strings.Contains(failureTail, "ALREADY") {
		t.Errorf("verbose failure section must exclude ALREADY:\n%s", failureTail)
	}
}

func TestRunCheckinArgumentErrorsAndNoAuthFiles(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCheckin(CheckinOptions{
		Args:    []string{"-h"},
		Project: t.TempDir(),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Env:     Env{},
	})
	if code != 0 || !strings.Contains(stdout.String(), "签到结果表格化工具") {
		t.Fatalf("help code=%d out=%s", code, stdout.String())
	}
	stdout.Reset()
	code = RunCheckin(CheckinOptions{
		Args:    []string{"--bad"},
		Project: t.TempDir(),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Env:     Env{},
	})
	if code != 2 || !strings.Contains(stderr.String(), "未知参数") {
		t.Fatalf("bad arg code=%d err=%s", code, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = RunCheckin(CheckinOptions{
		Args:    []string{"-v"},
		Project: t.TempDir(),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Env:     Env{"WB2A_URL": "http://checkin.test"},
		Deps: Dependencies{
			HTTPClient: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("offline")
			}),
		},
	})
	if code != 0 || !strings.Contains(stderr.String(), "no auth files in auths") {
		t.Fatalf("verbose no-auth code=%d stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "── 问题明细") || !strings.Contains(stdout.String(), "（无 fail）") {
		t.Fatalf("verbose no-auth output=%s", stdout.String())
	}
}

func TestRunCheckinMalformedStatusFallsBackToHTTP200Message(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := RunCheckin(CheckinOptions{
		Args:    []string{"-h"},
		Project: t.TempDir(),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Env:     Env{},
	})
	if code != 0 {
		t.Fatal(code)
	}

	stdout.Reset()
	code = RunCheckin(CheckinOptions{
		Args:    []string{},
		Project: t.TempDir(),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Env:     Env{"WB2A_URL": "http://checkin.test"},
		Deps: Dependencies{
			HTTPClient: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return testResponse(200, `not json`), nil
			}),
		},
	})
	if code != 1 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(stdout.String(), "网关在线（/status 200）") {
		t.Fatalf("fallback header missing:\n%s", stdout.String())
	}
}
