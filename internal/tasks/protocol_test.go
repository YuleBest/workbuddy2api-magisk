package tasks

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func TestLoadAccountIsReadOnlyAndUsesRawRealm(t *testing.T) {
	service := NewService()
	service.AuthsDir = t.TempDir()
	path := writeAuthFile(t, service.AuthsDir, "workbuddy-abcdefgh.json",
		authDocument("uid-abcdefgh-1234", "token", "www.workbuddy.ai", ""))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.loadAccount("abcdefgh")
	if err != nil {
		t.Fatal(err)
	}
	if !account.IsGlobal() {
		t.Fatal("domain=www.workbuddy.ai must resolve global without auth.Realm escape gate")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("loadAccount wrote back or backfilled the auth file")
	}
}

func TestGlobalRealmIsCaseInsensitiveAndIgnoresEscapeGate(t *testing.T) {
	account := testAccount()
	account.Realm = "  GLOBAL "
	account.Domain = ""
	if !account.IsGlobal() {
		t.Fatal("raw GLOBAL realm must be treated as global like the legacy scripts")
	}
}

func TestFourHTTPDomainFingerprints(t *testing.T) {
	log := &requestLog{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		writeJSONResponse(w, 200, map[string]any{"code": 0})
	})
	service, _ := testService(t, handler, nil)
	account := testAccount()
	ctx := context.Background()

	service.doGrowth(ctx, account, "POST", service.BillingBase, "/v2/report",
		[]map[string]any{{"eventCode": "growth"}}, nil)
	service.reportSchoolEvents(ctx, io.Discard, account,
		[]map[string]any{miniChatEvent(account, 123, "conv", true)}, service.CloudAgent, nil, 1)
	service.reportDesktopEvents(ctx, account, desktopChatSequence(
		account, 123, "conv", "request", "message", "fast-model", "fast-model"))
	service.reportWebEvent(ctx, account, "web_element_click", "https://page", "id", "name")

	requests := log.snapshot()
	if len(requests) != 4 {
		t.Fatalf("requests=%d want 4", len(requests))
	}
	growth, school, desktop, web := requests[0], requests[1], requests[2], requests[3]

	if growth.Path != "/billing/v2/report" || growth.Header.Get("User-Agent") != growthUA {
		t.Errorf("growth request path/header=%s/%q", growth.Path, growth.Header.Get("User-Agent"))
	}
	if growth.Header.Get("Origin") != "https://www.codebuddy.cn" ||
		growth.Header.Get("Referer") != "https://www.codebuddy.cn/" ||
		growth.Header.Get("X-User-Id") != account.UID ||
		growth.Header.Get("X-Domain") != account.Domain {
		t.Errorf("growth fingerprint headers=%v", growth.Header)
	}
	if growth.Header.Get("Accept-Encoding") != "identity" ||
		growth.Header.Get("Connection") != "close" {
		t.Errorf("urllib-compatible transport headers missing: %v", growth.Header)
	}

	if school.Path != "/school/v2/report" || school.Header.Get("User-Agent") != mpUA {
		t.Errorf("school request path/header=%s/%q", school.Path, school.Header.Get("User-Agent"))
	}
	if school.Header.Get("X-User-Id") != "" {
		t.Errorf("school mini-program request must not inject X-User-Id: %v", school.Header)
	}
	schoolEvents := decodeEventArray(t, school.Body)
	if len(schoolEvents) != 1 || schoolEvents[0]["activityId"] != schoolActivityID {
		t.Errorf("school event body=%s", school.Body)
	}

	if desktop.Path != "/chat/v2/report" || desktop.Header.Get("User-Agent") != desktopUA {
		t.Errorf("desktop request path/header=%s/%q", desktop.Path, desktop.Header.Get("User-Agent"))
	}
	if desktop.Header.Get("X-Product") != "SaaS" || desktop.Header.Get("X-User-Id") != account.UID ||
		desktop.Header.Get("X-Domain") != service.ChatBase {
		t.Errorf("desktop fingerprint headers=%v", desktop.Header)
	}
	if events := decodeEventArray(t, desktop.Body); len(events) != 6 {
		t.Errorf("desktop sequence events=%d want 6", len(events))
	}

	if web.Path != "/web/v2/report" || web.Header.Get("User-Agent") != webUA {
		t.Errorf("web request path/header=%s/%q", web.Path, web.Header.Get("User-Agent"))
	}
	if web.Header.Get("x-client-platform") != "web" ||
		web.Header.Get("X-Domain") != service.WebBase ||
		web.Header.Get("Referer") != "https://page" {
		t.Errorf("web fingerprint headers=%v", web.Header)
	}
}

func TestSchoolRetryThreeAttemptsWithLinearGap(t *testing.T) {
	clock := newFakeClock()
	calls := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			writeJSONResponse(w, 500, map[string]any{"code": 500})
			return
		}
		writeJSONResponse(w, 200, map[string]any{
			"code": 0,
			"data": map[string]any{"in_period": true, "tasks": []any{}},
		})
	})
	service, _ := testService(t, handler, clock)
	tasks, inPeriod, err := service.fetchSchoolTasks(context.Background(), io.Discard, testAccount(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || !inPeriod || len(tasks) != 0 {
		t.Fatalf("calls=%d in_period=%t tasks=%d", calls, inPeriod, len(tasks))
	}
	if len(clock.sleeps) != 2 || clock.sleeps[0].Seconds() != 1 || clock.sleeps[1].Seconds() != 2 {
		t.Fatalf("sleeps=%v want [1s 2s]", clock.sleeps)
	}
}

func TestClaim400FallsBackToWebFingerprint(t *testing.T) {
	log := &requestLog{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		switch r.URL.Path {
		case "/chat/activity/growth/tasks/chat_5/claim":
			writeJSONResponse(w, 400, map[string]any{"code": 400, "msg": "chat domain rejects"})
		case "/web/activity/growth/tasks/chat_5/claim":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{"credit": 7, "energy": 2},
			})
		default:
			http.NotFound(w, r)
		}
	})
	service, _ := testService(t, handler, nil)
	var out strings.Builder
	var stats Stats
	result := service.claimOne(context.Background(), testAccount(), "chat_5", "uid-1234",
		&stats, 1, false, &out)
	if result != 1 || stats.Credit != 7 || stats.Energy != 2 || stats.OK != 1 {
		t.Fatalf("result=%d stats=%+v\n%s", result, stats, out.String())
	}
	requests := log.snapshot()
	if len(requests) != 2 || requests[1].Header.Get("x-client-platform") != "web" {
		t.Fatalf("fallback requests=%+v", requests)
	}
	if !strings.Contains(out.String(), "降级 web 域") {
		t.Errorf("missing fallback log:\n%s", out.String())
	}
}

func TestStringCodeIsNotTreatedAsSuccess(t *testing.T) {
	if isOK(map[string]any{"code": "0"}) {
		t.Fatal("string code \"0\" is not numeric code 0 and must not be accepted")
	}
	if codeZeroOrNil("0") {
		t.Fatal("string code \"0\" must not satisfy the buddy first success check")
	}
}

func TestLotteryNoChanceBoundary(t *testing.T) {
	clock := newFakeClock()
	clock.uuids = []string{"uuid-one", "uuid-two"}
	draws := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/school/portal/activity/school/config":
			writeJSONResponse(w, 200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"in_period": true,
					"chance": map[string]any{
						"balance": 2, "total_earned": 2, "lottery_limit": 10,
					},
				},
			})
		case "/school/portal/activity/school/wheel/draw":
			draws++
			if draws == 1 {
				writeJSONResponse(w, 200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"prize_code": "school_credit_6", "credit_amount": 6, "chance_balance": 1,
					},
				})
			} else {
				writeJSONResponse(w, 409, map[string]any{"code": 40900, "message": "no chance"})
			}
		default:
			http.NotFound(w, r)
		}
	})
	service, _ := testService(t, handler, clock)
	var out strings.Builder
	var stats Stats
	service.lotteryAccount(context.Background(), testAccount(),
		SchoolOptions{Yes: true, LotteryOnly: true, Gap: 1}, &stats, true, &out)
	if draws != 2 {
		t.Fatalf("draws=%d want 2", draws)
	}
	if stats.Accounts != 1 || stats.OK != 1 || stats.Fail != 0 {
		t.Fatalf("stats=%+v\n%s", stats, out.String())
	}
	if !strings.Contains(out.String(), "40900") || !strings.Contains(out.String(), "6积分") {
		t.Errorf("missing boundary/prize output:\n%s", out.String())
	}
}
