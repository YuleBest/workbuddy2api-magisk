package wbapicli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"workbuddy2api/internal/wbapicli/signin"
)

type CheckinOptions struct {
	Args    []string
	Env     Env
	Project string
	Stdout  io.Writer
	Stderr  io.Writer
	Deps    Dependencies
	Client  signin.Client
}

const checkinHelp = `checkin — 签到结果表格化工具

用法:
  checkin            # 网关状态头 + Go signin 表格
  checkin -v         # 表格 + 末尾 fail 明细汇总
  checkin auths_dir  # 指定 auths 目录

环境变量:
  WB2A_CONFIG  配置文件路径（默认 config.json，读取端口与 api_key）
  WB2A_URL     直接指定服务地址（如 http://1.2.3.4:7863）
`

func RunCheckin(opts CheckinOptions) int {
	if opts.Stdout == nil {
		opts.Stdout = discardWriter{}
	}
	if opts.Stderr == nil {
		opts.Stderr = discardWriter{}
	}
	verbose := false
	authDir := "auths"
	for _, arg := range opts.Args {
		switch arg {
		case "-v", "--verbose":
			verbose = true
		case "-h", "--help":
			fmt.Fprint(opts.Stdout, checkinHelp)
			return 0
		default:
			if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(opts.Stderr, "未知参数: %s（可用 -v / -h）\n", arg)
				return 2
			}
			authDir = arg
		}
	}

	env := cloneEnv(opts.Env)
	if configPath := env["WB2A_CONFIG"]; configPath != "" && !filepath.IsAbs(configPath) {
		env["WB2A_CONFIG"] = filepath.Join(opts.Project, configPath)
	}
	app := newApp(Options{
		Env:     env,
		Project: opts.Project,
		Stdout:  opts.Stdout,
		Stderr:  opts.Stderr,
		Deps:    opts.Deps,
	})
	app.project = opts.Project
	if configPath := env["WB2A_CONFIG"]; configPath != "" {
		app.configPath = configPath
	} else {
		app.configPath = filepath.Join(opts.Project, "config.json")
	}
	app.quietConfig = true
	app.loadConfig()

	base := strings.TrimSpace(env["WB2A_URL"])
	if base == "" {
		listen := fallbackString(app.cfg["listen"], ":7863")
		port := listen
		if idx := strings.LastIndex(listen, ":"); idx >= 0 {
			port = listen[idx+1:]
		}
		if port == "" {
			port = "7863"
		}
		base = "http://localhost:" + port
	}
	base = strings.TrimRight(base, "/")
	requestApp := *app
	requestEnv := cloneEnv(env)
	requestEnv["WB2A_GATEWAY"] = base
	requestApp.env = requestEnv
	requestApp.cfgLoaded = true

	key := tokenString(app.cfg["api_key"])
	statusRaw, statusCode, statusErr := requestApp.rawRequest("GET", base+"/status", key, nil, 15*time.Second)
	if statusErr != nil {
		statusCode = 0
	}
	fmt.Fprintln(opts.Stdout, "── 网关状态 ─────────────────────────────")
	if statusCode == 200 {
		statusData, err := decodeObject(bytes.TrimSpace(statusRaw))
		if err == nil {
			fmt.Fprintf(opts.Stdout, "网关在线：total=%s healthy=%s cooling=%s disabled=%s\n",
				pyValue(statusData["total"]), pyValue(statusData["healthy"]),
				pyValue(statusData["cooling"]), pyValue(statusData["disabled"]))
		} else {
			fmt.Fprintln(opts.Stdout, "网关在线（/status 200）")
		}
		fmt.Fprintln(opts.Stdout, "提示：定时签到已在网关侧运行（09/21 点）。下面跑离线签到工具即时对账。")
	} else {
		switch statusCode {
		case 401:
			fmt.Fprintln(opts.Stdout, "网关鉴权失败（401）：api_key 与 config.json 不一致")
		case 0:
			fmt.Fprintf(opts.Stdout, "连不上 %s：服务未启动或端口不对\n", base)
		default:
			fmt.Fprintf(opts.Stdout, "网关 /status 异常（HTTP %d）\n", statusCode)
		}
	}
	fmt.Fprintln(opts.Stdout)

	displayAuthDir := authDir
	if !filepath.IsAbs(authDir) {
		authDir = filepath.Join(opts.Project, authDir)
	}
	client := opts.Client
	if client == nil {
		client = signin.NewClient()
	}
	summary, err := signin.Collect(authDir, client)
	if err != nil {
		if errors.Is(err, signin.ErrNoAuthFiles) {
			fmt.Fprintf(opts.Stderr, "no auth files in %s\n", displayAuthDir)
		} else {
			fmt.Fprintln(opts.Stderr, err)
		}
		if !verbose {
			return 1
		}
		fmt.Fprintln(opts.Stdout)
	} else {
		signin.Render(opts.Stdout, summary)
	}
	if verbose {
		fmt.Fprintln(opts.Stdout)
		signin.RenderFailures(opts.Stdout, summary)
	}
	return 0
}

func cloneEnv(env Env) Env {
	out := make(Env, len(env))
	for key, value := range env {
		out[key] = value
	}
	return out
}
