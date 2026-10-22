package main

import (
	"fmt"
	"os"

	"workbuddy2api/internal/timezone"
)

const usage = `wbapi - WorkBuddy2API KernelSU 模块管理

用法:
  wbapi status [--json]       服务与账号池状态
  wbapi accounts              账号池详情
  wbapi quota [test [模型]]   限额状态或模型探测
  wbapi acct ...              账号停用 / 恢复 / 复活
  wbapi task ...              手动触发 checkin / activity / travel / keepalive / school / cat
  wbapi key ...               list | new [备注] | revoke <前缀>
  wbapi models [--realm cn|global]
  wbapi stats [--reset]
  wbapi panel [--show-token]
  wbapi config                生效配置，敏感字段脱敏
  wbapi log [N]               最近 N 行日志
  wbapi start|stop|restart    管理网关进程
  wbapi doctor                本地体检
  wbapi login [--realm=cn|global]
  wbapi help

Android 版不启动自动定时任务，所有任务可在需要时手动触发。
`

func main() {
	timezone.Init()
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"status"}
	}
	command := args[0]
	rest := args[1:]
	if command == "help" || command == "-h" || command == "--help" {
		fmt.Print(usage)
		return
	}

	env, err := loadEnv()
	if err != nil {
		fatal(err)
	}
	switch command {
	case "status", "check":
		err = env.cmdStatus(hasArg(rest, "--json"))
	case "accounts":
		err = env.cmdAccounts()
	case "quota", "pool":
		err = env.cmdQuota(rest)
	case "acct", "account":
		err = env.cmdAcct(rest)
	case "task":
		err = env.cmdTask(rest)
	case "key":
		err = cmdKey(env, rest)
	case "models", "list":
		err = env.cmdModels(rest)
	case "stats":
		err = env.cmdStats(rest)
	case "panel":
		err = env.cmdPanel(rest)
	case "config":
		err = env.cmdConfig()
	case "doctor":
		err = env.cmdDoctor()
	case "log":
		err = env.cmdLog(rest)
	case "start":
		err = env.startService(hasArg(rest, "--quiet"))
	case "stop":
		err = env.stopService(hasArg(rest, "--quiet"))
	case "restart":
		err = env.restartService()
	case "login":
		err = env.cmdLogin(rest)
	case "tunnel":
		err = fmt.Errorf("Android 模块不包含 Cloudflare Tunnel")
	default:
		fmt.Print(usage)
		os.Exit(2)
	}
	if err != nil {
		fatal(err)
	}
}

func cmdKey(env *moduleEnv, args []string) error {
	if len(args) == 0 || args[0] == "list" {
		return env.keyList(hasArg(args, "--show"))
	}
	switch args[0] {
	case "new":
		note := ""
		if len(args) > 1 {
			note = args[1]
		}
		return env.keyNew(note)
	case "revoke", "rm", "delete":
		if len(args) < 2 {
			return fmt.Errorf("用法: wbapi key revoke <前缀>")
		}
		return env.keyRevoke(args[1])
	default:
		return fmt.Errorf("未知 key 子命令: %s", args[0])
	}
}

func hasArg(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "[!]", err)
	os.Exit(1)
}
