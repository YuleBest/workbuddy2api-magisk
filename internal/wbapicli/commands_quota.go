package wbapicli

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (a *App) cmdQuota(args []string) int {
	if len(args) > 0 && args[0] == "test" {
		return a.cmdQuotaTest(args[1:])
	}
	if len(args) > 0 {
		return a.usageError("quota", "", fmt.Sprintf("未知子命令 %q", args[0]))
	}
	st, data := a.request("/status", a.gatewayToken(), "GET", nil, 15*time.Second)
	if st != 200 {
		return a.fail("读取池状态失败：" + errMessage(data, "请求失败"))
	}
	fmt.Fprintf(a.stdout, "池状态: %s 个账号, 健康 %s, 冷却 %s, 禁用 %s, 在途占满 %s\n\n",
		pyValue(data["total"]), pyValue(data["healthy"]), pyValue(data["cooling"]),
		pyValue(data["disabled"]), pyValue(data["in_flight_full"]))
	for _, item := range sliceValue(data["accounts"]) {
		account := mapValue(item)
		head := pad(fallbackString(account["nickname"], "(未命名)"), 10)
		fmt.Fprintf(a.stdout, "  %s %s %s\n", stateFlag(account), head, stateText(account))
		for _, extra := range accountExtras(account) {
			fmt.Fprintf(a.stdout, "      %s\n", extra)
		}
	}
	fmt.Fprintln(a.stdout, "\n提示: 6004（当日模型用量）按账号+模型独立计，冷却只表示该号在该模型上撞过限，换模型通常立即可用。")
	fmt.Fprintln(a.stdout, "      想实测哪个模型现在能用：wbapi quota test [模型名]")
	return 0
}

func (a *App) cmdQuotaTest(models []string) int {
	if len(models) == 0 {
		st, data := a.request("/v1/models", a.gatewayToken(), "GET", nil, 20*time.Second)
		if st != 200 {
			return a.fail("拉模型列表失败：" + errMessage(data, "请求失败"))
		}
		for _, item := range sliceValue(data["data"]) {
			models = append(models, stringValue(mapValue(item)["id"]))
		}
	}
	fmt.Fprintf(a.stdout, "实测 %d 个模型（每个发一条 max_tokens=16 的最小请求，消耗少量配额）:\n\n", len(models))
	var available, unavailable []string
	for _, model := range models {
		fmt.Fprintf(a.stdout, "  %-28s ", model)
		start := a.deps.Now()
		body := struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			MaxTokens int `json:"max_tokens"`
		}{
			Model: model,
			Messages: []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{{Role: "user", Content: "hi"}},
			MaxTokens: 16,
		}
		st, data := a.request("/v1/chat/completions", a.gatewayToken(), http.MethodPost, body, 90*time.Second)
		elapsed := a.deps.Now().Sub(start).Seconds()
		if st == 200 && len(sliceValue(data["choices"])) > 0 {
			fmt.Fprintf(a.stdout, "✓ %.1fs\n", elapsed)
			available = append(available, model)
		} else {
			message := errMessage(data, "请求失败")
			message = truncateRunes(message, 70)
			fmt.Fprintf(a.stdout, "✗ [%d] %s\n", st, message)
			unavailable = append(unavailable, model)
		}
	}
	fmt.Fprintf(a.stdout, "\n可用 %d/%d\n", len(available), len(models))
	if len(unavailable) > 0 {
		fmt.Fprintln(a.stdout, "不可用: "+strings.Join(unavailable, ", "))
	}
	return 0
}
