package tasks

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

func SchoolCLI(args []string, out, errOut io.Writer, service *Service) int {
	return SchoolCLIContext(context.Background(), args, out, errOut, service)
}

func SchoolCLIContext(
	ctx context.Context,
	args []string,
	out, errOut io.Writer,
	service *Service,
) (code int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintf(errOut, "ERR: school2026 panic: %v\n", recovered)
			code = 1
		}
	}()
	if service == nil {
		service = NewService()
	}
	opts := SchoolOptions{Gap: 1.5}
	accounts := []string{}
	var token, tokenUID string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			accounts = append(accounts, args[i+1:]...)
			i = len(args)
		case arg == "--list":
			// --list is the default mode and, like argparse, does not cancel --run.
		case arg == "--run":
			opts.Run = true
		case arg == "--lottery-only":
			opts.LotteryOnly = true
		case arg == "--yes":
			opts.Yes = true
		case arg == "--help" || arg == "-h":
			fmt.Fprint(out, schoolUsage)
			return 0
		case arg == "--token":
			value, next, ok := flagValue(args, i)
			if !ok {
				return cliArgError(errOut, "--token requires a value")
			}
			token, i = value, next
		case strings.HasPrefix(arg, "--token="):
			token = strings.TrimPrefix(arg, "--token=")
		case arg == "--uid":
			value, next, ok := flagValue(args, i)
			if !ok {
				return cliArgError(errOut, "--uid requires a value")
			}
			tokenUID, i = value, next
		case strings.HasPrefix(arg, "--uid="):
			tokenUID = strings.TrimPrefix(arg, "--uid=")
		case arg == "--gap":
			value, next, ok := flagValue(args, i)
			if !ok {
				return cliArgError(errOut, "--gap requires a value")
			}
			gap, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return cliArgError(errOut, "invalid float value for --gap")
			}
			opts.Gap, i = gap, next
		case strings.HasPrefix(arg, "--gap="):
			gap, err := strconv.ParseFloat(strings.TrimPrefix(arg, "--gap="), 64)
			if err != nil {
				return cliArgError(errOut, "invalid float value for --gap")
			}
			opts.Gap = gap
		case strings.HasPrefix(arg, "-"):
			return cliArgError(errOut, "unrecognized arguments: "+arg)
		default:
			accounts = append(accounts, arg)
		}
	}
	if opts.Gap < 1 {
		opts.Gap = 1
	}
	mode := "LIST"
	if opts.LotteryOnly {
		mode = "LOTTERY"
	} else if opts.Run {
		mode = "RUN"
	}
	runMode := opts.Run || opts.LotteryOnly
	if runMode && !opts.Yes {
		fmt.Fprintf(out, "mode=%s dry-run（写操作需 --yes 放行；%s 不带 --yes 只打印将发送的动作）\n", mode, mode)
	} else {
		real := ""
		if opts.Yes {
			real = "REAL"
		}
		fmt.Fprintf(out, "mode=%s %s gap=%s\n", mode, real, formatFloat(opts.Gap))
	}

	loaded := []Account{}
	if token != "" {
		loaded = append(loaded, Account{Token: token, UID: tokenUID, Nickname: "--token--", File: "<cli>"})
	} else {
		selectors := []string{}
		if len(accounts) == 0 || (len(accounts) == 1 && strings.EqualFold(accounts[0], "ALL")) {
			all, loadErrs, err := service.allAccountResults()
			if err != nil {
				fmt.Fprintf(out, "ERR: %v\n", err)
			}
			for _, loadErr := range loadErrs {
				fmt.Fprintf(out, "ERR: %v\n", loadErr)
			}
			loaded = append(loaded, all...)
		} else {
			seen := map[string]bool{}
			for _, account := range accounts {
				if !seen[account] {
					seen[account] = true
					selectors = append(selectors, account)
				}
			}
			for _, selector := range selectors {
				account, err := service.loadAccount(selector)
				if err != nil {
					fmt.Fprintf(out, "ERR: %v\n", err)
					continue
				}
				loaded = append(loaded, account)
			}
		}
	}
	if len(loaded) == 0 {
		fmt.Fprintln(out, "ERR: 无可用账号（检查 auths/ 或 --token）")
		return 1
	}
	stats := service.RunSchoolAccounts(ctx, loaded, opts, out)
	if stats.Fail > 0 {
		return 2
	}
	return 0
}

func TaskRunnerCLI(args []string, out, errOut io.Writer, service *Service) int {
	return TaskRunnerCLIContext(context.Background(), args, out, errOut, service)
}

func TaskRunnerCLIContext(
	ctx context.Context,
	args []string,
	out, errOut io.Writer,
	service *Service,
) (code int) {
	defer func() {
		if recovered := recover(); recovered != nil {
			fmt.Fprintf(errOut, "ERR: task_runner panic: %v\n", recovered)
			code = 1
		}
	}()
	if service == nil {
		service = NewService()
	}
	opts := TaskRunnerOptions{Gap: 1}
	selectors := []string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			selectors = append(selectors, args[i+1:]...)
			i = len(args)
		case arg == "--yes":
			opts.Yes = true
		case arg == "--only-claim":
			opts.OnlyClaim = true
		case arg == "--help" || arg == "-h":
			fmt.Fprint(out, taskRunnerUsage)
			return 0
		case arg == "--only":
			value, next, ok := flagValue(args, i)
			if !ok {
				return cliArgError(errOut, "--only requires a value")
			}
			opts.OnlyCodes, i = append(opts.OnlyCodes, value), next
		case strings.HasPrefix(arg, "--only="):
			opts.OnlyCodes = append(opts.OnlyCodes, strings.TrimPrefix(arg, "--only="))
		case arg == "--gap":
			value, next, ok := flagValue(args, i)
			if !ok {
				return cliArgError(errOut, "--gap requires a value")
			}
			gap, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return cliArgError(errOut, "invalid float value for --gap")
			}
			opts.Gap, i = gap, next
		case strings.HasPrefix(arg, "--gap="):
			gap, err := strconv.ParseFloat(strings.TrimPrefix(arg, "--gap="), 64)
			if err != nil {
				return cliArgError(errOut, "invalid float value for --gap")
			}
			opts.Gap = gap
		case strings.HasPrefix(arg, "-"):
			return cliArgError(errOut, "unrecognized arguments: "+arg)
		default:
			selectors = append(selectors, arg)
		}
	}
	if len(selectors) == 0 {
		return cliArgError(errOut, "the following arguments are required: accounts")
	}
	if opts.Gap < 1 {
		opts.Gap = 1
	}

	expanded := []string{}
	for _, selector := range selectors {
		if strings.EqualFold(selector, "ALL") {
			prefixes, err := service.allAuthPrefixes()
			if err != nil {
				fmt.Fprintf(out, "ERR: %v\n", err)
			}
			expanded = append(expanded, prefixes...)
		} else {
			expanded = append(expanded, selector)
		}
	}
	expanded = dedupStrings(expanded)
	mode := "DRY-RUN"
	if opts.Yes {
		mode = "REAL"
	}
	fmt.Fprintf(out, "mode=%s accounts=%s only=%s only_claim=%s gap=%s\n",
		mode, formatQuotedList(expanded), formatOnly(opts.OnlyCodes), legacyValue(opts.OnlyClaim), formatFloat(opts.Gap))
	if opts.Yes && len(expanded) > 2 && len(opts.OnlyCodes) == 0 && !opts.OnlyClaim {
		fmt.Fprintln(out, "WARN: 全量批量 + --yes 未限定 --only，注意 54 号批量——请确认 Hermes 决策后再跑")
	}

	loaded := []Account{}
	for _, selector := range expanded {
		account, err := service.loadAccount(selector)
		if err != nil {
			fmt.Fprintf(out, "ERR: %v\n", err)
			continue
		}
		loaded = append(loaded, account)
	}
	service.RunTaskRunnerAccounts(ctx, loaded, opts, out)
	return 0
}

func flagValue(args []string, index int) (string, int, bool) {
	if index+1 >= len(args) {
		return "", index, false
	}
	value := args[index+1]
	if strings.HasPrefix(value, "-") && value != "-" {
		numeric := len(value) > 1 && ((value[1] >= '0' && value[1] <= '9') || value[1] == '.')
		if !numeric {
			return "", index, false
		}
	}
	return value, index + 1, true
}

func cliArgError(out io.Writer, message string) int {
	fmt.Fprintln(out, "error:", message)
	return 2
}

func dedupStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func formatQuotedList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, "'"+value+"'")
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func formatOnly(values []string) string {
	if len(values) == 0 {
		return "all"
	}
	return formatQuotedList(values)
}

func formatFloat(value float64) string {
	if math.IsNaN(value) {
		return "nan"
	}
	if math.IsInf(value, 1) {
		return "inf"
	}
	if math.IsInf(value, -1) {
		return "-inf"
	}
	text := strconv.FormatFloat(value, 'g', -1, 64)
	if !strings.ContainsAny(text, ".eE") {
		text += ".0"
	}
	return text
}

const schoolUsage = `usage: school_open_day_2026 [-h] [--list] [--run] [--lottery-only] [--yes]
                           [--token TOKEN] [--uid UID] [--gap GAP]
                           [accounts ...]

小程序开学季任务中心：只读盘点 + 事件上报自动化
`

const taskRunnerUsage = `usage: task_runner [-h] [--yes] [--only ONLY_CODES]
                   [--only-claim] [--gap GAP]
                   accounts [accounts ...]

成长任务一体机：查询→完成(点亮)→领取
`
