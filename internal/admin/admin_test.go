package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"workbuddy2api/internal/auth"
	"workbuddy2api/internal/pool"
	"workbuddy2api/internal/scheduler"
	"workbuddy2api/internal/server"
	"workbuddy2api/internal/upstream"
)

func newTestHandler(t *testing.T, token string) (*Handler, *pool.Pool, *server.KeyStore) {
	t.Helper()
	dir := t.TempDir()
	ks := server.NewKeyStore(filepath.Join(dir, "keys.json"), "")
	p := pool.New("") // 无 state_file：纯内存，测试间互不干扰
	sch := scheduler.New(scheduler.Config{Pool: p, Upstream: upstream.New()})
	h := NewHandler(Config{
		Pool:        p,
		Scheduler:   sch,
		Keys:        ks,
		Models:      func() []map[string]any { return nil },
		Token:       token,
		ConfigDoc:   map[string]any{"listen": ":7863"},
		Schedule:    ScheduleView{Checkin: TaskView{Enabled: true, Hours: []int{9, 21}}},
		Version:     "test",
		Listen:      ":7863",
		RedisMode:   "noop",
		PromptMode:  "passthrough",
		StickyCount: func() int { return 7 },
		Degraded:    func() bool { return false },
		Started:     time.Now().Add(-time.Minute),
	})
	return h, p, ks
}

// do 发一个请求（token 为空表示不带 Authorization 头）。
func do(t *testing.T, h *Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *strings.Reader
	if body == "" {
		rdr = strings.NewReader("")
	} else {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON: %v — %s", err, rec.Body.String())
	}
	return out
}

// TestAdminAuthGate 管理 API 必须挡住无 token / 错 token，静态页面不设卡。
func TestAdminAuthGate(t *testing.T) {
	h, _, _ := newTestHandler(t, "s3cret")
	cases := []struct {
		name   string
		token  string
		status int
	}{
		{"无 token", "", http.StatusUnauthorized},
		{"错 token", "nope", http.StatusUnauthorized},
		{"对 token", "s3cret", http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, "/api/admin/session", c.token, "")
			if rec.Code != c.status {
				t.Fatalf("status = %d, want %d", rec.Code, c.status)
			}
		})
	}
	// 静态资源（登录页本身）不鉴权，否则登录页永远打不开。
	rec := do(t, h, http.MethodGet, "/admin/", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("GET /admin/ = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "<") {
		t.Errorf("GET /admin/ 应返回 HTML，得到 %q", rec.Body.String())
	}
	// SPA history 路由回退到同一个 index.html。
	rec = do(t, h, http.MethodGet, "/admin/accounts", "", "")
	if rec.Code != http.StatusOK {
		t.Errorf("GET /admin/accounts = %d, want 200（SPA 回退）", rec.Code)
	}
	rec = do(t, h, http.MethodGet, "/admin", "", "")
	if rec.Code != http.StatusMovedPermanently {
		t.Errorf("GET /admin = %d, want 301", rec.Code)
	}
}

// TestAdminNoTokenMeansOpen 未配置 token 时不鉴权（与网关"留空=不鉴权"一致）。
func TestAdminNoTokenMeansOpen(t *testing.T) {
	h, _, _ := newTestHandler(t, "")
	rec := do(t, h, http.MethodGet, "/api/admin/session", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := decode(t, rec)["auth_required"]; got != false {
		t.Errorf("auth_required = %v, want false", got)
	}
}

// TestAdminOverviewShape 仪表盘依赖的字段必须都在。
func TestAdminOverviewShape(t *testing.T) {
	h, p, _ := newTestHandler(t, "t")
	p.Add(&auth.Auth{UID: "uid-1", Nickname: "猫咪"})
	p.Add(&auth.Auth{UID: "uid-2"})
	p.SetCredits("uid-1", 1200)
	p.Disable("uid-2", "12153 session dead")

	rec := do(t, h, http.MethodGet, "/api/admin/overview", "t", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	out := decode(t, rec)
	pl, ok := out["pool"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 pool 段：%v", out)
	}
	if pl["total"].(float64) != 2 || pl["disabled"].(float64) != 1 {
		t.Errorf("池计数不符：%v", pl)
	}
	accts, _ := out["accounts"].([]any)
	if len(accts) != 2 {
		t.Fatalf("accounts 长度 = %d, want 2", len(accts))
	}
	if out["sticky_sessions"].(float64) != 7 {
		t.Errorf("sticky_sessions = %v, want 7", out["sticky_sessions"])
	}
	if _, ok := out["requests"].(map[string]any); !ok {
		t.Error("缺少 requests 统计段")
	}
	sched, ok := out["schedule"].(map[string]any)
	if !ok {
		t.Fatal("缺少 schedule 段")
	}
	if sched["checkin"].(map[string]any)["enabled"] != true {
		t.Errorf("schedule.checkin.enabled 应为 true：%v", sched)
	}
}

// TestAdminAccountsDetail 账号明细要带 token 过期与成本账本。
func TestAdminAccountsDetail(t *testing.T) {
	h, p, _ := newTestHandler(t, "t")
	a := &auth.Auth{UID: "uid-1", Nickname: "猫咪", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	p.Add(a)
	p.NoteModelCost("uid-1", "deepseek-v4-flash", 0, 1000) // 免费模型：单价 0
	p.NoteModelCost("uid-1", "glm-5.2", 2, 1000)           // 2 credit / 1k token

	rec := do(t, h, http.MethodGet, "/api/admin/accounts", "t", "")
	out := decode(t, rec)
	accts := out["accounts"].([]any)
	first := accts[0].(map[string]any)
	if first["uid"] != "uid-1" || first["credits"].(float64) != 0 {
		t.Errorf("账号基本字段不符：%v", first)
	}
	if _, ok := first["expires_at"]; !ok {
		t.Error("缺少 expires_at")
	}
	// 成本台账由 pool.Status 透出（上游字段），后台只做展示
	costs, _ := first["model_costs"].([]any)
	if len(costs) != 2 {
		t.Fatalf("成本台账应有 2 条，得到 %d", len(costs))
	}
}

// TestAdminAccountActions 禁用/启用/清冷却，以及不存在账号的 404。
func TestAdminAccountActions(t *testing.T) {
	h, p, _ := newTestHandler(t, "t")
	p.Add(&auth.Auth{UID: "uid-1"})
	p.Cooldown("uid-1", pool.CoolSoft, time.Hour, "429 rate limit")

	rec := do(t, h, http.MethodPost, "/api/admin/accounts/uid-1/clear-cooldown", "t", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("clear-cooldown = %d: %s", rec.Code, rec.Body.String())
	}
	if st, _ := p.Status("uid-1"); st.Cooling {
		t.Error("清冷却后不应仍在冷却")
	}

	// 停用走 manual_disabled（与系统自动禁用 disabled 是两个独立位）
	rec = do(t, h, http.MethodPost, "/api/admin/accounts/uid-1/disable", "t", `{"reason":"手动排查"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable = %d: %s", rec.Code, rec.Body.String())
	}
	out := decode(t, rec)
	if out["manual_disabled"] != true || out["manual_reason"] != "手动排查" {
		t.Errorf("停用回执不符：%v", out)
	}
	st, _ := p.Status("uid-1")
	if !st.ManualDisabled || st.ManualReason != "手动排查" {
		t.Errorf("手动停用状态不符：%+v", st)
	}
	if st.Disabled {
		t.Error("手动停用不应污染系统自动禁用位")
	}

	rec = do(t, h, http.MethodPost, "/api/admin/accounts/uid-1/enable", "t", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("enable = %d: %s", rec.Code, rec.Body.String())
	}
	if st, _ := p.Status("uid-1"); st.ManualDisabled {
		t.Error("恢复后不应仍手动停用")
	}

	// revive 解系统自动禁用位，不动手动位
	p.Disable("uid-1", "12153 session dead")
	rec = do(t, h, http.MethodPost, "/api/admin/accounts/uid-1/revive", "t", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("revive = %d: %s", rec.Code, rec.Body.String())
	}
	if st, _ := p.Status("uid-1"); st.Disabled {
		t.Error("revive 后系统禁用位应清除")
	}

	rec = do(t, h, http.MethodPost, "/api/admin/accounts/nope/disable", "t", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("未知账号 = %d, want 404", rec.Code)
	}
}

// TestAdminKeysLifecycle 建 key → 列表掩码 → 新 key 立即能过网关鉴权 → 吊销。
func TestAdminKeysLifecycle(t *testing.T) {
	h, _, ks := newTestHandler(t, "t")

	rec := do(t, h, http.MethodPost, "/api/admin/keys", "t", `{"note":"手机"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	created := decode(t, rec)["key"].(string)
	if !ks.Valid(created) {
		t.Error("新建的 key 应立即可用于网关鉴权")
	}

	rec = do(t, h, http.MethodGet, "/api/admin/keys", "t", "")
	list := decode(t, rec)["keys"].([]any)
	if len(list) != 1 {
		t.Fatalf("keys 长度 = %d, want 1", len(list))
	}
	entry := list[0].(map[string]any)
	if strings.Contains(entry["masked"].(string), created[10:20]) {
		t.Errorf("列表不得泄漏完整 key：%v", entry)
	}
	if entry["note"] != "手机" {
		t.Errorf("备注丢失：%v", entry)
	}

	prefix := entry["prefix"].(string)
	rec = do(t, h, http.MethodDelete, "/api/admin/keys/"+prefix, "t", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("revoke = %d: %s", rec.Code, rec.Body.String())
	}
	if ks.Valid(created) {
		t.Error("吊销后 key 应立即失效")
	}
	if got := decode(t, rec)["remaining"].(float64); got != 0 {
		t.Errorf("remaining = %v, want 0", got)
	}

	// 重复吊销 → 400（没有匹配），而不是 500。
	rec = do(t, h, http.MethodDelete, "/api/admin/keys/"+prefix, "t", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("重复吊销 = %d, want 400", rec.Code)
	}
}

// TestAdminTaskTrigger 未知名 404、已知任务 202，且同一任务并发触发被拒。
func TestAdminTaskTrigger(t *testing.T) {
	h, _, _ := newTestHandler(t, "t")

	rec := do(t, h, http.MethodPost, "/api/admin/tasks/nope", "t", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("未知任务 = %d, want 404", rec.Code)
	}

	// 空池上的 keepalive 是空操作，可安全触发。
	rec = do(t, h, http.MethodPost, "/api/admin/tasks/keepalive", "t", "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("keepalive = %d: %s", rec.Code, rec.Body.String())
	}

	// 手动占用名额模拟"正在跑"，验证互斥分支。
	if !h.beginTask("checkin") {
		t.Fatal("首次 beginTask 应为 true")
	}
	rec = do(t, h, http.MethodPost, "/api/admin/tasks/checkin", "t", "")
	if rec.Code != http.StatusConflict {
		t.Errorf("任务执行中 = %d, want 409", rec.Code)
	}
	h.endTask("checkin")
	if !h.beginTask("checkin") {
		t.Error("任务结束后应可再次触发")
	}
	h.endTask("checkin")
}

// TestAdminLogsAndModels 日志接口字段与模型接口透传。
func TestAdminLogsAndModels(t *testing.T) {
	h, _, _ := newTestHandler(t, "t")
	rec := do(t, h, http.MethodGet, "/api/admin/logs?limit=5", "t", "")
	out := decode(t, rec)
	if _, ok := out["records"]; !ok {
		t.Errorf("缺少 records：%v", out)
	}
	if out["limit"].(float64) != 5 {
		t.Errorf("limit = %v, want 5", out["limit"])
	}
	if out["capacity"].(float64) <= 0 {
		t.Errorf("capacity 应 > 0：%v", out["capacity"])
	}

	rec = do(t, h, http.MethodGet, "/api/admin/models", "t", "")
	if _, ok := decode(t, rec)["models"]; !ok {
		t.Errorf("缺少 models：%s", rec.Body.String())
	}
}

// TestAdminConfigRedacted 设置页不得回传明文密钥（ConfigDoc 由调用方脱敏，这里守住接口形状）。
func TestAdminConfigRedacted(t *testing.T) {
	h, _, _ := newTestHandler(t, "t")
	rec := do(t, h, http.MethodGet, "/api/admin/config", "t", "")
	body := rec.Body.String()
	if strings.Contains(body, "s3cret") || strings.Contains(body, "api_key\":\"sk-") {
		t.Errorf("配置响应疑似泄漏密钥：%s", body)
	}
	out := decode(t, rec)
	if _, ok := out["config"].(map[string]any); !ok {
		t.Errorf("缺少 config 段：%v", out)
	}
}

// TestAdminCheckinSync 同步签到：空池上也走完整路径（无上游调用），回执结构必须完整。
func TestAdminCheckinSync(t *testing.T) {
	h, _, _ := newTestHandler(t, "t")
	rec := do(t, h, http.MethodPost, "/api/admin/tasks/checkin?wait=1", "t", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	out := decode(t, rec)
	if out["waited"] != true {
		t.Errorf("waited = %v, want true", out["waited"])
	}
	if _, ok := out["outcomes"]; !ok {
		t.Error("同步签到必须回传逐号回执（outcomes）")
	}
	sum, ok := out["summary"].(map[string]any)
	if !ok {
		t.Fatalf("缺少 summary：%v", out)
	}
	if sum["total"].(float64) != 0 {
		t.Errorf("空池 total = %v, want 0", sum["total"])
	}
	if msg, _ := out["message"].(string); !strings.Contains(msg, "签到完成") {
		t.Errorf("message = %q, 应包含可读回执", msg)
	}
}

// TestAdminTaskAsyncStillAccepted 非签到任务仍走异步 202（同步等会把隧道撑超时）。
func TestAdminTaskAsyncStillAccepted(t *testing.T) {
	h, _, _ := newTestHandler(t, "t")
	for _, task := range []string{"travel", "activity", "keepalive"} {
		rec := do(t, h, http.MethodPost, "/api/admin/tasks/"+task+"?wait=1", "t", "")
		if rec.Code != http.StatusAccepted {
			t.Errorf("%s = %d, want 202（wait 只对 checkin 生效）", task, rec.Code)
		}
	}
}

// TestCheckinMessageCounts 回执文案按各状态拼装，失败要能一眼看出来。
func TestCheckinMessageCounts(t *testing.T) {
	cases := []struct {
		ok, already, fail, skipped int
		want                       string
	}{
		{2, 0, 0, 0, "签到完成：2 个成功"},
		{1, 1, 0, 1, "签到完成：1 个成功，1 个今天已签到，1 个跳过（已禁用或无凭证）"},
		{0, 0, 2, 0, "签到完成：0 个成功，2 个失败"},
	}
	for _, c := range cases {
		if got := checkinMessage(c.ok, c.already, c.fail, c.skipped); got != c.want {
			t.Errorf("checkinMessage(%d,%d,%d,%d) = %q, want %q", c.ok, c.already, c.fail, c.skipped, got, c.want)
		}
	}
}
