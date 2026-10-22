package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"workbuddy2api/internal/region"
)

func TestRunHelpAndUsageExitCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want int
	}{
		{name: "help", args: []string{"--help"}, want: 0},
		{name: "missing uid", args: nil, want: 2},
		{name: "unknown flag", args: []string{"--uid", "u1", "--bad"}, want: 2},
		{name: "extra argument", args: []string{"--uid", "u1", "extra"}, want: 2},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var stdout, stderr strings.Builder
			got := run(tc.args, func(string) string { return "" }, strings.NewReader(""), &stdout, &stderr, nil)
			if got != tc.want {
				t.Fatalf("exit=%d want %d; stdout=%q stderr=%q", got, tc.want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunRequiresTokenFromEnvironment(t *testing.T) {
	t.Parallel()
	var stdout, stderr strings.Builder
	got := run(
		[]string{"--uid", "u1"},
		func(string) string { return "" },
		strings.NewReader(""),
		&stdout,
		&stderr,
		nil,
	)
	if got != 1 {
		t.Fatalf("exit=%d want 1", got)
	}
	if !strings.Contains(stderr.String(), "WORKBUDDY_TOKEN") {
		t.Errorf("stderr=%q", stderr.String())
	}
}

func TestRunAlreadyRegistered(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/realms/copilot/overseas/user/register" {
			t.Errorf("path=%q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"code":200,"msg":"register success"}`))
	}))
	defer server.Close()

	var stdout, stderr strings.Builder
	got := run(
		[]string{"--uid", "u1"},
		func(key string) string {
			if key == "WORKBUDDY_TOKEN" {
				return "tok"
			}
			return ""
		},
		strings.NewReader(""),
		&stdout,
		&stderr,
		&region.Client{HTTPClient: server.Client(), BaseURL: server.URL},
	)
	if got != 0 {
		t.Fatalf("exit=%d stderr=%q", got, stderr.String())
	}
	if stdout.String() != "register: 成功\n" {
		t.Errorf("stdout=%q", stdout.String())
	}
}

func TestRunAutoCompletesRegion(t *testing.T) {
	t.Parallel()
	var activations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/realms/copilot/overseas/user/register":
			if activations.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"code":500,"msg":"region required"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":200,"msg":"register success"}`))
		case "/billing/area/get-country-code":
			inner := `{"data":{"list":[{"EnName":"Singapore","IOS2":"SG","Code":"702"}]}}`
			encoded, _ := json.Marshal(inner)
			_, _ = io.WriteString(w, `{"code":0,"data":`+string(encoded)+`}`)
		case "/console/login/account":
			_, _ = w.Write([]byte(`{"code":0}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	var stdout, stderr strings.Builder
	got := run(
		[]string{"--uid", "u1", "--auto", "SG"},
		func(key string) string {
			if key == "WORKBUDDY_TOKEN" {
				return "tok"
			}
			return ""
		},
		strings.NewReader(""),
		&stdout,
		&stderr,
		&region.Client{HTTPClient: server.Client(), BaseURL: server.URL},
	)
	if got != 0 {
		t.Fatalf("exit=%d stderr=%q stdout=%q", got, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "complete: 地区已完善") {
		t.Errorf("stdout=%q", stdout.String())
	}
}
