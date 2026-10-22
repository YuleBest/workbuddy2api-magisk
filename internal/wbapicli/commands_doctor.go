package wbapicli

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

type doctorReport struct {
	ok       []string
	problems []string
}

func (r *doctorReport) check(condition bool, okMessage, badMessage string) {
	if condition {
		r.ok = append(r.ok, okMessage)
	} else {
		r.problems = append(r.problems, badMessage)
	}
}

func (a *App) cmdDoctor() int {
	a.loadConfig()
	report := &doctorReport{}

	unitUser := "root"
	unitExec := ""
	if a.osName == "windows" {
		exe := joinProject(a.project, "server.exe")
		task := a.deps.Runner.Run("schtasks", "/Query", "/TN", gatewayService)
		report.check(task.ExitCode == 0,
			"计划任务已注册（wbapi-gateway）",
			"计划任务 wbapi-gateway 未注册（开机自启失效）")
		_, err := a.deps.FS.Stat(exe)
		report.check(err == nil,
			"二进制存在："+exe,
			"server.exe 不存在（先 go build -o server.exe ./cmd/server）")
	} else {
		unitUser = strings.TrimSpace(a.systemctl("show", "-p", "User", "--value", gatewayService))
		if unitUser == "" {
			unitUser = "root"
		}
		fragmentPath := strings.TrimSpace(a.systemctl("show", "-p", "FragmentPath", "--value", gatewayService))
		if fragmentPath != "" {
			if raw, err := a.deps.FS.ReadFile(fragmentPath); err == nil {
				for _, line := range strings.Split(string(raw), "\n") {
					if strings.HasPrefix(line, "ExecStart=") {
						unitExec = strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))
						break
					}
				}
			}
		}
		report.check(unitExec != "",
			fmt.Sprintf("unit 已加载（User=%s）", unitUser),
			fmt.Sprintf("%s 未加载（systemd 里没有这个服务？）", gatewayService))
		exe := ""
		if fields := strings.Fields(unitExec); len(fields) > 0 {
			exe = fields[0]
		}
		_, err := a.deps.FS.Stat(exe)
		report.check(err == nil && exe != "",
			"二进制存在："+exe,
			"unit 指向的二进制不存在："+exe)
	}

	authDir := a.authDir()
	entries, err := a.deps.FS.ReadDir(authDir)
	var authFiles []string
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
				authFiles = append(authFiles, entry.Name())
			}
		}
	}
	sort.Strings(authFiles)
	report.check(len(authFiles) > 0,
		fmt.Sprintf("凭证文件 %d 个（%s）", len(authFiles), authDir),
		fmt.Sprintf("%s 里没有 .json 凭证（先跑 ./login.sh 登录）", authDir))

	if len(authFiles) > 0 && a.osName != "windows" {
		want, uidErr := a.deps.Platform.UserID(unitUser)
		if uidErr == nil {
			var wrongOwner, unreadable []string
			for _, name := range authFiles {
				path := joinProject(authDir, name)
				uid, err := a.deps.Platform.FileOwnerUID(path)
				if err == nil && uid != want {
					wrongOwner = append(wrongOwner, name)
				}
				if !a.deps.Platform.Readable(path) {
					unreadable = append(unreadable, name)
				}
			}
			report.check(len(wrongOwner) == 0,
				fmt.Sprintf("auths 属主与网关用户（%s）一致", unitUser),
				fmt.Sprintf("auths 里有 %d 个文件属主不是 %s（uid=%d）→ 网关读不到，池子会是 0：\n        sudo chown -R %s:%s %s",
					len(wrongOwner), unitUser, want, unitUser, unitUser, authDir))
			report.check(len(unreadable) == 0,
				"凭证文件当前用户可读",
				fmt.Sprintf("当前用户读不到 %d 个凭证文件", len(unreadable)))
		}
	}

	keysPath := a.keysPath()
	if info, err := a.deps.FS.Stat(keysPath); err == nil {
		mode := fs.FileMode(0)
		if info != nil {
			mode = info.Mode().Perm()
		}
		modeText := fmt.Sprintf("0o%03o", mode.Perm())
		if a.osName == "windows" {
			report.ok = append(report.ok, "keys.json 存在（Windows 下不检查权限位）")
		} else {
			report.check(mode == 0o600 || mode == 0o400,
				"keys.json 权限 "+modeText,
				"keys.json 权限 "+modeText+"（建议 600：内含明文调用 key）")
		}
	} else {
		report.ok = append(report.ok, "keys.json 不存在（网关不鉴权）")
	}

	admin := mapValue(a.cfg["admin"])
	adminEnabled := boolValue(admin["enabled"])
	report.check(adminEnabled, "admin.enabled=true", "admin.enabled 未开：面板与 /admin/* 端点都不会注册")
	if adminEnabled {
		report.check(tokenString(a.cfg["api_key"]) != "",
			"api_key 非空（admin.enabled 的前置）",
			"admin.enabled=true 但 api_key 为空 → 网关会拒绝启动")
	}

	if a.systemctl("is-active", gatewayService) == "active" {
		st, health := a.request("/healthz", "", "GET", nil, 5*time.Second)
		if st == 200 {
			total, healthy := intValue(health["total"]), intValue(health["healthy"])
			report.check(healthy > 0,
				fmt.Sprintf("可接活账号 %d/%d", healthy, total),
				fmt.Sprintf("账号总数 %d，可接活 %d → 看上面的 auths 属主检查", total, healthy))
			var realms []string
			for realm, value := range mapValue(health["realm_servable"]) {
				if boolValue(value) {
					realms = append(realms, realm)
				}
			}
			sort.Strings(realms)
			text := strings.Join(realms, ",")
			if text == "" {
				text = "无"
			}
			report.ok = append(report.ok, "可接活域: "+text)
		} else {
			report.problems = append(report.problems,
				fmt.Sprintf("/healthz 异常（HTTP %d）：%s", st, errMessage(health, "请求失败")))
		}
	} else {
		report.problems = append(report.problems,
			fmt.Sprintf("%s 未运行（sudo systemctl start %s）", gatewayService, gatewayService))
	}
	report.check(a.systemctl("is-active", tunnelService) == "active",
		tunnelService+" 运行中",
		tunnelService+" 未运行（公网不可达）")

	for _, line := range report.ok {
		fmt.Fprintf(a.stdout, "  ✓ %s\n", line)
	}
	for _, line := range report.problems {
		fmt.Fprintf(a.stdout, "  ✗ %s\n", line)
	}
	suffix := ""
	if len(report.problems) > 0 {
		suffix = "（都有可执行的修复命令）"
	}
	fmt.Fprintf(a.stdout, "\n%d 项正常，%d 项待处理%s\n", len(report.ok), len(report.problems), suffix)
	if len(report.problems) > 0 {
		return 1
	}
	return 0
}
