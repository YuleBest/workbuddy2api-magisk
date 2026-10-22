package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestExecutableName(t *testing.T) {
	if got := executableName("wbapi", "linux"); got != "wbapi" {
		t.Fatalf("linux executable = %q", got)
	}
	if got := executableName("wbapi", "windows"); got != "wbapi.exe" {
		t.Fatalf("windows executable = %q", got)
	}
}

func TestPrintTargetsIncludesCompatibilityCommands(t *testing.T) {
	var out bytes.Buffer
	printTargets(&out, "linux")
	for _, want := range []string{
		"wb2api\t./cmd/server",
		"login.sh\tcompat:login",
		"checkin.sh\tcompat:checkin",
		"school_open_day_cron.sh\tcompat:school_open_day_cron",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("target list missing %q:\n%s", want, out.String())
		}
	}
}

func TestPrintTargetsWindowsHasNoShellNames(t *testing.T) {
	var out bytes.Buffer
	printTargets(&out, "windows")
	if strings.Contains(out.String(), ".sh") {
		t.Fatalf("windows target list contains compatibility shell names:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "wbapi.exe\t./cmd/wbapi") {
		t.Fatalf("windows target list missing wbapi.exe:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "server.exe\tcompat:wb2api") {
		t.Fatalf("windows target list missing server.exe alias:\n%s", out.String())
	}
}

func TestWithEnvReplacesValues(t *testing.T) {
	got := withEnv([]string{"A=1", "GOOS=old", "B=2"}, map[string]string{
		"GOOS":        "linux",
		"CGO_ENABLED": "0",
	})
	joined := strings.Join(got, "\n")
	for _, want := range []string{"A=1", "B=2", "GOOS=linux", "CGO_ENABLED=0"} {
		if !strings.Contains(joined, want) {
			t.Errorf("environment missing %q: %v", want, got)
		}
	}
	if strings.Contains(joined, "GOOS=old") {
		t.Fatalf("old GOOS was not replaced: %v", got)
	}
}
