package wbapicli

import (
	"fmt"
	"strings"
)

type helpItem struct {
	Name        string
	Description string
}

type helpGroup struct {
	Name  string
	Items []helpItem
}

type commandHelp struct {
	Name     string
	Summary  string
	Usage    []string
	Children []helpItem
	Options  []helpItem
	Notes    []string
	Examples []string
}

var mainHelpGroups = []helpGroup{
	{
		Name: "服务",
		Items: []helpItem{
			{"status", "查看网关、隧道、健康状态与账号池概况"},
			{"start", "启动网关与隧道"},
			{"stop", "停止网关与隧道"},
			{"restart", "重启网关与隧道"},
			{"tunnel", "单独管理 Cloudflare Tunnel"},
		},
	},
	{
		Name: "诊断与数据",
		Items: []helpItem{
			{"doctor", "检查本地部署、权限与运行态"},
			{"accounts", "查看账号池详情"},
			{"quota", "查看冷却与限额，或实测模型可用性"},
			{"models", "查看模型、倍率、能力与上下文"},
			{"stats", "查看或清零请求统计"},
			{"log", "查看网关日志"},
			{"config", "查看脱敏后的生效配置"},
			{"panel", "显示管理后台地址与登录提示"},
		},
	},
	{
		Name: "账号与任务",
		Items: []helpItem{
			{"acct", "停用、恢复或复活账号"},
			{"task", "手动触发定时任务"},
		},
	},
	{
		Name: "密钥",
		Items: []helpItem{
			{"key", "查看、创建或吊销调用密钥"},
		},
	},
	{
		Name: "帮助",
		Items: []helpItem{
			{"help", "查看命令或子命令帮助"},
			{"env", "查看环境变量"},
			{"auth", "查看鉴权与账号登录说明"},
		},
	},
}

var commandHelpTopics = map[string]commandHelp{
	"status": {
		Name:    "wbapi status",
		Summary: "查看网关、隧道、健康状态与账号池概况",
		Usage:   []string{"wbapi status", "wbapi check"},
		Notes:   []string{"不带任何参数运行 wbapi 时，默认执行 status。"},
		Examples: []string{
			"wbapi",
			"wbapi status",
		},
	},
	"doctor": {
		Name:    "wbapi doctor",
		Summary: "检查本地部署、权限与运行态",
		Usage:   []string{"wbapi doctor"},
		Notes: []string{
			"检查服务入口、二进制、auths 属主、keys.json 权限、管理面和运行态。",
			"发现问题时返回非零退出码，并输出对应修复建议。",
		},
		Examples: []string{"wbapi doctor"},
	},
	"accounts": {
		Name:    "wbapi accounts",
		Summary: "查看账号池详情",
		Usage:   []string{"wbapi accounts"},
		Notes:   []string{"显示域、积分、冷却、停用状态、限额记录和实测成本。"},
		Examples: []string{
			"wbapi accounts",
		},
	},
	"quota": {
		Name:    "wbapi quota",
		Summary: "查看冷却与限额，或实测模型可用性",
		Usage: []string{
			"wbapi quota",
			"wbapi quota test [模型...]",
		},
		Children: []helpItem{
			{"test [模型...]", "发送最小请求实测模型可用性"},
		},
		Notes: []string{
			"quota 是 pool 的别名。",
			"test 不指定模型时，会测试网关返回的全部模型。",
			"每个测试请求使用 max_tokens=16，会消耗少量配额。",
		},
		Examples: []string{
			"wbapi quota",
			"wbapi quota test glm-5.2",
		},
	},
	"acct": {
		Name:    "wbapi acct",
		Summary: "停用、恢复或复活账号",
		Usage: []string{
			"wbapi acct list",
			"wbapi acct disable <uid|前缀> [原因]",
			"wbapi acct enable <uid|前缀>",
			"wbapi acct revive <uid|前缀>",
		},
		Children: []helpItem{
			{"list", "列出账号，等价于 wbapi accounts"},
			{"disable <uid|前缀> [原因]", "设置手动停用状态，将账号移出选号池"},
			{"enable <uid|前缀>", "清除手动停用状态"},
			{"revive <uid|前缀>", "清除网关写入的系统禁用状态"},
		},
		Notes: []string{
			"UID 可以写完整值或唯一前缀。",
			"手动停用与系统禁用是两个独立状态，都清除后账号才回到选号池。",
			"账号动作要求 config.admin.enabled=true。",
		},
		Examples: []string{
			"wbapi acct disable 00e26541 观察一天",
			"wbapi acct enable 00e26541",
		},
	},
	"task": {
		Name:    "wbapi task",
		Summary: "手动触发定时任务",
		Usage: []string{
			"wbapi task <任务> [--wait|--async]",
		},
		Children: []helpItem{
			{"checkin", "立即执行签到，默认等待并显示逐号结果"},
			{"activity", "立即执行活跃上报"},
			{"travel", "立即执行猫猫旅行"},
			{"keepalive", "立即执行 token 保活"},
			{"school", "立即执行开学季任务"},
			{"cat", "立即执行夜猫子任务"},
		},
		Options: []helpItem{
			{"--wait", "等待任务完成并显示回执"},
			{"--async", "立即返回；checkin 使用它可关闭默认等待"},
		},
		Notes: []string{
			"只有 checkin 默认等待；其他任务默认异步执行。",
			"异步任务的结果写入网关日志，可稍后用 wbapi accounts 或 wbapi log 查看。",
		},
		Examples: []string{
			"wbapi task checkin",
			"wbapi task activity --wait",
		},
	},
	"key": {
		Name:    "wbapi key",
		Summary: "查看、创建或吊销调用密钥",
		Usage: []string{
			"wbapi key list [--show]",
			"wbapi key new [备注]",
			"wbapi key revoke <前缀>",
		},
		Children: []helpItem{
			{"list [--show]", "列出密钥；默认掩码显示"},
			{"new [备注]", "创建并立即启用一个新密钥"},
			{"revoke <前缀>", "按唯一前缀吊销密钥"},
		},
		Options: []helpItem{
			{"--show", "配合 list 显示完整密钥"},
		},
		Notes: []string{
			"密钥直接写入 keys.json，网关会热加载，不需要重启。",
			"revoke 支持别名 rm 和 delete。",
		},
		Examples: []string{
			"wbapi key list",
			`wbapi key new "笔记本"`,
		},
	},
	"models": {
		Name:    "wbapi models",
		Summary: "查看模型、倍率、能力与上下文",
		Usage:   []string{"wbapi models [--realm cn|global] [--more]"},
		Options: []helpItem{
			{"--realm <cn|global>", "只显示指定域的模型"},
			{"--more", "显示推理档位和全部上游字段"},
		},
		Notes:    []string{"models 是 list 的别名。"},
		Examples: []string{"wbapi models", "wbapi models --realm global --more"},
	},
	"stats": {
		Name:    "wbapi stats",
		Summary: "查看或清零请求统计",
		Usage:   []string{"wbapi stats [--reset]"},
		Options: []helpItem{
			{"--reset", "清零 token、模型榜和延迟统计"},
		},
		Examples: []string{"wbapi stats", "wbapi stats --reset"},
	},
	"panel": {
		Name:     "wbapi panel",
		Summary:  "显示管理后台地址与登录提示",
		Usage:    []string{"wbapi panel"},
		Examples: []string{"wbapi panel"},
	},
	"config": {
		Name:     "wbapi config",
		Summary:  "查看脱敏后的生效配置",
		Usage:    []string{"wbapi config"},
		Notes:    []string{"API key、管理 token、Upstash 凭据和设备 token 会显示为 ***。"},
		Examples: []string{"wbapi config"},
	},
	"log": {
		Name:    "wbapi log",
		Summary: "查看网关日志",
		Usage:   []string{"wbapi log [行数]"},
		Notes: []string{
			"Linux 读取 wbapi-gateway 的 journalctl 日志。",
			"Windows 读取运行目录下的 gateway.err.log 或 gateway.log。",
			"默认显示最近 30 行。",
		},
		Examples: []string{"wbapi log", "wbapi log 100"},
	},
	"start": {
		Name:     "wbapi start",
		Summary:  "启动网关与隧道",
		Usage:    []string{"wbapi start"},
		Examples: []string{"wbapi start"},
	},
	"stop": {
		Name:     "wbapi stop",
		Summary:  "停止网关与隧道",
		Usage:    []string{"wbapi stop"},
		Examples: []string{"wbapi stop"},
	},
	"restart": {
		Name:     "wbapi restart",
		Summary:  "重启网关与隧道",
		Usage:    []string{"wbapi restart"},
		Notes:    []string{"重启隧道时公网连接会短暂中断。"},
		Examples: []string{"wbapi restart"},
	},
	"tunnel": {
		Name:    "wbapi tunnel",
		Summary: "单独管理 Cloudflare Tunnel",
		Usage: []string{
			"wbapi tunnel status",
			"wbapi tunnel start",
			"wbapi tunnel stop",
			"wbapi tunnel restart",
		},
		Children: []helpItem{
			{"status", "查看隧道状态，默认动作"},
			{"start", "启动隧道"},
			{"stop", "停止隧道"},
			{"restart", "重启隧道"},
		},
		Examples: []string{"wbapi tunnel status", "wbapi tunnel restart"},
	},
	"healthcheck": {
		Name:     "wbapi healthcheck",
		Summary:  "静默检查网关健康状态",
		Usage:    []string{"wbapi healthcheck"},
		Notes:    []string{"健康时返回 0，不输出内容；异常时返回 1。主要供服务探活使用。"},
		Examples: []string{"wbapi healthcheck"},
	},
	"help": {
		Name:    "wbapi help",
		Summary: "查看命令或子命令帮助",
		Usage: []string{
			"wbapi help",
			"wbapi help <命令>",
			"wbapi help <命令> <子命令>",
		},
		Notes: []string{"所有命令也支持 -h 或 --help。"},
		Examples: []string{
			"wbapi help acct",
			"wbapi help task checkin",
		},
	},
	"env": {
		Name:    "wbapi 环境变量",
		Summary: "控制命令使用的路径、网关地址和公网域名",
		Options: []helpItem{
			{"WB2A_HOME", "项目根目录，默认取 wbapi 可执行文件所在目录"},
			{"WB2A_CONFIG", "配置文件路径，默认 $WB2A_HOME/config.json"},
			{"WB2A_GATEWAY", "网关地址，覆盖 config.json 的 listen"},
			{"WB2A_PUBLIC_HOST", "状态和面板显示的公网域名"},
		},
		Examples: []string{"WB2A_GATEWAY=http://127.0.0.1:7863 wbapi status"},
	},
	"auth": {
		Name:    "wbapi 鉴权与登录",
		Summary: "说明网关鉴权、管理 token 和账号登录",
		Notes: []string{
			"/v1/*、/status 和 /admin/* 使用调用密钥。",
			"/api/admin/* 优先使用 config.admin.token，未配置时回落到调用密钥。",
			"调用密钥优先取 keys.json，文件为空时回落到 config.api_key。",
			"添加账号运行 login.sh；国际版使用 login.sh --realm=global。",
		},
		Examples: []string{
			"login.sh",
			"wbapi accounts",
		},
	},
}

var commandSubHelp = map[string]map[string]commandHelp{
	"quota": {
		"test": {
			Name:    "wbapi quota test",
			Summary: "发送最小请求实测模型可用性",
			Usage:   []string{"wbapi quota test [模型...]"},
			Notes: []string{
				"不指定模型时测试网关返回的全部模型。",
				"每个请求使用 max_tokens=16，会消耗少量配额。",
			},
			Examples: []string{"wbapi quota test glm-5.2"},
		},
	},
	"acct": {
		"list": {
			Name:     "wbapi acct list",
			Summary:  "列出账号，等价于 wbapi accounts",
			Usage:    []string{"wbapi acct list"},
			Examples: []string{"wbapi acct list"},
		},
		"disable": {
			Name:     "wbapi acct disable",
			Summary:  "设置手动停用状态，将账号移出选号池",
			Usage:    []string{"wbapi acct disable <uid|前缀> [原因]"},
			Notes:    []string{"原因只取第一个非命令参数；包含空格时请加引号。"},
			Examples: []string{"wbapi acct disable 00e26541 观察一天"},
		},
		"enable": {
			Name:     "wbapi acct enable",
			Summary:  "清除手动停用状态",
			Usage:    []string{"wbapi acct enable <uid|前缀>"},
			Examples: []string{"wbapi acct enable 00e26541"},
		},
		"revive": {
			Name:     "wbapi acct revive",
			Summary:  "清除网关写入的系统禁用状态",
			Usage:    []string{"wbapi acct revive <uid|前缀>"},
			Notes:    []string{"系统禁用与手动停用相互独立。"},
			Examples: []string{"wbapi acct revive 00e26541"},
		},
	},
	"task": {
		"checkin": {
			Name:     "wbapi task checkin",
			Summary:  "立即执行签到并显示逐号结果",
			Usage:    []string{"wbapi task checkin [--async]"},
			Notes:    []string{"默认等待结果；加 --async 时立即返回。"},
			Examples: []string{"wbapi task checkin"},
		},
		"activity":  taskHelp("activity", "立即执行活跃上报"),
		"travel":    taskHelp("travel", "立即执行猫猫旅行"),
		"keepalive": taskHelp("keepalive", "立即执行 token 保活"),
		"school":    taskHelp("school", "立即执行开学季任务"),
		"cat":       taskHelp("cat", "立即执行夜猫子任务"),
	},
	"key": {
		"list": {
			Name:    "wbapi key list",
			Summary: "列出调用密钥",
			Usage:   []string{"wbapi key list [--show]"},
			Options: []helpItem{{"--show", "显示完整密钥"}},
		},
		"new": {
			Name:     "wbapi key new",
			Summary:  "创建并立即启用一个新密钥",
			Usage:    []string{"wbapi key new [备注]"},
			Notes:    []string{"备注包含空格时请加引号。"},
			Examples: []string{`wbapi key new "笔记本"`},
		},
		"revoke": {
			Name:     "wbapi key revoke",
			Summary:  "按唯一前缀吊销密钥",
			Usage:    []string{"wbapi key revoke <前缀>"},
			Notes:    []string{"支持别名 rm 和 delete。"},
			Examples: []string{"wbapi key revoke sk-0101"},
		},
	},
	"tunnel": {
		"status":  simpleSubHelp("wbapi tunnel status", "查看隧道状态"),
		"start":   simpleSubHelp("wbapi tunnel start", "启动隧道"),
		"stop":    simpleSubHelp("wbapi tunnel stop", "停止隧道"),
		"restart": simpleSubHelp("wbapi tunnel restart", "重启隧道"),
	},
}

var helpTopicAliases = map[string]string{
	"check":    "status",
	"check-up": "doctor",
	"account":  "acct",
	"pool":     "quota",
	"list":     "models",
	"-h":       "help",
	"--help":   "help",
}

func taskHelp(name, summary string) commandHelp {
	return commandHelp{
		Name:     "wbapi task " + name,
		Summary:  summary,
		Usage:    []string{"wbapi task " + name + " [--wait]"},
		Notes:    []string{"默认异步执行；加 --wait 时等待任务回执。"},
		Examples: []string{"wbapi task " + name + " --wait"},
	}
}

func simpleSubHelp(name, summary string) commandHelp {
	return commandHelp{Name: name, Summary: summary, Usage: []string{name}}
}

func normalizeHelpTopic(topic string) string {
	if canonical, ok := helpTopicAliases[topic]; ok {
		return canonical
	}
	return topic
}

func normalizeHelpSubcommand(topic, sub string) string {
	switch topic {
	case "key":
		switch sub {
		case "rm", "delete":
			return "revoke"
		}
	}
	return sub
}

func renderMainHelp() string {
	var b strings.Builder
	b.WriteString("wbapi - WorkBuddy2API 网关运维 CLI\n\n")
	b.WriteString("用法:\n")
	b.WriteString("  wbapi [命令] [参数]\n\n")
	b.WriteString("命令:\n")
	for i, group := range mainHelpGroups {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "  %s:\n", group.Name)
		for _, item := range group.Items {
			fmt.Fprintf(&b, "    %s  %s\n", pad(item.Name, 22), item.Description)
		}
	}
	b.WriteString("\n选项:\n")
	b.WriteString("  -h, --help                显示命令帮助\n")
	b.WriteString("\n帮助:\n")
	b.WriteString("  wbapi help <命令>         查看命令用法\n")
	b.WriteString("  wbapi help <命令> <子命令> 查看子命令用法\n")
	return b.String()
}

func renderCommandHelp(topic, sub string) (string, bool) {
	topic = normalizeHelpTopic(topic)
	sub = normalizeHelpSubcommand(topic, sub)
	if sub != "" {
		if children, ok := commandSubHelp[topic]; ok {
			if h, ok := children[sub]; ok {
				return renderCommandHelpText(h), true
			}
		}
	}
	h, ok := commandHelpTopics[topic]
	if !ok {
		return "", false
	}
	return renderCommandHelpText(h), true
}

func renderCommandHelpText(h commandHelp) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s - %s\n", h.Name, h.Summary)
	if len(h.Usage) > 0 {
		b.WriteString("\n用法:\n")
		for _, line := range h.Usage {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}
	if len(h.Children) > 0 {
		b.WriteString("\n子命令:\n")
		renderHelpItems(&b, h.Children)
	}
	if len(h.Options) > 0 {
		b.WriteString("\n选项:\n")
		renderHelpItems(&b, h.Options)
	}
	if len(h.Notes) > 0 {
		b.WriteString("\n说明:\n")
		for _, line := range h.Notes {
			fmt.Fprintf(&b, "  - %s\n", line)
		}
	}
	if len(h.Examples) > 0 {
		b.WriteString("\n示例:\n")
		for _, line := range h.Examples {
			fmt.Fprintf(&b, "  %s\n", line)
		}
	}
	return b.String()
}

func renderHelpItems(b *strings.Builder, items []helpItem) {
	width := 0
	for _, item := range items {
		if current := displayWidth(item.Name); current > width {
			width = current
		}
	}
	for _, item := range items {
		fmt.Fprintf(b, "  %s  %s\n", pad(item.Name, width), item.Description)
	}
}
