package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func resetRequestLogForTest(t *testing.T) {
	t.Helper()
	reqLog.mu.Lock()
	reqLog.ring = make([]RequestRecord, reqLogCap)
	reqLog.next = 0
	reqLog.n = 0
	reqLog.path = ""
	reqLog.mu.Unlock()
	chatSeq.Store(0)
}

// TestRequestLogRingOrder 校验环形缓冲的"最新在前"与 limit 截断。
func TestRequestLogRingOrder(t *testing.T) {
	base := len(RecentRequests(0))
	for i := 1; i <= 3; i++ {
		appendRequest(RequestRecord{Seq: int64(1000 + i), Model: "m", Status: http.StatusOK})
	}
	got := RecentRequests(3)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Seq != 1003 || got[1].Seq != 1002 || got[2].Seq != 1001 {
		t.Errorf("顺序应为最新在前，得到 %d,%d,%d", got[0].Seq, got[1].Seq, got[2].Seq)
	}
	if all := RecentRequests(0); len(all) != base+3 {
		t.Errorf("limit=0 应返回全部，len = %d, want %d", len(all), base+3)
	}
}

// TestRequestLogCapacityWrap 校验写满后覆盖最旧而不是越界。
func TestRequestLogCapacityWrap(t *testing.T) {
	for i := 0; i < reqLogCap+10; i++ {
		appendRequest(RequestRecord{Seq: int64(2000 + i)})
	}
	got := RecentRequests(0)
	if len(got) != reqLogCap {
		t.Fatalf("len = %d, want 容量 %d", len(got), reqLogCap)
	}
	if got[0].Seq != int64(2000+reqLogCap+9) {
		t.Errorf("最新一条 = %d, want %d", got[0].Seq, 2000+reqLogCap+9)
	}
}

// TestLogChatRowFeedsRing 校验请求出口的表格日志同时入环（后台的实时日志来源）。
func TestLogChatRowFeedsRing(t *testing.T) {
	logChatRow(1500*time.Millisecond, 4*time.Second, "deepseek-v4-flash", "stream", "abcdefghijkl", "小于", http.StatusOK, 120, 800)
	got := RecentRequests(1)
	if len(got) != 1 {
		t.Fatalf("应有 1 条记录，得到 %d", len(got))
	}
	rec := got[0]
	if rec.Model != "deepseek-v4-flash" || rec.Status != http.StatusOK ||
		rec.PromptTokens != 120 || rec.CompletionTokens != 800 || rec.Tokens != 800 {
		t.Errorf("记录字段不符：%+v", rec)
	}
	if rec.UID != "abcdefgh" {
		t.Errorf("uid 应脱敏为前 8 位，得到 %q", rec.UID)
	}
	if rec.Nick != "小于" {
		t.Errorf("昵称应一并入环（后台日志表展示用），得到 %q", rec.Nick)
	}
	if rec.TTFBMS != 1500 || rec.TotalMS != 4000 {
		t.Errorf("耗时字段不符：ttfb=%d total=%d", rec.TTFBMS, rec.TotalMS)
	}
	if rec.TokPS != 200 {
		t.Errorf("速率 = %v, want 200", rec.TokPS)
	}
}

func TestRequestLogPersistsAcrossRestart(t *testing.T) {
	resetRequestLogForTest(t)
	t.Cleanup(func() {
		SetRequestLogPath("")
		chatSeq.Store(0)
	})
	path := filepath.Join(t.TempDir(), "requests.json")
	SetRequestLogPath(path)

	appendRequest(RequestRecord{
		Seq:              41,
		Time:             time.Now().Add(-time.Minute),
		Model:            "model-a",
		UID:              "uid-a",
		PromptTokens:     120,
		CompletionTokens: 30,
		Tokens:           30,
	})
	appendRequest(RequestRecord{
		Seq:              42,
		Time:             time.Now(),
		Model:            "model-b",
		UID:              "uid-b",
		PromptTokens:     80,
		CompletionTokens: 20,
		Tokens:           20,
	})

	resetRequestLogForTest(t)
	SetRequestLogPath(path)

	got := RecentRequests(0)
	if len(got) != 2 {
		t.Fatalf("重启后应有 2 条记录，得到 %d", len(got))
	}
	if got[0].Seq != 42 || got[1].Seq != 41 {
		t.Errorf("恢复顺序不符：%d,%d", got[0].Seq, got[1].Seq)
	}
	if got[0].PromptTokens != 80 || got[0].CompletionTokens != 20 {
		t.Errorf("恢复的 token 字段不符：%+v", got[0])
	}
	if seq := chatSeq.Load(); seq != 42 {
		t.Errorf("恢复后序号 = %d, want 42", seq)
	}
}

func TestRequestLogRestoreKeepsOnlyLatest500(t *testing.T) {
	resetRequestLogForTest(t)
	t.Cleanup(func() {
		SetRequestLogPath("")
		chatSeq.Store(0)
	})
	path := filepath.Join(t.TempDir(), "requests.json")
	records := make([]RequestRecord, reqLogCap+12)
	for i := range records {
		records[i] = RequestRecord{Seq: int64(i + 1), Model: "model"}
	}
	raw, err := json.Marshal(records)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	SetRequestLogPath(path)
	got := RecentRequests(0)
	if len(got) != reqLogCap {
		t.Fatalf("恢复 %d 条，want %d", len(got), reqLogCap)
	}
	if got[0].Seq != int64(reqLogCap+12) || got[len(got)-1].Seq != 13 {
		t.Errorf("恢复窗口 = %d..%d, want newest %d oldest 13", got[0].Seq, got[len(got)-1].Seq, reqLogCap+12)
	}
}

// TestTokPerSecEdge 无 usage（-1）或零耗时不产出速率，避免图表出现假数据。
func TestTokPerSecEdge(t *testing.T) {
	if got := tokPerSec(-1, time.Second); got != 0 {
		t.Errorf("无 usage 应为 0，得到 %v", got)
	}
	if got := tokPerSec(100, 0); got != 0 {
		t.Errorf("零耗时应为 0，得到 %v", got)
	}
}
