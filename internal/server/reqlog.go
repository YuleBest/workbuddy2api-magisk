// reqlog.go 进程内请求日志环形缓冲：保留最近若干条 chat 请求记录供管理后台读取。
//
// 与 stdout 表格日志同一数据源（logChatRow 同时落盘与入环），区别是这里可被
// HTTP 读走，进程重启清零；不作为审计日志，只作实时观测。
package server

import (
	"sync"
	"time"
)

// reqLogCap 环形缓冲容量。按当前用量（每天数百请求）足够回看数日。
const reqLogCap = 500

// RequestRecord 一次 /v1/chat/completions 请求的观测记录（脱敏：uid 只留前 8 位）。
type RequestRecord struct {
	Seq     int64     `json:"seq"`
	Time    time.Time `json:"time"`
	Model   string    `json:"model"`
	Mode    string    `json:"mode"` // "stream" | "sync"
	UID     string    `json:"uid"`
	Nick    string    `json:"nick,omitempty"` // 账号昵称（与 stdout 日志同一来源）
	Status  int       `json:"status"`
	TTFBMS  int64     `json:"ttfb_ms"` // 0 = 无首帧（非流式/失败）
	Tokens  int       `json:"tokens"`  // -1 = usage 缺失
	TotalMS int64     `json:"total_ms"`
	TokPS   float64   `json:"tok_per_sec"` // 0 = 无法折算
}

// reqLog 环形缓冲（写加锁，容量固定，写满覆盖最旧）。
var reqLog = struct {
	mu   sync.Mutex
	ring []RequestRecord
	next int
	n    int
}{ring: make([]RequestRecord, reqLogCap)}

// appendRequest 追加一条记录（写满后覆盖最旧）。
func appendRequest(rec RequestRecord) {
	reqLog.mu.Lock()
	defer reqLog.mu.Unlock()
	reqLog.ring[reqLog.next] = rec
	reqLog.next = (reqLog.next + 1) % reqLogCap
	if reqLog.n < reqLogCap {
		reqLog.n++
	}
}

// RecentRequests 返回最近 limit 条记录，按时间倒序（最新在前）。limit<=0 或超容量时取全部。
func RecentRequests(limit int) []RequestRecord {
	reqLog.mu.Lock()
	defer reqLog.mu.Unlock()
	if limit <= 0 || limit > reqLog.n {
		limit = reqLog.n
	}
	out := make([]RequestRecord, 0, limit)
	for i := 0; i < limit; i++ {
		// 从最新一条往回取：next-1 是最新写入位。
		idx := (reqLog.next - 1 - i + reqLogCap*2) % reqLogCap
		out = append(out, reqLog.ring[idx])
	}
	return out
}

// TotalRequests 进程启动以来处理的 chat 请求总数（含失败）。
func TotalRequests() int64 { return chatSeq.Load() }

// RequestLogCapacity 环形缓冲容量（前端展示"最近 N 条"用）。
func RequestLogCapacity() int { return reqLogCap }
