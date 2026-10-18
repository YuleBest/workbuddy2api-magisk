package server

import (
	"net/http"
	"testing"
	"time"
)

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
	logChatRow(1500*time.Millisecond, 4*time.Second, "deepseek-v4-flash", "stream", "abcdefghijkl", "小于", http.StatusOK, 800)
	got := RecentRequests(1)
	if len(got) != 1 {
		t.Fatalf("应有 1 条记录，得到 %d", len(got))
	}
	rec := got[0]
	if rec.Model != "deepseek-v4-flash" || rec.Status != http.StatusOK || rec.Tokens != 800 {
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

// TestTokPerSecEdge 无 usage（-1）或零耗时不产出速率，避免图表出现假数据。
func TestTokPerSecEdge(t *testing.T) {
	if got := tokPerSec(-1, time.Second); got != 0 {
		t.Errorf("无 usage 应为 0，得到 %v", got)
	}
	if got := tokPerSec(100, 0); got != 0 {
		t.Errorf("零耗时应为 0，得到 %v", got)
	}
}
