// keys.go 管理后台的调用 key 增删查（读写 keys.json，热生效，无需重启网关）。
package admin

import (
	"net/http"
	"strings"
)

// keysList 列出全部 key（掩码）。
func (h *Handler) keysList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"keys":          h.cfg.Keys.List(),
		"file":          h.cfg.Keys.Path(),
		"auth_required": h.cfg.Keys.AuthRequired(),
	})
}

// keysCreate 生成新 key（完整值仅在本次响应里返回一次）。
func (h *Handler) keysCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Note string `json:"note"`
	}
	_ = jsonDecode(r, &body)
	note := strings.TrimSpace(body.Note)
	key, err := h.cfg.Keys.Add(note)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "key_create_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"key":     key,
		"note":    note,
		"message": "新 key 已生效，请立即复制保存——关闭后不再显示完整值",
	})
}

// keysRevoke 按前缀吊销（前缀须唯一匹配）。
func (h *Handler) keysRevoke(w http.ResponseWriter, r *http.Request) {
	prefix := strings.TrimSpace(r.PathValue("prefix"))
	remaining, err := h.cfg.Keys.Remove(prefix)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "key_revoke_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "remaining": remaining})
}
