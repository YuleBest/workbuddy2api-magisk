package wbapicli

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDevStartStatusKeyStop(t *testing.T) {
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "wb2api"), []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "config.json"), []byte(`{"api_key":"dev-key"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	httpClient := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/healthz":
			return testResponse(200, `{"ok":true}`), nil
		case "/status":
			if req.Header.Get("Authorization") != "Bearer dev-key" {
				t.Errorf("status auth=%q", req.Header.Get("Authorization"))
			}
			return testResponse(200, `{"accounts":[{"uid":"1"},{"uid":"2"}]}`), nil
		default:
			t.Fatalf("path=%s", req.URL.Path)
			return nil, nil
		}
	})
	platform := &fakePlatform{osName: "linux", alive: make(map[int]bool)}
	deps := Dependencies{HTTPClient: httpClient, Platform: platform}
	var stdout, stderr bytes.Buffer
	run := func(args ...string) int {
		stdout.Reset()
		stderr.Reset()
		return RunDev(DevOptions{
			Args: args, Env: Env{}, Project: project,
			Stdout: &stdout, Stderr: &stderr, Deps: deps,
		})
	}

	if code := run("start"); code != 0 || !strings.Contains(stdout.String(), "已启动 (PID 1234)") {
		t.Fatalf("start code=%d out=%s err=%s", code, stdout.String(), stderr.String())
	}
	pidRaw, err := os.ReadFile(filepath.Join(project, ".server.pid"))
	if err != nil || strings.TrimSpace(string(pidRaw)) != "1234" {
		t.Fatalf("pid file=%q err=%v", pidRaw, err)
	}
	if code := run("start"); code != 0 || !strings.Contains(stdout.String(), "已在运行 (PID 1234)") {
		t.Fatalf("second start code=%d out=%s", code, stdout.String())
	}
	if code := run("status"); code != 0 {
		t.Fatalf("status code=%d", code)
	}
	for _, want := range []string{"运行中 (PID 1234)", `{"ok":true}`, "账号数: 2"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("status missing %q:\n%s", want, stdout.String())
		}
	}
	if code := run("key"); code != 0 || strings.TrimSpace(stdout.String()) != "dev-key" {
		t.Fatalf("key code=%d out=%q", code, stdout.String())
	}
	if code := run("stop"); code != 0 || !strings.Contains(stdout.String(), "已停止") {
		t.Fatalf("stop code=%d out=%s", code, stdout.String())
	}
	if _, err := os.Stat(filepath.Join(project, ".server.pid")); !os.IsNotExist(err) {
		t.Errorf("pid file should be removed, err=%v", err)
	}
	if code := run("status"); code != 0 || strings.TrimSpace(stdout.String()) != "未运行" {
		t.Fatalf("status after stop code=%d out=%q", code, stdout.String())
	}
}

func TestDevUnknownAction(t *testing.T) {
	var stdout bytes.Buffer
	code := RunDev(DevOptions{
		Args: []string{"wat"}, Project: t.TempDir(), Stdout: &stdout, Env: Env{},
	})
	if code != 1 || !strings.Contains(stdout.String(), "用法: dev") {
		t.Fatalf("code=%d out=%s", code, stdout.String())
	}
}
