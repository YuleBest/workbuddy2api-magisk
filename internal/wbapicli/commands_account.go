package wbapicli

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

func (a *App) resolveUID(prefix string) (string, int) {
	if utf8.RuneCountInString(prefix) >= 36 {
		return prefix, 0
	}
	st, data := a.request("/status", a.gatewayToken(), "GET", nil, 15e9)
	if st != 200 {
		return "", a.fail("读取账号列表失败：" + errMessage(data, "请求失败"))
	}
	var hits []string
	for _, item := range sliceValue(data["accounts"]) {
		uid := stringValue(mapValue(item)["uid"])
		if strings.HasPrefix(uid, prefix) {
			hits = append(hits, uid)
		}
	}
	if len(hits) == 0 {
		return "", a.fail(fmt.Sprintf("没有匹配 %s 的账号", pythonRepr(prefix)))
	}
	if len(hits) > 1 {
		return "", a.fail(fmt.Sprintf("前缀 %s 匹配到 %d 个账号，请写长一点", pythonRepr(prefix), len(hits)))
	}
	return hits[0], 0
}

func (a *App) cmdAcct(args []string) int {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	if sub == "list" {
		return a.cmdAccounts()
	}
	if sub != "disable" && sub != "enable" && sub != "revive" {
		return a.usageError("acct", "", fmt.Sprintf("未知子命令 %q", sub))
	}
	if len(args) < 2 {
		return a.usageError("acct", sub, "缺少账号 UID 或前缀")
	}
	uid, code := a.resolveUID(args[1])
	if code != 0 {
		return code
	}
	var body any
	if sub == "disable" && len(args) > 2 {
		body = map[string]any{"reason": args[2]}
	}
	st, data := a.request("/admin/accounts/"+uid+"/"+sub, a.gatewayToken(), "POST", body, 15e9)
	if st != 200 {
		return a.fail(fmt.Sprintf("%s 失败：%s", sub, errMessage(data, "请求失败")))
	}
	state := "手动位已清"
	if boolValue(data["manual_disabled"]) {
		state = "手动停用"
	}
	auto := "系统位正常"
	if boolValue(data["disabled"]) {
		auto = "系统禁用中（要 revive）"
	}
	changed := "无变化（本来就是该状态）"
	if boolValue(data["changed"]) {
		changed = "已生效"
	}
	fmt.Fprintf(a.stdout, "[✓] %s %s: %s，%s — %s\n", truncateRunes(uid, 8), sub, state, auto, changed)
	return 0
}
