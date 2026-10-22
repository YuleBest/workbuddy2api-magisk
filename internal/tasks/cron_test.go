package tasks

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTailLogLinesKeepsRequestedTail(t *testing.T) {
	var source bytes.Buffer
	for i := 0; i < 10; i++ {
		fmt.Fprintf(&source, "line-%d\n", i)
	}
	tail := string(tailLogLines(source.Bytes(), 3))
	if tail != "line-7\nline-8\nline-9\n" {
		t.Fatalf("tail=%q", tail)
	}
}

func TestTaskCronSkipsWhenLockHeld(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lock, ok, err := acquireCronLock(filepath.Join(dataDir, "school-cron.lock"))
	if err != nil || !ok {
		t.Fatalf("acquire lock ok=%t err=%v", ok, err)
	}
	defer lock.Close()

	service := NewService()
	service.Runtime = newFakeClock().runtime(http.DefaultClient)
	var stdout, stderr strings.Builder
	code := TaskCron(context.Background(), []string{"school"}, &stdout, &stderr, service, root)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, "school-cron.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "skip[school]: 上一次运行仍持有锁") {
		t.Fatalf("skip was not logged:\n%s", raw)
	}
}

func TestTaskCronRunsSchoolAndRotatesLargeLog(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dataDir, "school-cron.log")
	var old bytes.Buffer
	for i := 0; i < 3000; i++ {
		fmt.Fprintf(&old, "2026-10-19 00:00:00 line=%04d %s\n", i, strings.Repeat("x", 700))
	}
	if old.Len() <= cronLogLimit {
		t.Fatalf("fixture size=%d must exceed %d", old.Len(), cronLogLimit)
	}
	if err := os.WriteFile(logPath, old.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/school/portal/activity/school/tasks" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		writeJSONResponse(w, 200, map[string]any{
			"code": 0,
			"data": map[string]any{"in_period": false, "tasks": []any{}},
		})
	})
	service, _ := testService(t, handler, newFakeClock())
	writeAuthFile(t, service.AuthsDir, "workbuddy-cron0001.json",
		authDocument("cron0001-uid", "token", "copilot.tencent.com", "cn"))

	var stdout, stderr strings.Builder
	code := TaskCron(context.Background(), []string{"school"}, &stdout, &stderr, service, root)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	rotated, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(rotated) >= old.Len() {
		t.Fatalf("log was not rotated: old=%d new=%d", old.Len(), len(rotated))
	}
	lines := bytes.Split(bytes.TrimSuffix(rotated, []byte{'\n'}), []byte{'\n'})
	if len(lines) > cronTailLines {
		t.Fatalf("rotated lines=%d want <=%d", len(lines), cronTailLines)
	}
	text := string(rotated)
	if !strings.Contains(text, "cron[school] start:") ||
		!strings.Contains(text, "mode=RUN REAL gap=1.5") ||
		!strings.Contains(text, "cron[school] end: 完成") {
		t.Fatalf("latest cron block missing:\n%s", text[len(text)-500:])
	}
}
