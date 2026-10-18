package admin

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// chatStub 冒充数据面 handler：记录收到的请求，并按 SSE 回两帧。
type chatStub struct {
	method  string
	path    string
	auth    string
	xff     string
	body    string
	flushed bool
}

func (s *chatStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.method = r.Method
	s.path = r.URL.Path
	s.auth = r.Header.Get("Authorization")
	s.xff = r.Header.Get("X-Forwarded-For")
	b, _ := io.ReadAll(r.Body)
	s.body = string(b)

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你\"}}]}\n\n")
	if fl, ok := w.(http.Flusher); ok {
		fl.Flush()
		s.flushed = true
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
}

// TestAdminChatProxy 面板对话要原样走数据面：admin token 换成数据面 key，body 与 SSE 逐字透传。
func TestAdminChatProxy(t *testing.T) {
	h, _, ks := newTestHandler(t, "t")
	stub := &chatStub{}
	h.cfg.Chat = stub
	key, err := ks.Add("面板")
	if err != nil {
		t.Fatalf("造 key 失败: %v", err)
	}

	// 对话接口同样受 admin 鉴权保护，不能变成绕过面板鉴权的口子。
	rec := do(t, h, http.MethodPost, "/api/admin/v1/chat/completions", "", `{"model":"cn:x"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("无 token = %d, want 401", rec.Code)
	}

	body := `{"model":"cn:glm-4.6","messages":[{"role":"user","content":"hi"}],"stream":true}`
	rec = do(t, h, http.MethodPost, "/api/admin/v1/chat/completions", "t", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("对话 = %d: %s", rec.Code, rec.Body.String())
	}
	if stub.method != http.MethodPost || stub.path != "/v1/chat/completions" {
		t.Errorf("数据面收到 %s %s, want POST /v1/chat/completions", stub.method, stub.path)
	}
	if stub.auth != "Bearer "+key {
		t.Errorf("数据面 Authorization = %q, want %q", stub.auth, "Bearer "+key)
	}
	if stub.body != body {
		t.Errorf("body 未原样透传：%q", stub.body)
	}
	// SSE 必须逐帧到浏览器，而不是攒完一次性给（攒着就不叫流式了）。
	if !stub.flushed {
		t.Error("流式响应未 flush，面板会看不到逐字输出")
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if got := rec.Body.String(); !strings.Contains(got, `"content":"你"`) || !strings.Contains(got, "[DONE]") {
		t.Errorf("SSE 帧未透传：%q", got)
	}
}

// TestAdminChatForwardsClientIP 上游指纹用的客户端 IP 头要透传（数据面不读 RemoteAddr）。
func TestAdminChatForwardsClientIP(t *testing.T) {
	h, _, _ := newTestHandler(t, "t")
	stub := &chatStub{}
	h.cfg.Chat = stub

	req := httptest.NewRequest(http.MethodPost, "/api/admin/v1/chat/completions", strings.NewReader(`{"model":"m"}`))
	req.Header.Set("Authorization", "Bearer t")
	req.Header.Set("X-Forwarded-For", "203.0.113.9, 10.0.0.1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if stub.xff != "203.0.113.9, 10.0.0.1" {
		t.Errorf("X-Forwarded-For = %q, want 原样透传", stub.xff)
	}
}

// TestAdminChatUnavailable 未注入数据面 handler 时明确回 503，而不是 500 或空响应。
func TestAdminChatUnavailable(t *testing.T) {
	h, _, _ := newTestHandler(t, "t")
	rec := do(t, h, http.MethodPost, "/api/admin/v1/chat/completions", "t", `{"model":"m"}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("Chat 未接入 = %d, want 503", rec.Code)
	}
	if code := decode(t, rec)["error"].(map[string]any)["code"]; code != "chat_unavailable" {
		t.Errorf("error.code = %v, want chat_unavailable", code)
	}
}

// TestAdminChatBodyLimit 超大 body 回 413，不喂给数据面。
func TestAdminChatBodyLimit(t *testing.T) {
	h, _, _ := newTestHandler(t, "t")
	h.cfg.Chat = &chatStub{}
	big := `{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("x", maxChatBody) + `"}]}`
	rec := do(t, h, http.MethodPost, "/api/admin/v1/chat/completions", "t", big)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("超大 body = %d, want 413", rec.Code)
	}
}
