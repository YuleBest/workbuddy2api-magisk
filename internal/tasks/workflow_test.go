package tasks

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

type taskState struct {
	mu           sync.Mutex
	code         string
	acceptStatus string
	current      int
	target       int
	writes       []recordedRequest
}

func (s *taskState) setAccepted() {
	s.mu.Lock()
	s.acceptStatus = "accepted"
	s.mu.Unlock()
}

func (s *taskState) setProgress(current int) {
	s.mu.Lock()
	s.current = current
	s.acceptStatus = "in_progress"
	if current >= s.target {
		s.acceptStatus = "completed"
	}
	s.mu.Unlock()
}

func (s *taskState) complete() {
	s.mu.Lock()
	s.current = s.target
	s.acceptStatus = "completed"
	s.mu.Unlock()
}

func (s *taskState) task() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{
		"task_code":     s.code,
		"accept_status": s.acceptStatus,
		"progress":      map[string]any{"current": s.current, "target": s.target},
	}
}

func (s *taskState) record(r *http.Request) []byte {
	raw, _ := io.ReadAll(r.Body)
	r.Body.Close()
	s.mu.Lock()
	s.writes = append(s.writes, recordedRequest{
		Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: raw,
	})
	s.mu.Unlock()
	return raw
}

func workflowHandler(state *taskState) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/chat/v2/activity/growth/tasks":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0, "data": map[string]any{"tasks": []any{state.task()}},
			})
		case r.Method == "POST" && r.URL.Path == "/chat/v2/activity/growth/tasks/accept":
			state.record(r)
			state.setAccepted()
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{"results": []any{map[string]any{"status": "accepted"}}},
			})
		case r.Method == "POST" && r.URL.Path == "/billing/v2/report":
			state.record(r)
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		case r.Method == "POST" && r.URL.Path == "/chat/v2/report":
			raw := state.record(r)
			events := decodeEventArrayFromBytes(raw)
			if len(events) > 0 && state.code != "first_buddy" {
				state.setProgress(state.target)
			}
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		case r.Method == "POST" && r.URL.Path == "/web/v2/report":
			state.record(r)
			state.setProgress(state.target)
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		case r.Method == "POST" && r.URL.Path == "/chat/activity/growth/buddy/agreement":
			state.record(r)
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		case r.Method == "POST" && r.URL.Path == "/chat/activity/growth/buddy/first":
			state.record(r)
			state.complete()
			writeJSONResponse(w, 200, map[string]any{
				"code": 0, "msg": "ok", "data": map[string]any{"credit": 300, "energy": 8},
			})
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/chat/activity/growth/tasks/"):
			state.record(r)
			state.mu.Lock()
			state.acceptStatus = "claimed"
			state.mu.Unlock()
			writeJSONResponse(w, 200, map[string]any{
				"code": 0, "data": map[string]any{"already_claimed": true, "credit": 10, "energy": 1},
			})
		default:
			http.NotFound(w, r)
		}
	})
}

func decodeEventArrayFromBytes(raw []byte) []map[string]any {
	var events []map[string]any
	_ = json.Unmarshal(raw, &events)
	return events
}

func TestSpecialTaskWorkflowsAndEventBodies(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		wantPath    string
		wantEvents  int
		wantCodes   []string
		wantCredit  int
		wantEnergy  int
		wantAlready int
	}{
		{
			name: "first_buddy", code: "first_buddy",
			wantPath: "/chat/activity/growth/buddy/first", wantEvents: 0,
			wantCredit: 300, wantEnergy: 8, wantAlready: 1,
		},
		{
			name: "richmeow", code: "RichMeow_Chat",
			wantPath: "/chat/v2/report", wantEvents: 6,
			wantCodes: []string{
				"agent_task_created", "chat_message_send", "chat_request_send",
				"chat_message_response", "chat_message_status", "chat_request_response",
			},
			wantAlready: 1,
		},
		{
			name: "buddy5", code: "Buddy_App",
			wantPath: "/chat/v2/report", wantEvents: 5,
			wantCodes: []string{
				"buddyapp_discover_click", "buddyapp_show", "buddyapp_enter_click",
				"buddyapp_auth_confirm_click", "buddyapp_bindaccount_skip_click",
			},
			wantAlready: 1,
		},
		{
			name: "library", code: "Library_read",
			wantPath: "/web/v2/report", wantEvents: 1,
			wantCodes: []string{"web_element_click"}, wantAlready: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state := &taskState{code: tc.code, acceptStatus: "accepted", target: 1}
			service, _ := testService(t, workflowHandler(state), nil)
			account := testAccount()
			task := state.task()
			var stats Stats
			var out strings.Builder
			service.processTask(context.Background(), account, tc.code, task,
				TaskRunnerOptions{Yes: true, Gap: 1}, &stats, &out)

			state.mu.Lock()
			writes := append([]recordedRequest(nil), state.writes...)
			state.mu.Unlock()
			foundPath := false
			for _, request := range writes {
				if request.Path != tc.wantPath {
					continue
				}
				foundPath = true
				events := decodeEventArrayFromBytes(request.Body)
				if tc.wantEvents > 0 {
					if len(events) != tc.wantEvents {
						t.Fatalf("path=%s events=%d want %d body=%s", request.Path, len(events), tc.wantEvents, request.Body)
					}
					for i, code := range tc.wantCodes {
						if stringValue(events[i]["eventCode"]) != code {
							t.Errorf("event[%d]=%v want %s", i, events[i]["eventCode"], code)
						}
					}
					switch tc.code {
					case "RichMeow_Chat", "Buddy_App":
						if events[0]["machineId"] != deriveID(account, "machine") ||
							events[0]["sessionId"] != deriveID(account, "session") {
							t.Errorf("desktop fingerprint missing: %v", events[0])
						}
					case "Library_read":
						if events[0]["machineId"] != deriveID(account, "webmachine") ||
							events[0]["userId"] != account.UID {
							t.Errorf("web fingerprint missing: %v", events[0])
						}
					}
				}
			}
			if !foundPath {
				t.Fatalf("no request to %s; writes=%+v\n%s", tc.wantPath, writes, out.String())
			}
			if tc.code == "first_buddy" {
				foundReport := false
				for _, request := range writes {
					if request.Path != "/billing/v2/report" {
						continue
					}
					events := decodeEventArrayFromBytes(request.Body)
					if len(events) != 1 || events[0]["userId"] != account.UID ||
						events[0]["eventCode"] != "chat_request_send" {
						t.Errorf("first_buddy prerequisite body=%s", request.Body)
					}
					foundReport = true
				}
				if !foundReport {
					t.Fatalf("first_buddy prerequisite report missing")
				}
			}
			if stats.Credit != tc.wantCredit || stats.Energy != tc.wantEnergy ||
				stats.Already != tc.wantAlready {
				t.Errorf("stats=%+v want credit=%d energy=%d already=%d\n%s",
					stats, tc.wantCredit, tc.wantEnergy, tc.wantAlready, out.String())
			}
		})
	}
}

func TestBuddy5FallsBackToSingleEvent(t *testing.T) {
	state := &taskState{code: "Buddy_App", acceptStatus: "accepted", target: 1}
	reportCalls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/chat/v2/activity/growth/tasks":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0, "data": map[string]any{"tasks": []any{state.task()}},
			})
		case r.Method == "POST" && r.URL.Path == "/chat/v2/report":
			raw := state.record(r)
			reportCalls++
			events := decodeEventArrayFromBytes(raw)
			if reportCalls == 1 {
				if len(events) != 5 {
					t.Errorf("first report events=%d want 5", len(events))
				}
				writeJSONResponse(w, 200, map[string]any{"code": 1, "msg": "fallback"})
				return
			}
			if len(events) != 1 || events[0]["eventCode"] != "buddyapp_enter_click" {
				t.Errorf("fallback body=%s", raw)
			}
			state.complete()
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/chat/activity/growth/tasks/"):
			state.record(r)
			writeJSONResponse(w, 200, map[string]any{
				"code": 0, "data": map[string]any{"already_claimed": true},
			})
		default:
			http.NotFound(w, r)
		}
	})
	service, _ := testService(t, handler, nil)
	var stats Stats
	var out strings.Builder
	service.processTask(context.Background(), testAccount(), "Buddy_App", state.task(),
		TaskRunnerOptions{Yes: true, Gap: 1}, &stats, &out)
	if reportCalls != 2 || stats.Already != 1 {
		t.Fatalf("reportCalls=%d stats=%+v\n%s", reportCalls, stats, out.String())
	}
}

func TestSchoolTaskWorkflowUsesViewedReportClaim(t *testing.T) {
	state := &taskState{code: "chat_3_times", acceptStatus: "pending", target: 1}
	calls := []string{}
	var mu sync.Mutex
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		switch {
		case r.Method == "GET" && r.URL.Path == "/school/portal/activity/school/tasks":
			status := "in_progress"
			if state.current >= state.target {
				status = "completed"
			}
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"in_period": true,
					"tasks": []any{map[string]any{
						"task_code": "chat_3_times", "status": status,
						"progress": state.current, "target_count": state.target, "reward_credit": 100,
					}},
				},
			})
		case r.Method == "POST" && r.URL.Path == "/school/portal/activity/school/tasks/chat_3_times/viewed":
			state.setAccepted()
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		case r.Method == "POST" && r.URL.Path == "/school/v2/report":
			raw := state.record(r)
			events := decodeEventArrayFromBytes(raw)
			if len(events) != 1 || events[0]["activityId"] != schoolActivityID {
				t.Errorf("school report event=%s", raw)
			}
			state.setProgress(state.target)
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		case r.Method == "POST" && r.URL.Path == "/school/portal/activity/school/tasks/chat_3_times/claim":
			state.record(r)
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		default:
			http.NotFound(w, r)
		}
	})
	service, _ := testService(t, handler, nil)
	account := testAccount()
	task := map[string]any{
		"task_code": "chat_3_times", "status": "pending", "progress": 0,
		"target_count": 1, "reward_credit": 100,
	}
	var stats Stats
	var out strings.Builder
	service.processSchoolTask(context.Background(), account, "chat_3_times", task,
		TaskRunnerOptions{Yes: true, Gap: 1}, &stats, &out)
	if stats.Total != 1 || stats.OK != 1 || stats.Fail != 0 {
		t.Fatalf("stats=%+v\n%s", stats, out.String())
	}
	joined := strings.Join(calls, "\n")
	for _, want := range []string{"viewed", "/school/v2/report", "/claim"} {
		if !strings.Contains(joined, want) {
			t.Errorf("calls missing %s:\n%s", want, joined)
		}
	}
}

func TestSchoolDesktopSequenceUsesCopilotFingerprint(t *testing.T) {
	state := &taskState{code: "desktop_chat_1_time", acceptStatus: "in_progress", target: 1}
	var reportRequest recordedRequest
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/school/portal/activity/school/tasks":
			status := "in_progress"
			if state.current >= state.target {
				status = "completed"
			}
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"in_period": true,
					"tasks": []any{map[string]any{
						"task_code": "desktop_chat_1_time", "status": status,
						"progress": state.current, "target_count": 1, "reward_credit": 50,
					}},
				},
			})
		case r.Method == "POST" && r.URL.Path == "/copilot/v2/report":
			raw := state.record(r)
			reportRequest = recordedRequest{
				Method: r.Method, Path: r.URL.Path, Header: r.Header.Clone(), Body: raw,
			}
			state.setProgress(state.target)
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		case r.Method == "POST" && r.URL.Path == "/school/portal/activity/school/tasks/desktop_chat_1_time/claim":
			state.record(r)
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		default:
			http.NotFound(w, r)
		}
	})
	service, _ := testService(t, handler, nil)
	task := map[string]any{
		"task_code": "desktop_chat_1_time", "status": "in_progress", "progress": 0,
		"target_count": 1, "reward_credit": 50,
	}
	var stats Stats
	var out strings.Builder
	service.processSchoolTask(context.Background(), testAccount(), "desktop_chat_1_time", task,
		TaskRunnerOptions{Yes: true, Gap: 1}, &stats, &out)
	if reportRequest.Path == "" {
		t.Fatalf("copilot report missing\n%s", out.String())
	}
	if reportRequest.Header.Get("User-Agent") != desktopUA ||
		reportRequest.Header.Get("X-Product") != "SaaS" ||
		reportRequest.Header.Get("X-User-Id") != "" {
		t.Errorf("school desktop headers=%v", reportRequest.Header)
	}
	events := decodeEventArrayFromBytes(reportRequest.Body)
	if len(events) != 6 {
		t.Fatalf("desktop school events=%d want 6", len(events))
	}
	for i, event := range events {
		if event["activityId"] != schoolActivityID ||
			event["machineId"] != deriveID(testAccount(), "machine") {
			t.Errorf("event[%d] fingerprint=%v", i, event)
		}
	}
}

func TestSchoolExpertWorkflowUsesBackToSchoolEvent(t *testing.T) {
	state := &taskState{code: "expert_use", acceptStatus: "in_progress", target: 1}
	var reportBody []map[string]any
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/school/portal/activity/school/tasks":
			status := "in_progress"
			if state.current >= state.target {
				status = "completed"
			}
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"in_period": true,
					"tasks": []any{map[string]any{
						"task_code": "expert_use", "status": status,
						"progress": state.current, "target_count": 1, "reward_credit": 20,
					}},
				},
			})
		case r.Method == "POST" && r.URL.Path == "/school/v2/operation-platform/market/expert/list":
			state.record(r)
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{"experts": []any{map[string]any{
					"expert_id":       "ex_school",
					"display_name_zh": map[string]any{"zh": "开学专家"},
					"profession_zh":   map[string]any{"zh": "学习导师"},
				}}},
			})
		case r.Method == "POST" && r.URL.Path == "/school/v2/report":
			raw := state.record(r)
			reportBody = decodeEventArrayFromBytes(raw)
			state.setProgress(state.target)
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		case r.Method == "POST" && r.URL.Path == "/school/portal/activity/school/tasks/expert_use/claim":
			state.record(r)
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		default:
			http.NotFound(w, r)
		}
	})
	service, _ := testService(t, handler, nil)
	task := map[string]any{
		"task_code": "expert_use", "status": "in_progress", "progress": 0,
		"target_count": 1, "reward_credit": 20,
	}
	var stats Stats
	var out strings.Builder
	service.processSchoolTask(context.Background(), testAccount(), "expert_use", task,
		TaskRunnerOptions{Yes: true, Gap: 1}, &stats, &out)
	if len(reportBody) != 1 {
		t.Fatalf("expert report body=%v\n%s", reportBody, out.String())
	}
	event := reportBody[0]
	if event["eventCode"] != "expert_actual_use" ||
		event["id"] != "ex_school" ||
		event["expertTitle"] != "开学专家" ||
		event["activityId"] != schoolActivityID {
		t.Errorf("expert event=%v", event)
	}
}

func TestSchoolShareWorkflowUsesShareComplete(t *testing.T) {
	state := &taskState{code: "share_invite", acceptStatus: "in_progress", target: 1}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/school/portal/activity/school/tasks":
			status := "in_progress"
			if state.current >= state.target {
				status = "completed"
			}
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"in_period": true,
					"tasks": []any{map[string]any{
						"task_code": "share_invite", "status": status,
						"progress": state.current, "target_count": 1, "reward_credit": 10,
					}},
				},
			})
		case r.Method == "POST" && r.URL.Path == "/school/portal/activity/school/tasks/share-complete":
			state.record(r)
			state.setProgress(state.target)
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		case r.Method == "POST" && r.URL.Path == "/school/portal/activity/school/tasks/share_invite/claim":
			state.record(r)
			writeJSONResponse(w, 200, map[string]any{"code": 0})
		default:
			http.NotFound(w, r)
		}
	})
	service, _ := testService(t, handler, nil)
	task := map[string]any{
		"task_code": "share_invite", "status": "in_progress", "progress": 0,
		"target_count": 1, "reward_credit": 10,
	}
	var stats Stats
	var out strings.Builder
	service.processSchoolTask(context.Background(), testAccount(), "share_invite", task,
		TaskRunnerOptions{Yes: true, Gap: 1}, &stats, &out)
	state.mu.Lock()
	writes := append([]recordedRequest(nil), state.writes...)
	state.mu.Unlock()
	foundShare := false
	for _, request := range writes {
		if request.Path != "/school/portal/activity/school/tasks/share-complete" {
			continue
		}
		foundShare = true
		body := decodeBody(t, request.Body)
		if body["channel"] != "wechat" {
			t.Errorf("share body=%s", request.Body)
		}
	}
	if !foundShare || stats.OK != 1 {
		t.Fatalf("share=%t stats=%+v\n%s", foundShare, stats, out.String())
	}
}

func TestMiniGrowthWorkflowUsesMiniprogramHeaders(t *testing.T) {
	for _, tc := range []struct {
		code         string
		withActivity bool
	}{
		{code: "Sequential_Tasks_1", withActivity: false},
		{code: "school_season", withActivity: true},
	} {
		t.Run(tc.code, func(t *testing.T) {
			state := &taskState{code: tc.code, acceptStatus: "not_accepted", target: 1}
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "GET" && r.URL.Path == "/chat/v2/activity/growth/tasks":
					if r.Header.Get("X-Client-Platform") != "miniprogram" {
						t.Errorf("mp list missing header")
					}
					writeJSONResponse(w, 200, map[string]any{
						"code": 0, "data": map[string]any{"tasks": []any{state.task()}},
					})
				case r.Method == "POST" && r.URL.Path == "/chat/v2/activity/growth/tasks/accept":
					if r.Header.Get("X-Client-Platform") != "miniprogram" {
						t.Errorf("mp accept missing header")
					}
					state.record(r)
					state.setAccepted()
					writeJSONResponse(w, 200, map[string]any{
						"code": 0, "data": map[string]any{"results": []any{map[string]any{"status": "accepted"}}},
					})
				case r.Method == "POST" && r.URL.Path == "/school/v2/report":
					raw := state.record(r)
					events := decodeEventArrayFromBytes(raw)
					if len(events) != 1 {
						t.Fatalf("events=%s", raw)
					}
					_, hasActivity := events[0]["activityId"]
					if hasActivity != tc.withActivity {
						t.Errorf("activityId present=%t want %t body=%s", hasActivity, tc.withActivity, raw)
					}
					if events[0]["source"] != "mini_program" {
						t.Errorf("source=%v", events[0]["source"])
					}
					state.setProgress(state.target)
					writeJSONResponse(w, 200, map[string]any{"code": 0})
				case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/chat/activity/growth/tasks/"):
					if r.Header.Get("X-Client-Platform") != "miniprogram" {
						t.Errorf("mp claim missing header")
					}
					state.record(r)
					writeJSONResponse(w, 200, map[string]any{
						"code": 0, "data": map[string]any{"credit": 100, "energy": 5},
					})
				default:
					http.NotFound(w, r)
				}
			})
			service, _ := testService(t, handler, nil)
			var stats Stats
			var out strings.Builder
			service.processMinichatTask(context.Background(), testAccount(), tc.code, nil,
				TaskRunnerOptions{Yes: true, Gap: 1}, &stats, &out)
			if stats.OK != 1 || stats.Credit != 100 || stats.Energy != 5 {
				t.Fatalf("stats=%+v\n%s", stats, out.String())
			}
		})
	}
}
