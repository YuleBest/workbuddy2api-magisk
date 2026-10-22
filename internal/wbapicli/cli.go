package wbapicli

import (
	"fmt"
	"strconv"
	"strings"
)

func Run(opts Options) int {
	app := newApp(opts)
	if len(opts.Args) > 0 && opts.Args[0] == "healthcheck" {
		app.quietConfig = true
	}
	if !isHelpRequest(opts.Args) {
		app.loadConfig()
	}
	return app.run(opts.Args)
}

func (a *App) run(args []string) int {
	if len(args) == 0 {
		return a.cmdStatus()
	}
	cmd := args[0]
	rest := args[1:]
	if cmd == "help" {
		return a.cmdHelp(rest)
	}
	if cmd == "-h" || cmd == "--help" {
		return a.cmdHelp(nil)
	}
	if containsHelpFlag(rest) {
		helpArgs := []string{cmd}
		if _, hasChildren := commandSubHelp[normalizeHelpTopic(cmd)]; hasChildren {
			if sub := firstHelpArgument(rest); sub != "" {
				helpArgs = append(helpArgs, sub)
			}
		}
		return a.cmdHelp(helpArgs)
	}
	switch cmd {
	case "status", "check":
		return a.cmdStatus()
	case "accounts":
		return a.cmdAccounts()
	case "quota", "pool":
		return a.cmdQuota(rest)
	case "acct", "account":
		return a.cmdAcct(rest)
	case "task":
		return a.cmdTask(rest)
	case "models", "list":
		return a.cmdModels(rest)
	case "stats":
		return a.cmdStats(rest)
	case "panel":
		return a.cmdPanel()
	case "config":
		return a.cmdConfig()
	case "doctor", "check-up":
		return a.cmdDoctor()
	case "key":
		return a.cmdKey(args, rest)
	case "log":
		n := 30
		if len(rest) > 0 {
			value, err := strconv.Atoi(rest[0])
			if err != nil {
				return a.usageError("log", "", fmt.Sprintf("日志行数必须是整数：%s", rest[0]))
			}
			n = value
		}
		return a.cmdLog(n)
	case "start", "stop", "restart":
		return a.cmdService(cmd)
	case "tunnel":
		return a.cmdTunnel(rest)
	case "healthcheck":
		return a.cmdHealthcheck()
	default:
		fmt.Fprintf(a.stderr, "错误: 未知命令 %q\n", cmd)
		fmt.Fprintln(a.stderr, "运行 `wbapi help` 查看可用命令。")
		return 2
	}
}

func (a *App) cmdHelp(args []string) int {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg != "-h" && arg != "--help" {
			filtered = append(filtered, arg)
		}
	}
	if len(filtered) == 0 {
		fmt.Fprint(a.stdout, renderMainHelp())
		return 0
	}
	topic := filtered[0]
	sub := ""
	if len(filtered) > 1 {
		sub = filtered[1]
	}
	if sub != "" {
		topic = normalizeHelpTopic(topic)
		sub = normalizeHelpSubcommand(topic, sub)
		children, hasChildren := commandSubHelp[topic]
		if !hasChildren {
			fmt.Fprintf(a.stderr, "错误: %q 没有子命令 %q\n", topic, sub)
			fmt.Fprintf(a.stderr, "运行 `wbapi help %s` 查看用法。\n", topic)
			return 2
		}
		if _, ok := children[sub]; !ok {
			fmt.Fprintf(a.stderr, "错误: %q 没有子命令 %q\n", topic, sub)
			fmt.Fprintf(a.stderr, "运行 `wbapi help %s` 查看可用子命令。\n", topic)
			return 2
		}
	}
	text, ok := renderCommandHelp(topic, sub)
	if !ok {
		fmt.Fprintf(a.stderr, "错误: 没有 %q 的帮助信息\n", topic)
		fmt.Fprintln(a.stderr, "运行 `wbapi help` 查看可用命令。")
		return 2
	}
	fmt.Fprint(a.stdout, text)
	return 0
}

func (a *App) usageError(topic, sub, message string) int {
	fmt.Fprintf(a.stderr, "错误: %s\n", message)
	target := strings.TrimSpace("wbapi help " + topic + " " + sub)
	fmt.Fprintf(a.stderr, "运行 `%s` 查看用法。\n", target)
	return 2
}

func containsHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func isHelpRequest(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return args[0] == "help" || args[0] == "-h" || args[0] == "--help" || containsHelpFlag(args[1:])
}

func firstHelpArgument(args []string) string {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || strings.HasPrefix(arg, "-") {
			continue
		}
		return arg
	}
	return ""
}

func (a *App) cmdKey(allArgs, rest []string) int {
	sub := "list"
	if len(rest) > 0 {
		sub = rest[0]
	}
	switch sub {
	case "list":
		show := false
		for _, arg := range allArgs {
			if arg == "--show" {
				show = true
				break
			}
		}
		return a.cmdKeyList(show)
	case "new":
		note := ""
		if len(rest) > 1 {
			note = rest[1]
		}
		return a.cmdKeyNew(note)
	case "revoke", "rm", "delete":
		if len(rest) < 2 {
			return a.usageError("key", "revoke", "缺少要吊销的密钥前缀")
		}
		return a.cmdKeyRevoke(rest[1])
	default:
		return a.usageError("key", "", fmt.Sprintf("未知子命令 %q", sub))
	}
}

func pythonRepr(s string) string {
	return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", `\'`) + "'"
}
