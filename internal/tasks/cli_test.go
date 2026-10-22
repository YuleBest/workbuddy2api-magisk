package tasks

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSchoolCLIInterleavedFlagsDryRunHasNoWrites(t *testing.T) {
	log := &requestLog{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		switch r.URL.Path {
		case "/school/portal/activity/school/tasks":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"in_period": true,
					"tasks": []any{map[string]any{
						"task_code": "share_invite", "status": "pending",
						"progress": 0, "target_count": 1, "reward_credit": 10,
					}},
				},
			})
		case "/school/portal/activity/school/config":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"in_period": true,
					"chance":    map[string]any{"balance": 0},
				},
			})
		default:
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		}
	})
	service, _ := testService(t, handler, nil)
	writeAuthFile(t, service.AuthsDir, "workbuddy-dryrun01.json",
		authDocument("dryrun01-uid", "token", "copilot.tencent.com", "cn"))

	var stdout, stderr strings.Builder
	code := SchoolCLI([]string{"--run", "--list", "dryrun01", "--gap", "0.2"}, &stdout, &stderr, service)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s\n%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "mode=RUN dry-run") {
		t.Errorf("unexpected output:\n%s", stdout.String())
	}
	for _, request := range log.snapshot() {
		if request.Method == "POST" {
			t.Errorf("dry-run sent write request: %s %s", request.Method, request.Path)
		}
	}
}

func TestSchoolCLIModePriorityAndTokenEscape(t *testing.T) {
	log := &requestLog{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		if r.URL.Path != "/school/portal/activity/school/config" {
			t.Errorf("lottery-only should only read config, got %s", r.URL.Path)
		}
		writeJSONResponse(w, 200, map[string]any{
			"code": 0,
			"data": map[string]any{
				"in_period": true,
				"chance":    map[string]any{"balance": 0},
			},
		})
	})
	service, _ := testService(t, handler, nil)
	var stdout, stderr strings.Builder
	code := SchoolCLI([]string{
		"--run", "--lottery-only", "--yes", "--token", "override-token", "--uid", "override-uid",
		"--gap", "0.2",
	}, &stdout, &stderr, service)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s\n%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "mode=LOTTERY REAL gap=1.0") {
		t.Errorf("mode priority not preserved:\n%s", stdout.String())
	}
	requests := log.snapshot()
	if len(requests) == 0 || requests[0].Header.Get("Authorization") != "Bearer override-token" {
		t.Fatalf("token override not used: %+v", requests)
	}
}

func TestSchoolCLIExitTwoWhenWorkflowFails(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, 500, map[string]any{"code": 500})
	})
	service, _ := testService(t, handler, newFakeClock())
	var stdout, stderr strings.Builder
	code := SchoolCLI([]string{
		"--run", "--yes", "--token", "token", "--uid", "uid",
	}, &stdout, &stderr, service)
	if code != 2 {
		t.Fatalf("exit=%d want school failure exit 2\n%s", code, stdout.String())
	}
	if !strings.Contains(stdout.String(), "fail=1") {
		t.Errorf("failure summary missing:\n%s", stdout.String())
	}
}

func TestTaskRunnerCLIInterleavedAndAlwaysZeroOnWorkflowError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, 500, map[string]any{"code": 500})
	})
	service, _ := testService(t, handler, nil)
	writeAuthFile(t, service.AuthsDir, "workbuddy-error001.json",
		authDocument("error001-uid", "token", "copilot.tencent.com", "cn"))

	var stdout, stderr strings.Builder
	code := TaskRunnerCLI([]string{"error001", "--only", "chat_5", "--gap", "0.2"},
		&stdout, &stderr, service)
	if code != 0 {
		t.Fatalf("task_runner must exit 0 after workflow failures, got %d stderr=%s\n%s",
			code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "only=['chat_5']") ||
		!strings.Contains(stdout.String(), "gap=1.0") ||
		!strings.Contains(stdout.String(), "list_tasks 失败") {
		t.Errorf("unexpected output:\n%s", stdout.String())
	}
}

func TestTaskRunnerDryRunHasNoWrites(t *testing.T) {
	log := &requestLog{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		switch r.URL.Path {
		case "/chat/v2/activity/growth/energy":
			writeJSONResponse(w, 200, map[string]any{"code": 0, "data": map[string]any{"balance": 1}})
		case "/chat/v2/activity/growth/tasks":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{"tasks": []any{map[string]any{
					"task_code": "chat_5", "accept_status": "in_progress",
					"progress": map[string]any{"current": 0, "target": 5},
				}}},
			})
		default:
			http.Error(w, "write endpoint must not be called", 500)
		}
	})
	service, _ := testService(t, handler, nil)
	writeAuthFile(t, service.AuthsDir, "workbuddy-dryrun02.json",
		authDocument("dryrun02-uid", "token", "copilot.tencent.com", "cn"))

	var stdout, stderr strings.Builder
	code := TaskRunnerCLI([]string{"--only", "chat_5", "dryrun02"}, &stdout, &stderr, service)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s\n%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "mode=DRY-RUN") ||
		!strings.Contains(stdout.String(), "dry-run 跳过") {
		t.Errorf("unexpected dry-run output:\n%s", stdout.String())
	}
	for _, request := range log.snapshot() {
		if request.Method == "POST" {
			t.Errorf("dry-run sent write request: %s %s", request.Method, request.Path)
		}
	}
}

func TestTaskRunnerOnlyClaimDryRunDoesNotLightUp(t *testing.T) {
	log := &requestLog{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		switch r.URL.Path {
		case "/chat/v2/activity/growth/energy":
			writeJSONResponse(w, 200, map[string]any{"code": 0, "data": map[string]any{"balance": 1}})
		case "/chat/v2/activity/growth/tasks":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{"tasks": []any{map[string]any{
					"task_code": "chat_5", "accept_status": "completed",
					"progress": map[string]any{"current": 5, "target": 5},
				}}},
			})
		default:
			http.Error(w, "write endpoint must not be called", 500)
		}
	})
	service, _ := testService(t, handler, nil)
	writeAuthFile(t, service.AuthsDir, "workbuddy-dryrun03.json",
		authDocument("dryrun03-uid", "token", "copilot.tencent.com", "cn"))

	var stdout, stderr strings.Builder
	code := TaskRunnerCLI([]string{"dryrun03", "--only-claim", "--only", "chat_5"},
		&stdout, &stderr, service)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s\n%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "only_claim=True") ||
		!strings.Contains(stdout.String(), "only_claim dry-run 跳过") {
		t.Errorf("unexpected only-claim output:\n%s", stdout.String())
	}
	for _, request := range log.snapshot() {
		if request.Method == "POST" {
			t.Errorf("only-claim dry-run sent write request: %s %s", request.Method, request.Path)
		}
	}
}

func TestGlobalAccountSkipsWithoutRequests(t *testing.T) {
	var calls atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "no request expected", 500)
	})
	service, _ := testService(t, handler, nil)
	writeAuthFile(t, service.AuthsDir, "workbuddy-global01.json",
		authDocument("global01-uid", "token", "www.workbuddy.ai", ""))

	var stdout, stderr strings.Builder
	code := TaskRunnerCLI([]string{"global01"}, &stdout, &stderr, service)
	if code != 0 || calls.Load() != 0 {
		t.Fatalf("exit=%d calls=%d stderr=%s\n%s", code, calls.Load(), stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "global realm 不适用 CN 任务") {
		t.Errorf("missing global skip:\n%s", stdout.String())
	}
}

func TestCLIAllExpansion(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/v2/activity/growth/energy":
			writeJSONResponse(w, 200, map[string]any{"code": 0, "data": map[string]any{"balance": 1}})
		case "/chat/v2/activity/growth/tasks":
			writeJSONResponse(w, 200, map[string]any{"code": 0, "data": map[string]any{"tasks": []any{}}})
		case "/school/portal/activity/school/tasks":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0, "data": map[string]any{"in_period": false, "tasks": []any{}},
			})
		case "/school/portal/activity/freshman/profile", "/school/portal/activity/teacher/profile":
			writeJSONResponse(w, 200, map[string]any{"code": 0, "data": map[string]any{
				"claimed": false, "enabled": true,
			}})
		default:
			http.NotFound(w, r)
		}
	})
	service, _ := testService(t, handler, nil)
	for _, item := range []struct {
		prefix string
		uid    string
	}{
		{prefix: "acct0001", uid: "acct0001-uid"},
		{prefix: "acct0002", uid: "acct0002-uid"},
	} {
		writeAuthFile(t, service.AuthsDir, "workbuddy-"+item.prefix+".json",
			authDocument(item.uid, "token", "copilot.tencent.com", "cn"))
	}

	var stdout, stderr strings.Builder
	if code := TaskRunnerCLI([]string{"ALL", "--only", "chat_5"}, &stdout, &stderr, service); code != 0 {
		t.Fatalf("task_runner ALL exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "accounts=['acct0001', 'acct0002']") ||
		!strings.Contains(stdout.String(), "accounts=2") {
		t.Errorf("task_runner ALL did not expand/dedupe:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := SchoolCLI([]string{"ALL"}, &stdout, &stderr, service); code != 0 {
		t.Fatalf("school ALL exit=%d stderr=%s\n%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "accounts=2") {
		t.Errorf("school ALL did not process both accounts:\n%s", stdout.String())
	}
}

func TestCLIUsageErrorsAndHelp(t *testing.T) {
	service := NewService()
	var stdout, stderr strings.Builder
	if code := SchoolCLI([]string{"--help"}, &stdout, &stderr, service); code != 0 {
		t.Fatalf("school help exit=%d", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := SchoolCLI([]string{"--unknown"}, &stdout, &stderr, service); code != 2 {
		t.Fatalf("school invalid exit=%d want 2", code)
	}
	stdout.Reset()
	stderr.Reset()
	if code := TaskRunnerCLI(nil, &stdout, &stderr, service); code != 2 {
		t.Fatalf("task_runner no accounts exit=%d want 2", code)
	}
}
