package wbapicli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

func (a *App) cmdStats(args []string) int {
	if containsArgument(args, "--reset") {
		st, data, code := a.adminRequest("/stats/reset", "POST", nil)
		if code != 0 {
			return code
		}
		if st != 200 {
			return a.fail("清零失败：" + errMessage(data, "请求失败"))
		}
		fmt.Fprintln(a.stdout, "[✓] 统计已清零（从此刻重新累计）")
		return 0
	}
	st, data := a.request("/v1/stats", a.gatewayToken(), "GET", nil, 30*time.Second)
	if st != 200 {
		return a.fail("读取统计失败：" + errMessage(data, "请求失败"))
	}
	if enabled, ok := data["enabled"]; ok && !boolValue(enabled) {
		fmt.Fprintf(a.stdout, "统计未启用：%s\n", fallbackString(data["message"], ""))
		return 0
	}
	total := mapValue(data["total"])
	fmt.Fprintf(a.stdout, "统计窗口: %s 起，已运行 %s\n\n", timestamp(data["since"]), fmtSecs(data["uptime_sec"]))
	fmt.Fprintf(a.stdout, "  总计   请求 %s（成功 %s / 失败 %s）  token %s\n",
		pyValue(defaultAny(total["requests"], 0)), pyValue(defaultAny(total["success"], 0)),
		pyValue(defaultAny(total["failed"], 0)), pyValue(defaultAny(total["total_tokens"], 0)))
	fmt.Fprintf(a.stdout, "         平均首字 %.0fms  平均耗时 %.0fms  速率 %.1ftok/s  缓存命中率 %.0f%%\n",
		pyFloat(total["avg_ttfb_ms"]), pyFloat(total["avg_latency_ms"]),
		pyFloat(total["tokens_per_sec"]), 100*pyFloat(total["cache_hit_rate"]))
	models := sliceValue(data["models"])
	if len(models) > 12 {
		models = models[:12]
	}
	for _, item := range models {
		model := mapValue(item)
		fmt.Fprintf(a.stdout, "  %-28s 请求 %5d  失败 %4d  首字 %6.0fms  token %8d\n",
			stringValue(model["model"]), intValue(model["requests"]), intValue(model["failed"]),
			pyFloat(model["avg_ttfb_ms"]), intValue(model["total_tokens"]))
	}
	return 0
}

func (a *App) cmdPanel() int {
	a.loadConfig()
	token := a.adminToken()
	admin := mapValue(a.cfg["admin"])
	source := "keys.json 首个 key / config.api_key"
	if tokenString(admin["token"]) != "" {
		source = "config.admin.token"
	}
	fmt.Fprintf(a.stdout, "管理后台: %s/admin/\n", a.baseURL())
	fmt.Fprintf(a.stdout, "公网地址: https://%s/admin/\n", a.publicHost)
	if token == "" {
		fmt.Fprintln(a.stdout, "登录 token: 未配置（网关不鉴权）")
	} else {
		fmt.Fprintf(a.stdout, "登录 token: %s…（%s）\n", keyPrefix(token, 8), source)
	}
	if !boolValue(admin["enabled"]) {
		fmt.Fprintln(a.stdout, "[!] config.admin.enabled=false：后台与 /admin/accounts/* 端点都没注册，先在 config.json 打开再重启")
	}
	return 0
}

func (a *App) cmdConfig() int {
	raw, status, err := a.rawRequest("GET", a.baseURL()+"/api/admin/config", a.adminToken(), nil, 15*time.Second)
	if err == nil && status == 200 {
		var envelope struct {
			Config json.RawMessage `json:"config"`
		}
		if json.Unmarshal(raw, &envelope) == nil {
			if len(envelope.Config) == 0 {
				fmt.Fprintln(a.stdout, "{}")
			} else {
				cfg, err := decodeJSON(envelope.Config)
				if err == nil {
					if pyFalsy(cfg) {
						fmt.Fprintln(a.stdout, "{}")
					} else if err := writeIndentedJSON(a.stdout, envelope.Config); err != nil {
						fmt.Fprintln(a.stdout, "{}")
					}
					return 0
				}
			}
			return 0
		}
	}
	a.loadConfig()
	if cfg, err := decodeOrdered(a.cfgRaw); err == nil {
		if _, ok := cfg.(orderedMap); ok {
			var compact bytes.Buffer
			if err := writeCompactOrdered(&compact, redactOrderedLocal(cfg)); err == nil {
				_ = writeIndentedJSON(a.stdout, compact.Bytes())
				return 0
			}
		}
	}
	a.writeJSON(redactLocal(a.cfg))
	return 0
}

func (a *App) writeJSON(v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
	fmt.Fprint(a.stdout, buf.String())
}

func redactLocal(cfg map[string]any) map[string]any {
	out := cloneMap(cfg)
	if !pyFalsy(out["api_key"]) {
		out["api_key"] = "***"
	}
	if upstash := mapValue(out["upstash"]); upstash != nil {
		for _, key := range []string{"url", "token"} {
			if !pyFalsy(upstash[key]) {
				upstash[key] = "***"
			}
		}
		out["upstash"] = upstash
	}
	if admin := mapValue(out["admin"]); admin != nil {
		if !pyFalsy(admin["token"]) {
			admin["token"] = "***"
		}
		out["admin"] = admin
	}
	if upstream := mapValue(out["upstream"]); upstream != nil {
		if !pyFalsy(upstream["device_token"]) {
			upstream["device_token"] = "***"
		}
		out["upstream"] = upstream
	}
	return out
}

func cloneMap(src map[string]any) map[string]any {
	out := make(map[string]any, len(src))
	for key, value := range src {
		switch typed := value.(type) {
		case map[string]any:
			out[key] = cloneMap(typed)
		case []any:
			out[key] = cloneSlice(typed)
		default:
			out[key] = value
		}
	}
	return out
}

func cloneSlice(src []any) []any {
	out := make([]any, len(src))
	for i, value := range src {
		switch typed := value.(type) {
		case map[string]any:
			out[i] = cloneMap(typed)
		case []any:
			out[i] = cloneSlice(typed)
		default:
			out[i] = value
		}
	}
	return out
}

func (a *App) cmdLog(lines int) int {
	if a.osName == "windows" {
		for _, name := range []string{"gateway.err.log", "gateway.log"} {
			raw, err := a.deps.FS.ReadFile(joinProject(a.project, name))
			if err != nil {
				continue
			}
			if len(raw) > 0 {
				text := strings.ToValidUTF8(string(raw), "\uFFFD")
				text = strings.ReplaceAll(text, "\r\n", "\n")
				text = strings.ReplaceAll(text, "\r", "\n")
				fmt.Fprint(a.stdout, lastLines(text, lines))
				return 0
			}
		}
		fmt.Fprintf(a.stdout, "（%s 下没有网关日志文件）\n", a.project)
		return 0
	}
	result := a.deps.Runner.Run("journalctl", "-u", gatewayService, "-n", fmt.Sprint(lines), "--no-pager")
	fmt.Fprintln(a.stdout, result.Stdout)
	return 0
}

func lastLines(text string, count int) string {
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if count == 0 {
		return strings.Join(lines, "")
	}
	if count < 0 {
		start := -count
		if start >= len(lines) {
			return ""
		}
		return strings.Join(lines[start:], "")
	}
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "")
}

func joinProject(project, name string) string {
	return filepath.Join(project, name)
}
