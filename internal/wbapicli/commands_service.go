package wbapicli

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

func (a *App) cmdService(action string) int {
	if a.osName == "windows" {
		for _, service := range []struct {
			Name string
			Proc string
		}{
			{gatewayService, "server.exe"},
			{tunnelService, "cloudflared.exe"},
		} {
			var result CommandResult
			ok := false
			switch action {
			case "start":
				result = a.deps.Runner.Run("schtasks", "/Run", "/TN", service.Name)
				ok = result.ExitCode == 0
			case "stop":
				result = a.deps.Runner.Run("taskkill", "/F", "/IM", service.Proc)
				ok = result.ExitCode == 0
			default:
				_ = a.deps.Runner.Run("taskkill", "/F", "/IM", service.Proc)
				a.deps.Platform.Sleep(2 * time.Second)
				result = a.deps.Runner.Run("schtasks", "/Run", "/TN", service.Name)
				ok = result.ExitCode == 0
			}
			state := "[✓] 完成"
			if !ok {
				detail := strings.TrimSpace(result.Stderr)
				if detail == "" {
					detail = strings.TrimSpace(result.Stdout)
				}
				state = "[!] 失败：" + detail
			}
			fmt.Fprintf(a.stdout, "%s %s: %s\n", action, service.Name, state)
		}
		return 0
	}
	for _, service := range []string{gatewayService, tunnelService} {
		out := a.systemctl(action, service)
		if out == "" {
			out = "ok"
		}
		fmt.Fprintf(a.stdout, "%s %s: %s\n", action, service, out)
	}
	return 0
}

func (a *App) cmdTunnel(args []string) int {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	if sub == "status" {
		fmt.Fprintf(a.stdout, "隧道: %s (https://%s)\n", a.systemctl("is-active", tunnelService), a.publicHost)
		return 0
	}
	if a.osName == "windows" {
		switch sub {
		case "stop":
			result := a.deps.Runner.Run("taskkill", "/F", "/IM", "cloudflared.exe")
			state := "[!] 未在运行"
			if result.ExitCode == 0 {
				state = "[✓] 完成"
			}
			fmt.Fprintf(a.stdout, "stop %s: %s\n", tunnelService, state)
			return 0
		case "start":
			result := a.deps.Runner.Run("schtasks", "/Run", "/TN", tunnelService)
			a.printTunnelStartResult("start", result)
			return 0
		case "restart":
			_ = a.deps.Runner.Run("taskkill", "/F", "/IM", "cloudflared.exe")
			a.deps.Platform.Sleep(2 * time.Second)
			result := a.deps.Runner.Run("schtasks", "/Run", "/TN", tunnelService)
			a.printTunnelStartResult("restart", result)
			return 0
		}
	}
	out := a.systemctl(sub, tunnelService)
	if out == "" {
		out = "[✓] " + sub + " 完成"
	}
	fmt.Fprintln(a.stdout, out)
	return 0
}

func (a *App) printTunnelStartResult(action string, result CommandResult) {
	state := "[✓] 完成"
	if result.ExitCode != 0 {
		detail := strings.TrimSpace(result.Stderr)
		if detail == "" {
			detail = strings.TrimSpace(result.Stdout)
		}
		state = "[!] 失败：" + detail
	}
	fmt.Fprintf(a.stdout, "%s %s: %s\n", action, tunnelService, state)
}

func (a *App) cmdHealthcheck() int {
	_, status, err := a.rawRequest(http.MethodGet, a.baseURL()+"/healthz", "", nil, 5*time.Second)
	if err == nil && status == http.StatusOK {
		return 0
	}
	return 1
}
