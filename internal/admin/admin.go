// Package admin 网关管理后台：观测 API + 运维动作 + 内嵌 Vue SPA。
//
// 定位：把 wbapi CLI 的 status/accounts/quota/key/log 能力搬到浏览器里，只做
// 「看」与少量「动」（禁用/启用账号、清冷却、触发定时任务、增删 key）。
// 不改网关数据面行为：所有动作都调用 pool/scheduler/keystore 已有入口。
//
// 鉴权：Bearer token（config 的 admin.token → keys.json 首个 key → api_key），
// 与网关 chat 接口的"留空=不鉴权"语义保持一致。静态资源不鉴权（页面本身无秘密），
// 数据全部走 /api/admin/*。
package admin

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"workbuddy2api/internal/pool"
	"workbuddy2api/internal/scheduler"
	"workbuddy2api/internal/server"
)

// Config 管理后台依赖。
type Config struct {
	Pool      *pool.Pool
	Scheduler *scheduler.Scheduler
	Keys      *server.KeyStore
	// Chat 数据面 handler：面板对话经它走完整链路（选号 / 错误策略 / 请求日志），
	// admin 只把 admin token 换成数据面 key。nil = 未接入，对话接口回 503。
	Chat http.Handler
	// Models 返回网关模型列表（含动态拉取与静态回退），来自 gateway handler。
	Models func() []map[string]any
	// Token 管理 token；空 = 不鉴权（此时与网关同为完全开放语义）。
	Token string
	// ConfigDoc 脱敏后的生效配置快照（由 cmd/server 组装，admin 不解析配置结构）。
	ConfigDoc any
	// Schedule 排程概览（同由 cmd/server 映射，避免 admin 反向解析配置键名）。
	Schedule ScheduleView
	Version  string
	Listen   string
	// RedisMode / PromptMode / StickyCount / Degraded 为网关运行态观测字段。
	RedisMode   string
	PromptMode  string
	StickyCount func() int
	Degraded    func() bool
	// Started 进程启动时刻。
	Started time.Time
}

// ScheduleView 六类定时任务的开关与触发时点（时点即配置里的整点，非运行时推算）。
type ScheduleView struct {
	Checkin   TaskView `json:"checkin"`
	Travel    TaskView `json:"travel"`
	Activity  TaskView `json:"activity"`
	Keepalive TaskView `json:"keepalive"`
	School    TaskView `json:"school"`
	Cat       TaskView `json:"cat"`
}

// TaskView 单个任务的排程配置。
type TaskView struct {
	Enabled bool  `json:"enabled"`
	Hours   []int `json:"hours"`
}

// Handler 管理后台路由。
type Handler struct {
	cfg Config
	mux *http.ServeMux

	// 手动任务的互斥：同一任务在前一次未结束时拒绝再次触发（避免重复打上游）。
	taskMu  sync.Mutex
	running map[string]bool
}

// NewHandler 构建路由。
func NewHandler(cfg Config) *Handler {
	h := &Handler{cfg: cfg, mux: http.NewServeMux(), running: map[string]bool{}}
	// 管理 API：全部要求 Bearer token。
	h.mux.HandleFunc("GET /api/admin/session", h.auth(h.session))
	h.mux.HandleFunc("GET /api/admin/overview", h.auth(h.overview))
	h.mux.HandleFunc("GET /api/admin/accounts", h.auth(h.accounts))
	h.mux.HandleFunc("POST /api/admin/accounts/{uid}/disable", h.auth(h.accountDisable))
	h.mux.HandleFunc("POST /api/admin/accounts/{uid}/enable", h.auth(h.accountEnable))
	h.mux.HandleFunc("POST /api/admin/accounts/{uid}/revive", h.auth(h.accountRevive))
	h.mux.HandleFunc("POST /api/admin/accounts/{uid}/clear-cooldown", h.auth(h.accountClearCooldown))
	h.mux.HandleFunc("GET /api/admin/keys", h.auth(h.keysList))
	h.mux.HandleFunc("POST /api/admin/keys", h.auth(h.keysCreate))
	h.mux.HandleFunc("DELETE /api/admin/keys/{prefix}", h.auth(h.keysRevoke))
	h.mux.HandleFunc("GET /api/admin/logs", h.auth(h.logs))
	h.mux.HandleFunc("GET /api/admin/models", h.auth(h.models))
	h.mux.HandleFunc("GET /api/admin/config", h.auth(h.config))
	h.mux.HandleFunc("POST /api/admin/tasks/{task}", h.auth(h.runTask))
	// 面板对话：OpenAI 兼容面（/v1 前缀），供前端 openai SDK 直连。与上面的 REST
	// 端点分开，是因为那些端点返回各自的信封（如 /models 的 {models:[...]}），
	// 混在同一前缀下会让 SDK 按 OpenAI 格式解析而误判。
	h.mux.HandleFunc("POST /api/admin/v1/chat/completions", h.auth(h.chatCompletions))
	// 静态资源：SPA 本体，不鉴权。
	h.mux.HandleFunc("GET /admin", h.redirectIndex)
	h.mux.HandleFunc("GET /admin/", h.serveSPA)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

// AuthRequired 报告管理 API 是否需要 token。
func (h *Handler) AuthRequired() bool { return h.cfg.Token != "" }

// auth 校验 Bearer token（token 为空时不鉴权，与网关语义一致）。
func (h *Handler) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.cfg.Token != "" {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !constTimeEqual(got, h.cfg.Token) {
				writeErr(w, http.StatusUnauthorized, "invalid_admin_token", "管理 token 无效或缺失")
				return
			}
		}
		next(w, r)
	}
}

// session 校验当前 token 并返回身份信息（登录页用它判断 token 是否可用）。
func (h *Handler) session(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"service":       server.ServiceName,
		"version":       h.cfg.Version,
		"auth_required": h.AuthRequired(),
		"now":           time.Now(),
	})
}

// overview 仪表盘总览：池状态 + 请求统计 + 排程概览。
func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	total, healthy, cooling, disabled, inFlightFull := h.cfg.Pool.CountsDetailed()
	records := server.RecentRequests(0)

	// 池内账号的紧凑视图（供仪表盘的账号一览条）。
	statuses := h.cfg.Pool.List()
	brief := make([]map[string]any, 0, len(statuses))
	inFlight := 0
	for _, st := range statuses {
		inFlight += st.InFlight
		brief = append(brief, map[string]any{
			"uid":            st.UID,
			"nickname":       st.Nickname,
			"state":          accountState(st),
			"credits":        st.Credits,
			"cool_remaining": st.CoolRemaining,
			"reason":         reasonOf(st),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"service":    server.ServiceName,
		"version":    h.cfg.Version,
		"started_at": h.cfg.Started,
		"uptime_sec": int64(time.Since(h.cfg.Started).Seconds()),
		"listen":     h.cfg.Listen,
		"now":        time.Now(),
		"pool": map[string]any{
			"total":          total,
			"healthy":        healthy,
			"cooling":        cooling,
			"disabled":       disabled,
			"in_flight":      inFlight,
			"in_flight_full": inFlightFull,
			"servable":       h.cfg.Pool.ServableNow(),
		},
		"accounts":        brief,
		"requests":        requestStats(records),
		"sticky_sessions": h.sticky(),
		"redis_mode":      h.cfg.RedisMode,
		"keys_count":      h.cfg.Keys.Count(),
		"prompt_mode":     h.cfg.PromptMode,
		"degraded":        h.degraded(),
		"schedule":        h.cfg.Schedule,
	})
}

// accounts 账号池明细：池状态 + token 过期 + 设备 token + 成本账本。
func (h *Handler) accounts(w http.ResponseWriter, r *http.Request) {
	statuses := h.cfg.Pool.List()
	out := make([]accountView, 0, len(statuses))
	for _, st := range statuses {
		v := accountView{Status: st}
		if a := h.cfg.Pool.AuthByUID(st.UID); a != nil {
			expiresAt, hasDevice := a.ExpiryInfo()
			if expiresAt > 0 {
				v.ExpiresAt = time.Unix(expiresAt, 0)
				v.ExpiresInSec = int64(time.Until(v.ExpiresAt).Seconds())
			}
			v.HasDeviceToken = hasDevice
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": out, "now": time.Now()})
}

// accountView 账号视图：内嵌池状态（字段内联）+ 管理后台补充字段。
// 积分/冷却/熔断/成本台账（model_costs）/双位停用（manual_disabled + disabled）
// 都在 pool.Status 里，这里只补凭证侧的过期时间与设备 token 有无。
type accountView struct {
	pool.Status
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	ExpiresInSec   int64     `json:"expires_in_sec,omitempty"`
	HasDeviceToken bool      `json:"has_device_token"`
}

// accountStateView 动作响应体：回显操作后的双位状态，面板据此直接更新 UI。
// 字段名与上游 /admin/accounts/{uid}/* 端点的 adminState 保持一致（同一套语义，
// 两个入口只是给不同客户端用：CLI 走 /admin/*，Web 后台走 /api/admin/*）。
type accountStateView struct {
	UID            string `json:"uid"`
	ManualDisabled bool   `json:"manual_disabled"`
	ManualReason   string `json:"manual_reason,omitempty"`
	// Disabled 系统自动禁用位（12153/连败等）。与手动位独立：enable 只解手动位，
	// 系统位要靠 revive——面板据此区分「我摘的」与「系统判坏的」。
	Disabled bool `json:"disabled"`
	Changed  bool `json:"changed"`
}

// accountDisable 手动停用（manual_disabled）：把账号摘出选号池但保留在池里。
// 语义是「对话流量摘除」而非「账号冻结」——不碰冷却/熔断，签到与保活照常执行，
// 凭证和积分都是活的；与系统自动禁用（disabled）是两个独立状态位。
func (h *Handler) accountDisable(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("uid")
	var body struct {
		Reason string `json:"reason"`
	}
	_ = jsonDecode(r, &body)
	reason := strings.TrimSpace(body.Reason)
	if reason == "" {
		reason = "manual"
	}
	found, changed := h.cfg.Pool.SetManualDisabled(uid, true, reason)
	if !found {
		writeErr(w, http.StatusNotFound, "unknown_account", "账号不存在")
		return
	}
	log.Printf("admin: manual disable %s (%s) changed=%v", uidPrefix(uid), reason, changed)
	h.writeAccountState(w, uid, changed)
}

// accountEnable 解除手动停用。系统自动禁用位不受影响：仍 disabled 的账号需要 revive
// 才回到选号池（响应里的 disabled 字段就是给面板看的提示）。
func (h *Handler) accountEnable(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("uid")
	found, changed := h.cfg.Pool.SetManualDisabled(uid, false, "")
	if !found {
		writeErr(w, http.StatusNotFound, "unknown_account", "账号不存在")
		return
	}
	log.Printf("admin: manual enable %s changed=%v", uidPrefix(uid), changed)
	h.writeAccountState(w, uid, changed)
}

// accountRevive 解除系统自动禁用（清 disabled + reason + 连续 12153 计数）。
// 不碰手动停用位：运维明确摘除的号不该被一次 revive 悄悄放回池子。
func (h *Handler) accountRevive(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("uid")
	if _, _, found := h.cfg.Pool.ManualDisabledState(uid); !found {
		writeErr(w, http.StatusNotFound, "unknown_account", "账号不存在")
		return
	}
	changed := h.cfg.Pool.ReviveDisabled(uid)
	log.Printf("admin: revive %s changed=%v", uidPrefix(uid), changed)
	h.writeAccountState(w, uid, changed)
}

// writeAccountState 回显操作后的双位状态（与上游 adminState 同形）。
func (h *Handler) writeAccountState(w http.ResponseWriter, uid string, changed bool) {
	manual, reason, _ := h.cfg.Pool.ManualDisabledState(uid)
	auto := false
	for _, st := range h.cfg.Pool.List() {
		if st.UID == uid {
			auto = st.Disabled
			break
		}
	}
	writeJSON(w, http.StatusOK, accountStateView{
		UID: uid, ManualDisabled: manual, ManualReason: reason, Disabled: auto, Changed: changed,
	})
}

// logs 最近请求日志（环形缓冲，最新在前）。
func (h *Handler) logs(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"records":  server.RecentRequests(limit),
		"total":    server.TotalRequests(),
		"limit":    limit,
		"capacity": server.RequestLogCapacity(),
	})
}

func (h *Handler) models(w http.ResponseWriter, r *http.Request) {
	models := []map[string]any{}
	if h.cfg.Models != nil {
		models = h.cfg.Models()
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (h *Handler) config(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"config":          h.cfg.ConfigDoc,
		"service":         server.ServiceName,
		"version":         h.cfg.Version,
		"keys_file":       h.cfg.Keys.Path(),
		"auth_required":   h.AuthRequired(),
		"listen":          h.cfg.Listen,
		"redis_mode":      h.cfg.RedisMode,
		"prompt_mode":     h.cfg.PromptMode,
		"started_at":      h.cfg.Started,
		"sticky_sessions": h.sticky(),
	})
}

// runTask 手动触发定时任务。
//
// 支持 checkin / activity / travel / keepalive / school / cat 六类任务。
// 默认异步：立刻回 202，结果写服务日志（旅行/活跃上报按账号限速、逐条上报，
// 同步等可能到分钟级，公网经隧道会先超时）。
//
// checkin 例外：`?wait=1` 时同步等待并回传逐号回执。签到每号只做"刷新 + 签到 +
// 查余额"三次短请求，秒级完成——把结果直接摆给页面，比让用户去翻 journalctl 有用。
func (h *Handler) runTask(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("task")
	if h.cfg.Scheduler == nil {
		writeErr(w, http.StatusServiceUnavailable, "scheduler_disabled", "调度器未启用")
		return
	}
	if (name == "school" || name == "cat") && os.Getenv("WB2A_PYTHON_TASKS") == "0" {
		writeErr(w, http.StatusNotImplemented, "python_task_disabled", "Android 版不包含该 Python 任务")
		return
	}
	if name == "checkin" && r.URL.Query().Get("wait") == "1" {
		h.runCheckinSync(w)
		return
	}
	var run func()
	var label string
	switch name {
	case "checkin":
		run, label = h.cfg.Scheduler.RunCheckinNow, "签到"
	case "activity":
		run, label = h.cfg.Scheduler.RunActivityNow, "活跃上报"
	case "travel":
		run, label = h.cfg.Scheduler.RunTravelNow, "猫猫旅行"
	case "keepalive":
		run, label = h.cfg.Scheduler.RunKeepaliveNow, "token 保活"
	case "school":
		run, label = h.cfg.Scheduler.RunSchoolNow, "开学季任务"
	case "cat":
		run, label = h.cfg.Scheduler.RunCatNow, "夜猫子任务"
	default:
		writeErr(w, http.StatusNotFound, "unknown_task", "未知任务："+name)
		return
	}
	if !h.beginTask(name) {
		writeErr(w, http.StatusConflict, "task_running", label+"正在执行中，请等它跑完")
		return
	}
	log.Printf("admin: run %s", name)
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "task": name, "message": label + "已触发，结果见服务日志"})
	go func() {
		defer h.endTask(name)
		run()
	}()
}

// checkinWait 同步签到的等待上限。超过就回 202 让任务在后台跑完——上限取值
// 要小于公网隧道（Cloudflare 默认 ~100s）的超时，避免响应被隧道先掐掉。
const checkinWait = 80 * time.Second

// runCheckinSync 同步跑一次全量签到，回传逐号结果（状态 / 签到后余额 / 失败原因）。
func (h *Handler) runCheckinSync(w http.ResponseWriter) {
	if !h.beginTask("checkin") {
		writeErr(w, http.StatusConflict, "task_running", "签到正在执行中，请等它跑完")
		return
	}
	type result struct {
		outcomes []scheduler.CheckinOutcome
		err      error
	}
	done := make(chan result, 1)
	go func() {
		defer h.endTask("checkin")
		out, err := h.cfg.Scheduler.CheckinAll()
		done <- result{outcomes: out, err: err}
	}()
	select {
	case res := <-done:
		if res.err != nil {
			// ErrBusy：定时任务（09/21 点）与手动触发互斥，等它跑完再点。
			writeErr(w, http.StatusConflict, "task_running", "签到正在执行中（定时任务先占了锁），稍后再点")
			return
		}
		ok, already, fail, skipped := countCheckin(res.outcomes)
		log.Printf("admin: checkin waited done total=%d ok=%d already=%d fail=%d skipped=%d",
			len(res.outcomes), ok, already, fail, skipped)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"task":     "checkin",
			"waited":   true,
			"outcomes": res.outcomes,
			"summary": map[string]any{
				"total": len(res.outcomes), "ok": ok, "already": already, "fail": fail, "skipped": skipped,
			},
			"message": checkinMessage(ok, already, fail, skipped),
		})
	case <-time.After(checkinWait):
		writeJSON(w, http.StatusAccepted, map[string]any{
			"ok": true, "task": "checkin", "waited": false,
			"message": "签到还在跑（账号多或上游慢时需要更久），稍后看账号池积分是否变化，或查服务日志",
		})
	}
}

// countCheckin 统计逐号回执（与 scheduler 日志同一口径）。
func countCheckin(outcomes []scheduler.CheckinOutcome) (ok, already, fail, skipped int) {
	for _, oc := range outcomes {
		switch oc.Status {
		case scheduler.CheckinOK:
			ok++
		case scheduler.CheckinAlready:
			already++
		case scheduler.CheckinSkipped:
			skipped++
		default:
			fail++
		}
	}
	return
}

// checkinMessage 生成一句人话回执（页面顶部提示条直接用它）。
func checkinMessage(ok, already, fail, skipped int) string {
	msg := fmt.Sprintf("签到完成：%d 个成功", ok)
	if already > 0 {
		msg += fmt.Sprintf("，%d 个今天已签到", already)
	}
	if skipped > 0 {
		msg += fmt.Sprintf("，%d 个跳过（已禁用或无凭证）", skipped)
	}
	if fail > 0 {
		msg += fmt.Sprintf("，%d 个失败", fail)
	}
	return msg
}

func (h *Handler) beginTask(name string) bool {
	h.taskMu.Lock()
	defer h.taskMu.Unlock()
	if h.running[name] {
		return false
	}
	h.running[name] = true
	return true
}

func (h *Handler) endTask(name string) {
	h.taskMu.Lock()
	defer h.taskMu.Unlock()
	delete(h.running, name)
}

func (h *Handler) sticky() int {
	if h.cfg.StickyCount != nil {
		return h.cfg.StickyCount()
	}
	return 0
}

func (h *Handler) degraded() bool {
	if h.cfg.Degraded != nil {
		return h.cfg.Degraded()
	}
	return false
}

// accountState 把池状态归一到三态字符串（前端据此上色）。
func accountState(st pool.Status) string {
	switch {
	case st.Disabled:
		return "disabled"
	case st.Cooling:
		return "cooling"
	default:
		return "healthy"
	}
}

// reasonOf 取冷却/禁用原因（无则空串）。
func reasonOf(st pool.Status) string {
	if st.Disabled {
		return st.DisabledReason
	}
	if st.Cooling {
		return st.Reason
	}
	return ""
}

// modelCount 单个模型的请求次数（仪表盘模型分布用）。
type modelCount struct {
	Model string `json:"model"`
	Count int    `json:"count"`
}

// requestStats 从请求环形缓冲汇总统计（口径：缓冲窗口内的最近若干条）。
func requestStats(records []server.RequestRecord) map[string]any {
	var okN, errN, ttfbSum, tokSum, tokN int
	perModel := map[string]int{}
	lastHour := 0
	cutoff := time.Now().Add(-time.Hour)
	for _, rec := range records {
		if rec.Status > 0 && rec.Status < 400 {
			okN++
		} else {
			errN++
		}
		if rec.TTFBMS > 0 {
			ttfbSum += int(rec.TTFBMS)
		}
		if rec.Tokens > 0 {
			tokSum += rec.Tokens
			tokN++
		}
		perModel[rec.Model]++
		if rec.Time.After(cutoff) {
			lastHour++
		}
	}
	models := make([]modelCount, 0, len(perModel))
	for m, c := range perModel {
		models = append(models, modelCount{Model: m, Count: c})
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Count != models[j].Count {
			return models[i].Count > models[j].Count
		}
		return models[i].Model < models[j].Model
	})
	avgTTFB := 0
	if n := okN + errN; n > 0 {
		avgTTFB = ttfbSum / n
	}
	avgTokens := 0
	if tokN > 0 {
		avgTokens = tokSum / tokN
	}
	return map[string]any{
		"total":       server.TotalRequests(),
		"window":      len(records),
		"capacity":    server.RequestLogCapacity(),
		"ok":          okN,
		"error":       errN,
		"avg_ttfb_ms": avgTTFB,
		"avg_tokens":  avgTokens,
		"last_hour":   lastHour,
		"models":      models,
	}
}

// uidPrefix 只保留 uid 前 8 位（日志脱敏）。
func uidPrefix(uid string) string {
	if len(uid) > 8 {
		return uid[:8]
	}
	return uid
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "encode response failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": msg}})
}

// constTimeEqual 常量时间比较（token 比对不泄漏长度/前缀信息）。
func constTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// jsonDecode 解请求体 JSON；空体不算错误（可选字段场景）。
func jsonDecode(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// accountClearCooldown 人工清冷却：清 until/coolKind/reason/softStreak，不动熔断器与
// credits（语义同签到解冻）。上游没有对应端点，是后台独有的"确认上游已放行，立刻重试"入口。
func (h *Handler) accountClearCooldown(w http.ResponseWriter, r *http.Request) {
	uid := r.PathValue("uid")
	if _, ok := h.cfg.Pool.Status(uid); !ok {
		writeErr(w, http.StatusNotFound, "unknown_account", "账号不存在")
		return
	}
	h.cfg.Pool.ClearCooldown(uid)
	log.Printf("admin: clear cooldown %s", uidPrefix(uid))
	h.writeAccountState(w, uid, true)
}
