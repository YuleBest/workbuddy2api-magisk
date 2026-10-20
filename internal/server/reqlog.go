// reqlog.go 请求日志环形缓冲：保留最近若干条 chat 请求记录供管理后台读取。
//
// 与 stdout 表格日志同一数据源（logChatRow 同时入环）。环形窗口会原子写入
// state.json 同目录，服务重启后恢复最近 500 条；它仍是短期观测窗口，不替代 journald。
package server

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// reqLogCap 环形缓冲容量。按当前用量（每天数百请求）足够回看数日。
const reqLogCap = 500

// RequestRecord 一次 /v1/chat/completions 请求的观测记录（脱敏：uid 只留前 8 位）。
type RequestRecord struct {
	Seq              int64     `json:"seq"`
	Time             time.Time `json:"time"`
	Model            string    `json:"model"`
	Mode             string    `json:"mode"` // "stream" | "sync"
	UID              string    `json:"uid"`
	Nick             string    `json:"nick,omitempty"` // 账号昵称（与 stdout 日志同一来源）
	Status           int       `json:"status"`
	TTFBMS           int64     `json:"ttfb_ms"`           // 0 = 无首帧（非流式/失败）
	PromptTokens     int       `json:"prompt_tokens"`     // -1 = usage 缺失
	CompletionTokens int       `json:"completion_tokens"` // -1 = usage 缺失
	Tokens           int       `json:"tokens"`            // completion token 的兼容别名；-1 = usage 缺失
	TotalMS          int64     `json:"total_ms"`
	TokPS            float64   `json:"tok_per_sec"` // 0 = 无法折算
}

// reqLog 环形缓冲（写加锁，容量固定，写满覆盖最旧）。
var reqLog = struct {
	mu   sync.Mutex
	ring []RequestRecord
	next int
	n    int
	path string
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
	persistRequestLogLocked()
}

// SetRequestLogPath 设置持久化文件并加载最近窗口。空路径 = 仅内存（测试/首次装配）。
func SetRequestLogPath(path string) {
	reqLog.mu.Lock()
	defer reqLog.mu.Unlock()

	reqLog.ring = make([]RequestRecord, reqLogCap)
	reqLog.next = 0
	reqLog.n = 0
	reqLog.path = path
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("WARN: [reqlog] 恢复失败 path=%s: %v", path, err)
		}
		return
	}
	var records []RequestRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		log.Printf("WARN: [reqlog] 解析失败 path=%s: %v", path, err)
		return
	}
	if len(records) > reqLogCap {
		records = records[len(records)-reqLogCap:]
	}
	var maxSeq int64
	for _, rec := range records {
		reqLog.ring[reqLog.next] = rec
		reqLog.next = (reqLog.next + 1) % reqLogCap
		reqLog.n++
		if rec.Seq > maxSeq {
			maxSeq = rec.Seq
		}
	}
	if current := chatSeq.Load(); maxSeq > current {
		chatSeq.Store(maxSeq)
	}
}

// persistRequestLogLocked 原子写整个 500 条窗口。调用方必须持有 reqLog.mu。
func persistRequestLogLocked() {
	if reqLog.path == "" || reqLog.n == 0 {
		return
	}
	records := make([]RequestRecord, 0, reqLog.n)
	start := (reqLog.next - reqLog.n + reqLogCap) % reqLogCap
	for i := 0; i < reqLog.n; i++ {
		records = append(records, reqLog.ring[(start+i)%reqLogCap])
	}
	raw, err := json.Marshal(records)
	if err != nil {
		log.Printf("WARN: [reqlog] 编码失败 path=%s: %v", reqLog.path, err)
		return
	}
	dir := filepath.Dir(reqLog.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("WARN: [reqlog] 创建目录失败 path=%s: %v", dir, err)
		return
	}
	file, err := os.CreateTemp(dir, ".requests-*.tmp")
	if err != nil {
		log.Printf("WARN: [reqlog] 创建临时文件失败 path=%s: %v", reqLog.path, err)
		return
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		log.Printf("WARN: [reqlog] 设置权限失败 path=%s: %v", temp, err)
		return
	}
	if _, err := file.Write(raw); err != nil {
		file.Close()
		log.Printf("WARN: [reqlog] 写入失败 path=%s: %v", temp, err)
		return
	}
	if err := file.Sync(); err != nil {
		file.Close()
		log.Printf("WARN: [reqlog] 同步失败 path=%s: %v", temp, err)
		return
	}
	if err := file.Close(); err != nil {
		log.Printf("WARN: [reqlog] 关闭失败 path=%s: %v", temp, err)
		return
	}
	if err := os.Rename(temp, reqLog.path); err != nil {
		log.Printf("WARN: [reqlog] 替换失败 path=%s: %v", reqLog.path, err)
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

// TotalRequests 返回持久化请求序列的最新编号（含失败）；空窗口为 0。
func TotalRequests() int64 { return chatSeq.Load() }

// RequestLogCapacity 环形缓冲容量（前端展示"最近 N 条"用）。
func RequestLogCapacity() int { return reqLogCap }
