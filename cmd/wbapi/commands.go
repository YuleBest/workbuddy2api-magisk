package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (e *moduleEnv) cmdStatus(asJSON bool) error {
	state := e.serviceState()
	healthy, health := e.health(3 * time.Second)
	var pool map[string]any
	if healthy {
		pool, _ = e.poolStatus()
	}
	if asJSON {
		out := map[string]any{
			"module_dir": e.ModuleDir,
			"base_url":   e.baseURL(),
			"pid":        state.PID,
			"running":    state.Running,
			"healthy":    healthy,
			"health":     health,
			"pool":       pool,
		}
		raw, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(raw))
		return nil
	}
	fmt.Printf("网关:   %s  %s\n", runningText(state.Running), e.baseURL())
	if healthy {
		fmt.Printf("健康:   healthy=%v  账号数=%v\n", health["healthy"], health["total"])
	}
	if pool != nil {
		fmt.Printf("账号池: 健康 %v / 冷却 %v / 禁用 %v / 在途占满 %v\n",
			pool["healthy"], pool["cooling"], pool["disabled"], pool["in_flight_full"])
	}
	fmt.Printf("模块:   %s\n", e.ModuleDir)
	return nil
}

func (e *moduleEnv) cmdAccounts() error {
	value, err := e.poolStatus()
	if err != nil {
		return err
	}
	accounts, _ := value["accounts"].([]any)
	if len(accounts) == 0 {
		fmt.Println("账号池为空，可运行: " + filepath.Join(filepath.Dir(e.ServerBin), "wbapi") + " login")
		return nil
	}
	fmt.Printf("账号池（%d 个）:\n", len(accounts))
	for _, item := range accounts {
		account, _ := item.(map[string]any)
		flags := []string{}
		if boolValue(account["manual_disabled"]) {
			flags = append(flags, "手动停用")
		}
		if boolValue(account["disabled"]) {
			flags = append(flags, "系统禁用")
		}
		if boolValue(account["cooling"]) {
			flags = append(flags, "冷却")
		}
		if len(flags) == 0 {
			flags = append(flags, "正常")
		}
		fmt.Printf("  %-8s  %-12s  %v 积分  %s\n",
			shortUID(stringValue(account["uid"])),
			stringValue(account["nickname"]),
			account["credits"],
			strings.Join(flags, " / "))
	}
	return nil
}

func (e *moduleEnv) cmdAcct(args []string) error {
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	if sub == "list" {
		return e.cmdAccounts()
	}
	if sub != "disable" && sub != "enable" && sub != "revive" {
		return fmt.Errorf("用法: wbapi acct list|disable <uid> [原因]|enable <uid>|revive <uid>")
	}
	if len(args) < 2 {
		return fmt.Errorf("缺少 uid")
	}
	uid, err := e.resolveUID(args[1])
	if err != nil {
		return err
	}
	var body any
	if sub == "disable" {
		reason := "manual"
		if len(args) > 2 {
			reason = strings.Join(args[2:], " ")
		}
		body = map[string]string{"reason": reason}
	}
	status, raw, err := e.request(fmt.Sprintf("/admin/accounts/%s/%s", uid, sub), e.gatewayToken(), "POST", body, 15*time.Second)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("操作失败 HTTP %d: %s", status, apiError(raw))
	}
	fmt.Printf("已执行 %s: %s\n", sub, shortUID(uid))
	return nil
}

func (e *moduleEnv) resolveUID(prefix string) (string, error) {
	if len(prefix) >= 36 {
		return prefix, nil
	}
	value, err := e.poolStatus()
	if err != nil {
		return "", err
	}
	accounts, _ := value["accounts"].([]any)
	var hits []string
	for _, item := range accounts {
		account, _ := item.(map[string]any)
		uid := stringValue(account["uid"])
		if strings.HasPrefix(uid, prefix) {
			hits = append(hits, uid)
		}
	}
	if len(hits) != 1 {
		return "", fmt.Errorf("uid 前缀匹配到 %d 个账号", len(hits))
	}
	return hits[0], nil
}

func (e *moduleEnv) cmdQuota(args []string) error {
	if len(args) > 0 && args[0] == "test" {
		return e.cmdQuotaTest(args[1:])
	}
	value, err := e.poolStatus()
	if err != nil {
		return err
	}
	fmt.Printf("账号 %v，健康 %v，冷却 %v，禁用 %v\n",
		value["total"], value["healthy"], value["cooling"], value["disabled"])
	return nil
}

func (e *moduleEnv) cmdQuotaTest(models []string) error {
	if len(models) == 0 {
		status, raw, err := e.request("/v1/models", e.gatewayToken(), "GET", nil, 30*time.Second)
		if err != nil {
			return err
		}
		if status != 200 {
			return fmt.Errorf("拉取模型失败 HTTP %d: %s", status, apiError(raw))
		}
		var body struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return err
		}
		for _, model := range body.Data {
			models = append(models, model.ID)
		}
	}
	for _, model := range models {
		started := time.Now()
		status, raw, err := e.request("/v1/chat/completions", e.gatewayToken(), "POST", map[string]any{
			"model": model, "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_tokens": 16,
		}, 90*time.Second)
		if err != nil {
			fmt.Printf("  %-28s 失败: %v\n", model, err)
			continue
		}
		if status == 200 {
			fmt.Printf("  %-28s 可用 %.1fs\n", model, time.Since(started).Seconds())
		} else {
			fmt.Printf("  %-28s 不可用 HTTP %d: %s\n", model, status, apiError(raw))
		}
	}
	return nil
}

func (e *moduleEnv) cmdTask(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("用法: wbapi task checkin|activity|travel|keepalive [--wait]")
	}
	name := args[0]
	if name == "school" || name == "cat" {
		return fmt.Errorf("Android 版不包含 Python 任务: %s", name)
	}
	switch name {
	case "checkin", "activity", "travel", "keepalive":
	default:
		return fmt.Errorf("未知任务: %s", name)
	}
	wait := name == "checkin"
	for _, arg := range args[1:] {
		if arg == "--wait" {
			wait = true
		}
		if arg == "--async" {
			wait = false
		}
	}
	path := "/tasks/" + name
	if wait {
		path += "?wait=1"
	}
	status, raw, err := e.adminRequest(path, "POST", nil, 95*time.Second)
	if err != nil {
		return err
	}
	if status != 200 && status != 202 {
		return fmt.Errorf("触发失败 HTTP %d: %s", status, apiError(raw))
	}
	value, err := decodeObject(raw)
	if err != nil {
		return err
	}
	fmt.Println(stringValue(value["message"]))
	return nil
}

func (e *moduleEnv) cmdModels(args []string) error {
	realm := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--realm" && i+1 < len(args) {
			realm = args[i+1]
			i++
		}
	}
	status, raw, err := e.request("/v1/models", e.gatewayToken(), "GET", nil, 30*time.Second)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("拉取模型失败 HTTP %d: %s", status, apiError(raw))
	}
	var body struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return err
	}
	for _, model := range body.Data {
		if realm != "" && !strings.HasPrefix(model.ID, realm+":") {
			continue
		}
		if model.Name != "" {
			fmt.Printf("  %-32s %s\n", model.ID, model.Name)
		} else {
			fmt.Printf("  %s\n", model.ID)
		}
	}
	return nil
}

func (e *moduleEnv) cmdStats(args []string) error {
	reset := len(args) > 0 && args[0] == "--reset"
	path := "/v1/stats"
	method := "GET"
	if reset {
		path = "/v1/stats/reset"
		method = "POST"
	}
	status, raw, err := e.request(path, e.gatewayToken(), method, nil, 30*time.Second)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("统计请求失败 HTTP %d: %s", status, apiError(raw))
	}
	if reset {
		fmt.Println("统计已清零")
		return nil
	}
	value, err := decodeObject(raw)
	if err != nil {
		return err
	}
	total, _ := value["total"].(map[string]any)
	fmt.Printf("请求 %v，成功 %v，失败 %v，token %v\n",
		total["requests"], total["success"], total["failed"], total["total_tokens"])
	return nil
}

func (e *moduleEnv) cmdPanel(args []string) error {
	showToken := false
	for _, arg := range args {
		switch arg {
		case "--show-token":
			showToken = true
		default:
			return fmt.Errorf("未知参数: %s", arg)
		}
	}
	token := e.adminToken()
	source := "keys.json 首个 key / config.api_key"
	if e.Config.Admin.Token != "" {
		source = "config.admin.token"
	}
	fmt.Printf("管理后台: %s\n", e.adminURL())
	if showToken {
		if token == "" {
			fmt.Println("管理 token: 未配置，后台不鉴权")
		} else {
			fmt.Printf("管理 token: %s（%s）\n", token, source)
		}
	} else if token != "" {
		fmt.Printf("管理 token: %s（%s）\n", maskKey(token), source)
	}
	return nil
}

func (e *moduleEnv) cmdConfig() error {
	status, raw, err := e.adminRequest("/config", "GET", nil, 15*time.Second)
	if err == nil && status == 200 {
		var value map[string]any
		if json.Unmarshal(raw, &value) == nil {
			if cfg, ok := value["config"]; ok {
				pretty, _ := json.MarshalIndent(cfg, "", "  ")
				fmt.Println(string(pretty))
				return nil
			}
		}
	}
	raw, err = os.ReadFile(e.ConfigPath)
	if err != nil {
		return err
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	redactConfig(cfg)
	pretty, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(pretty))
	return nil
}

func redactConfig(cfg map[string]any) {
	for _, key := range []string{"api_key"} {
		if value, ok := cfg[key].(string); ok && value != "" {
			cfg[key] = "***"
		}
	}
	if admin, ok := cfg["admin"].(map[string]any); ok {
		if value, ok := admin["token"].(string); ok && value != "" {
			admin["token"] = "***"
		}
	}
	if upstream, ok := cfg["upstream"].(map[string]any); ok {
		if value, ok := upstream["device_token"].(string); ok && value != "" {
			upstream["device_token"] = "***"
		}
	}
	if upstash, ok := cfg["upstash"].(map[string]any); ok {
		for _, key := range []string{"url", "token"} {
			if value, ok := upstash[key].(string); ok && value != "" {
				upstash[key] = "***"
			}
		}
	}
}

func (e *moduleEnv) cmdLog(args []string) error {
	lines := 30
	if len(args) > 0 {
		if value, err := strconv.Atoi(args[0]); err == nil && value > 0 {
			lines = value
		}
	}
	raw, err := os.ReadFile(e.LogFile)
	if os.IsNotExist(err) {
		fmt.Println("暂无日志")
		return nil
	}
	if err != nil {
		return err
	}
	all := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	fmt.Println(strings.Join(all, "\n"))
	return nil
}

func (e *moduleEnv) cmdDoctor() error {
	var problems []string
	check := func(ok bool, good, bad string) {
		if ok {
			fmt.Printf("  ✓ %s\n", good)
		} else {
			problems = append(problems, bad)
			fmt.Printf("  ✗ %s\n", bad)
		}
	}
	serverInfo, _ := os.Stat(e.ServerBin)
	check(serverInfo != nil && serverInfo.Mode().IsRegular(), "服务端二进制存在", "服务端二进制缺失: "+e.ServerBin)
	check(e.listenIsLoopback(), "监听地址仅限 loopback", "监听地址不是 loopback: "+e.Config.Listen)
	check(e.Config.Admin.Enabled, "管理后台已启用", "admin.enabled=false")
	check(e.Config.APIKey != "", "api_key 已配置", "api_key 为空")
	authFiles, _ := os.ReadDir(e.AuthDir)
	authCount := 0
	for _, entry := range authFiles {
		if strings.HasSuffix(entry.Name(), ".json") {
			authCount++
		}
	}
	check(authCount > 0, fmt.Sprintf("凭证文件 %d 个", authCount), "auths 中没有凭证，运行 bin/wbapi login")
	if info, err := os.Stat(e.KeysFile); err == nil {
		check(info.Mode().Perm()&0o077 == 0, "keys.json 权限正确", "keys.json 权限过宽: "+info.Mode().Perm().String())
	} else {
		check(false, "", "keys.json 不存在: "+e.KeysFile)
	}
	state := e.serviceState()
	check(state.Running, "服务进程运行中", "服务未运行")
	healthy, health := e.health(5 * time.Second)
	check(healthy, "健康检查通过", "健康检查失败")
	if healthy {
		check(intValue(health["total"]) > 0, fmt.Sprintf("可接活账号 %v/%v", health["healthy"], health["total"]), "账号数为 0")
	}
	if len(problems) > 0 {
		return fmt.Errorf("发现 %d 项问题", len(problems))
	}
	fmt.Println("检查完成，未发现问题")
	return nil
}

func boolValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func intValue(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func shortUID(uid string) string {
	if len(uid) <= 8 {
		return uid
	}
	return uid[:8]
}
