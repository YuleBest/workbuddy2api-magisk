package wbapicli

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	gatewayService = "wbapi-gateway"
	tunnelService  = "wbapi-tunnel"
)

func (a *App) systemctl(args ...string) string {
	if a.osName == "windows" {
		if len(args) > 0 && args[0] == "is-active" {
			service := ""
			if len(args) > 1 {
				service = args[1]
			}
			switch service {
			case gatewayService:
				status, _ := a.request("/healthz", "", "GET", nil, 3*time.Second)
				if status == 200 {
					return "active"
				}
				return "inactive"
			case tunnelService:
				result := a.deps.Runner.Run("tasklist", "/FI", "IMAGENAME eq cloudflared.exe")
				if strings.Contains(result.Stdout, "cloudflared.exe") {
					return "active"
				}
				return "inactive"
			}
		}
		return ""
	}
	result := a.deps.Runner.Run("systemctl", args...)
	out := strings.TrimSpace(result.Stdout)
	if out == "" {
		out = strings.TrimSpace(result.Stderr)
	}
	return out
}

func (a *App) cmdStatus() int {
	gateway := a.systemctl("is-active", gatewayService)
	tunnel := a.systemctl("is-active", tunnelService)
	gatewayDetail := "未运行"
	if gateway == "active" {
		gatewayDetail = a.baseURL()
	}
	tunnelDetail := "未运行"
	if tunnel == "active" {
		tunnelDetail = "https://" + a.publicHost
	}
	fmt.Fprintf(a.stdout, "网关:   %s  (%s)\n", gateway, gatewayDetail)
	fmt.Fprintf(a.stdout, "隧道:   %s  (%s)\n", tunnel, tunnelDetail)
	if gateway != "active" {
		return 0
	}

	st, data := a.request("/healthz", "", "GET", nil, 15*time.Second)
	if st == 0 {
		fmt.Fprintf(a.stdout, "健康:   查询失败 %s\n", errMessage(data, "请求失败"))
		return 0
	}
	realms := mapValue(data["realm_servable"])
	servable := make([]string, 0, len(realms))
	for key, value := range realms {
		if boolValue(value) {
			servable = append(servable, key)
		}
	}
	sort.Strings(servable)
	servableText := strings.Join(servable, ",")
	if servableText == "" {
		servableText = "无"
	}
	fmt.Fprintf(a.stdout, "健康:   healthy=%s  账号数=%s  可接活域=%s\n",
		pyValue(data["healthy"]), pyValue(data["total"]), servableText)

	st, status := a.request("/status", a.gatewayToken(), "GET", nil, 15*time.Second)
	if st == 200 {
		fmt.Fprintf(a.stdout, "池:     健康 %s / 冷却 %s / 禁用 %s / 在途占满 %s / 粘性 %s\n",
			pyValue(status["healthy"]), pyValue(status["cooling"]), pyValue(status["disabled"]),
			pyValue(status["in_flight_full"]), pyValue(defaultAny(status["sticky_sessions"], 0)))
	}
	if st == 401 {
		fmt.Fprintln(a.stdout, "池:     鉴权失败（keys.json 首个 key 不对？）")
	}
	return 0
}

func defaultAny(v any, fallback any) any {
	if v == nil {
		return fallback
	}
	return v
}

func stateFlag(account map[string]any) string {
	if boolValue(account["manual_disabled"]) {
		return "⏸"
	}
	if boolValue(account["disabled"]) {
		return "✗"
	}
	if boolValue(account["cooling"]) {
		return "⏳"
	}
	return "✓"
}

func stateText(account map[string]any) string {
	var bits []string
	if boolValue(account["manual_disabled"]) {
		bits = append(bits, fmt.Sprintf("手动停用（%s）", fallbackString(account["manual_reason"], "无原因")))
	}
	if boolValue(account["disabled"]) {
		bits = append(bits, fmt.Sprintf("系统禁用（%s）", fallbackString(account["disabled_reason"], "无原因")))
	}
	if boolValue(account["cooling"]) {
		kind := fallbackString(account["cool_kind"], "冷却")
		switch kind {
		case "soft_rate":
			kind = "限流冷却"
		case "hard_credit":
			kind = "积分耗尽"
		}
		bits = append(bits, fmt.Sprintf("%s，剩 %s", kind, fmtSecs(account["cool_remaining_sec"])))
	}
	if len(bits) == 0 {
		bits = append(bits, fmt.Sprintf("正常（成功 %s 次，在途 %s）",
			pyValue(defaultAny(account["success_count"], 0)), pyValue(defaultAny(account["in_flight"], 0))))
	}
	return strings.Join(bits, "；")
}

func fallbackString(v any, fallback string) string {
	if pyFalsy(v) {
		return fallback
	}
	s := stringValue(v)
	return s
}

func accountExtras(account map[string]any) []string {
	var out []string
	if limited := sliceValue(account["rate_limited_models"]); len(limited) > 0 {
		models := make([]string, 0, len(limited))
		for _, item := range limited {
			models = append(models, fallbackString(mapValue(item)["model"], "?"))
		}
		out = append(out, "限额模型: "+strings.Join(models, ", "))
	}
	if until := fallbackString(account["degrade_until"], ""); until != "" && prefix4(until) != "0001" {
		out = append(out, fmt.Sprintf("连败降权至 %s（连续失败 %v 次）",
			timestamp(until), defaultAny(account["consecutive_fails"], 0)))
	}
	if costs := sliceValue(account["model_costs"]); len(costs) > 0 {
		if len(costs) > 3 {
			costs = costs[:3]
		}
		items := make([]string, 0, len(costs))
		for _, item := range costs {
			row := mapValue(item)
			cost := pyFloat(row["cost_per_1k"])
			costText := "免费"
			if cost > 0 {
				costText = formatFloat(cost, 4)
			}
			items = append(items, fmt.Sprintf("%s=%s/1k", stringValue(row["model"]), costText))
		}
		out = append(out, "成本台账: "+strings.Join(items, ", "))
	}
	if last := fallbackString(account["last_success"], ""); last != "" && prefix4(last) != "0001" {
		out = append(out, "最近成功 "+timestamp(last))
	}
	return out
}

func prefix4(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[:4]
}

func (a *App) cmdAccounts() int {
	st, data := a.request("/status", a.gatewayToken(), "GET", nil, 15*time.Second)
	if st != 200 {
		return a.fail("读取账号池失败：" + errMessage(data, "请求失败"))
	}
	accounts := sliceValue(data["accounts"])
	if len(accounts) == 0 {
		fmt.Fprintln(a.stdout, "账号池是空的：先跑 ./login.sh 完成一次设备授权登录")
		return 0
	}
	grouped := make(map[string][]map[string]any)
	for _, item := range accounts {
		account := mapValue(item)
		realm := fallbackString(account["realm"], "cn")
		grouped[realm] = append(grouped[realm], account)
	}
	realms := make([]string, 0, len(grouped))
	for realm := range grouped {
		realms = append(realms, realm)
	}
	sort.Strings(realms)
	fmt.Fprintf(a.stdout, "账号池（%d 个，域 %d 个）:\n", len(accounts), len(grouped))
	for _, realm := range realms {
		fmt.Fprintf(a.stdout, "  [%s]\n", realm)
		for _, account := range grouped[realm] {
			fmt.Fprintf(a.stdout, "    %s %s %7d 积分   %s\n",
				stateFlag(account),
				pad(fallbackString(account["nickname"], "(未命名)"), 10),
				intValue(account["credits"]),
				stateText(account))
			for _, extra := range accountExtras(account) {
				fmt.Fprintf(a.stdout, "        %s\n", extra)
			}
		}
	}
	fmt.Fprintln(a.stdout, "\n  提示: 手动停用（我摘的）用 `wbapi acct enable` 放回；系统禁用（网关判坏的）用 `wbapi acct revive`。")
	return 0
}
