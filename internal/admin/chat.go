package admin

import (
	"bytes"
	"io"
	"net/http"
)

// 面板对话请求体上限：正常聊天几 KB，留足长上下文余量即可。
const maxChatBody = 4 << 20

// chatCompletions 把面板的对话请求代理进数据面。
//
// 为什么是"伪造一个请求再喂给数据面 handler"而不是自己调 upstream：数据面没有
// 内部可信调用方旁路，withAuth 只认数据面 key，而面板用的是另一套 admin token
// （config.admin.token 可与 keys 不同）。这里把 Authorization 换成 keys.First()
// 后交给数据面，选号/粘性/错误策略/请求日志/统计整条链路原样复用，零复制。
//
// w 直接透传给数据面：SSE 分块 flush 落到浏览器，中间不做任何缓冲。
func (h *Handler) chatCompletions(w http.ResponseWriter, r *http.Request) {
	if h.cfg.Chat == nil {
		writeErr(w, http.StatusServiceUnavailable, "chat_unavailable", "数据面未接入，无法对话")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxChatBody))
	if err != nil {
		writeErr(w, http.StatusRequestEntityTooLarge, "body_too_large", "请求体过大（上限 4MB）")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "proxy_failed", "构造数据面请求失败："+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	// key 每请求现取：面板里吊销/新增 key 不会让对话功能失效。
	if h.cfg.Keys != nil {
		if k := h.cfg.Keys.First(); k != "" {
			req.Header.Set("Authorization", "Bearer "+k)
		}
	}
	// 客户端 IP 透传（上游指纹用）：数据面只认这两个头，不读 RemoteAddr。
	for _, name := range []string{"X-Forwarded-For", "X-Real-IP"} {
		if v := r.Header.Get(name); v != "" {
			req.Header.Set(name, v)
		}
	}
	h.cfg.Chat.ServeHTTP(w, req)
}
