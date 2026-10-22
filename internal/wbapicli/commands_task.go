package wbapicli

import (
	"fmt"
	"strings"
)

var taskLabels = map[string]string{
	"checkin":   "签到",
	"activity":  "活跃上报",
	"travel":    "猫猫旅行",
	"keepalive": "token 保活",
	"school":    "开学季任务",
	"cat":       "夜猫子任务",
}

func (a *App) cmdTask(args []string) int {
	if len(args) == 0 {
		return a.usageError("task", "", "缺少任务名")
	}
	name := args[0]
	label, ok := taskLabels[name]
	if !ok {
		return a.usageError("task", "", fmt.Sprintf("未知任务 %q", name))
	}
	wait := containsArgument(args, "--wait") || (name == "checkin" && !containsArgument(args, "--async"))
	path := "/tasks/" + name
	if wait {
		path += "?wait=1"
	}
	st, data, code := a.adminRequest(path, "POST", nil)
	if code != 0 {
		return code
	}
	if st != 200 && st != 202 {
		return a.fail("触发失败：" + errMessage(data, "请求失败"))
	}
	message := fallbackString(data["message"], label+"已触发")
	fmt.Fprintf(a.stdout, "[✓] %s\n", message)
	outcomes := sliceValue(data["outcomes"])
	if len(outcomes) > 0 {
		for _, item := range outcomes {
			outcome := mapValue(item)
			status := stringValue(outcome["status"])
			statusLabel := status
			switch status {
			case "ok":
				statusLabel = "✓ 签到成功"
			case "already":
				statusLabel = "· 今天已签到"
			case "fail":
				statusLabel = "✗ 失败"
			case "skipped":
				statusLabel = "· 跳过"
			}
			credits := strings.Repeat(" ", 11)
			if outcome["credits"] != nil {
				credits = fmt.Sprintf("%7d 积分", intValue(outcome["credits"]))
			}
			nickname := fallbackString(outcome["nickname"], truncateRunes(stringValue(outcome["uid"]), 8))
			detail := ""
			if value := fallbackString(outcome["detail"], ""); value != "" {
				detail = "  " + value
			}
			fmt.Fprintf(a.stdout, "    %s %s %s%s\n",
				pad(statusLabel, 12), pad(nickname, 10), credits, detail)
		}
	} else if wait && data["waited"] == false {
		fmt.Fprintln(a.stdout, "    （没等到结果，任务仍在后台跑；稍后 wbapi accounts 看积分变化）")
	}
	return 0
}

func containsArgument(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}
