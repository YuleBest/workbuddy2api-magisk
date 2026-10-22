package tasks

import (
	"context"
	"fmt"
	"io"
	"strings"
)

type SchoolOptions struct {
	Run         bool
	LotteryOnly bool
	Yes         bool
	Gap         float64
}

var knownSchoolTasks = map[string]struct {
	Mode       string
	Note       string
	ReportKind string
}{
	"task_student_verify": {"manual", "微信学生认证（人工，不碰）", ""},
	"share_invite":        {"share", "分享活动给好友；share-complete 点亮", ""},
	"chat_3_times":        {"report", "与 AI 对话 3 次；mini chat_request_send+activityId 点亮", "mini_chat"},
	"desktop_chat_1_time": {"report", "桌面端对话 1 次；桌面指纹 6 连+activityId 点亮", "desktop_seq"},
	"expert_use":          {"report", "召唤开学季专家并对话；BackToSchool 分类专家 + expert_actual_use 点亮", "expert"},
}

func (s *Service) RunSchoolAccounts(
	ctx context.Context,
	accounts []Account,
	opts SchoolOptions,
	out io.Writer,
) Stats {
	if opts.Gap < 1 {
		opts.Gap = 1
	}
	var stats Stats
	for _, account := range accounts {
		if ctx.Err() != nil {
			break
		}
		if account.IsGlobal() {
			fmt.Fprintf(out, "[skip] %s global realm 不适用 CN 任务\n", account.UID8())
			stats.Skip++
			continue
		}
		switch {
		case opts.LotteryOnly:
			s.lotteryAccount(ctx, account, opts, &stats, true, out)
		case opts.Run:
			s.runSchoolAccount(ctx, account, opts, &stats, out)
		default:
			s.listSchoolAccount(ctx, account, opts, &stats, out)
		}
	}
	fmt.Fprintf(out, "school2026 done: accounts=%d ok=%d already=%d skipped=%d pending=%d fail=%d\n",
		stats.Accounts, stats.OK, stats.Already, stats.Skip, stats.Pending, stats.Fail)
	s.printSchoolNotes(out)
	return stats
}

func (s *Service) RunSchoolAll(ctx context.Context, out io.Writer) Stats {
	accounts, loadErrs, err := s.allAccountResults()
	if err != nil {
		fmt.Fprintf(out, "ERR: 无可用账号（检查 auths/ 或 --token）: %v\n", err)
		return Stats{Fail: 1}
	}
	for _, loadErr := range loadErrs {
		fmt.Fprintf(out, "ERR: %v\n", loadErr)
	}
	if len(accounts) == 0 {
		fmt.Fprintln(out, "ERR: 无可用账号（检查 auths/ 或 --token）")
		return Stats{Fail: 1}
	}
	return s.RunSchoolAccounts(ctx, accounts, SchoolOptions{Run: true, Yes: true, Gap: 1.5}, out)
}

func (s *Service) listSchoolAccount(
	ctx context.Context,
	a Account,
	opts SchoolOptions,
	stats *Stats,
	out io.Writer,
) {
	fmt.Fprintf(out, "== %s (%s) ==\n", a.UID8(), fallback(a.Nickname, "-"))
	tasks, inPeriod, err := s.fetchSchoolTasks(ctx, out, a, opts.Gap)
	if err != nil {
		fmt.Fprintf(out, "[school2026] %s school/tasks 拉取失败: %v\n", a.UID8(), err)
		stats.Fail++
		tasks, inPeriod = nil, false
	}
	fmt.Fprintf(out, "[school2026] %s school/tasks in_period=%s tasks=%d\n",
		a.UID8(), legacyValue(inPeriod), len(tasks))
	for _, task := range tasks {
		code := fallback(stringValue(task["task_code"]), "?")
		status := fallback(stringValue(task["status"]), "?")
		progress := int(number(task["progress"]))
		target := int(number(task["target_count"]))
		if target == 0 {
			target = 1
		}
		kind := stringValue(task["task_type"])
		if kind == "single" {
			kind = "单次"
		} else if kind == "recurring" {
			kind = "每日"
		}
		spec := knownSchoolTasks[code]
		mode := spec.Mode
		if mode == "" {
			mode = "unknown"
		}
		reward := int(number(task["reward_credit"]))
		fmt.Fprintf(out, "  - %-20s status=%-10s %d/%d reward=%3d [%s/%s] %s\n",
			code, status, progress, target, reward, kind, mode, stringValue(task["title"]))
		if desc := strings.TrimSpace(stringValue(task["description"])); desc != "" {
			fmt.Fprintf(out, "      desc: %s\n", desc)
		}
		if spec.Note != "" {
			fmt.Fprintf(out, "      note: %s\n", spec.Note)
		}
	}
	if len(tasks) == 0 {
		fmt.Fprintln(out, "  (无任务 / 未登录或活动未开启)")
	}
	for _, which := range []string{"freshman", "teacher"} {
		profile, err := s.fetchSchoolProfile(ctx, out, a, which, opts.Gap)
		if err != nil {
			fmt.Fprintf(out, "[school2026] %s %s/profile 拉取失败: %v\n", a.UID8(), which, err)
			stats.Fail++
			continue
		}
		if profile == nil {
			fmt.Fprintf(out, "[school2026] %s %s/profile -> 空\n", a.UID8(), which)
			continue
		}
		fmt.Fprintf(out, "[school2026] %s %s/profile claimed=%s enabled=%s\n",
			a.UID8(), which, legacyValue(profile["claimed"]), legacyValue(profile["enabled"]))
	}
	stats.Accounts++
}

func (s *Service) runSchoolAccount(
	ctx context.Context,
	a Account,
	opts SchoolOptions,
	stats *Stats,
	out io.Writer,
) {
	uid8 := a.UID8()
	stats.Accounts++
	tasks, inPeriod, err := s.fetchSchoolTasks(ctx, out, a, opts.Gap)
	if err != nil {
		fmt.Fprintf(out, "[school2026] %s run 拉取任务失败: %v\n", uid8, err)
		stats.Fail++
		return
	}
	if !inPeriod {
		fmt.Fprintf(out, "[school2026] %s 活动非进行期（in_period=false），跳过\n", uid8)
		return
	}

	refetch := func(code string) map[string]any {
		current, _, err := s.fetchSchoolTasks(ctx, out, a, opts.Gap)
		if err != nil {
			return nil
		}
		return findTask(current, code)
	}

	for _, task := range tasks {
		code := stringValue(task["task_code"])
		if code == "" {
			continue
		}
		status := strings.ToLower(fallback(stringValue(task["status"]), "?"))
		spec := knownSchoolTasks[code]
		mode := spec.Mode
		if mode == "" {
			mode = "unknown"
		}
		if status == "completed" || status == "claimed" {
			fmt.Fprintf(out, "[school2026] %s %s: %s 已完成/已领，跳过\n", uid8, code, status)
			stats.Already++
			continue
		}
		if mode == "manual" {
			fmt.Fprintf(out, "[school2026] %s %s: %s -> 人工环节（%s），跳过\n",
				uid8, code, status, spec.Note)
			stats.Skip++
			continue
		}
		if mode != "share" && mode != "report" {
			fmt.Fprintf(out, "[school2026] %s %s: %s -> 未知任务类型（%s），无分类证据，保守跳过\n",
				uid8, code, status, stringValue(task["title"]))
			stats.Skip++
			continue
		}
		if !opts.Yes {
			fmt.Fprintf(out, "[school2026] %s %s: %s -> %s 动作（dry-run 不写）\n",
				uid8, code, status, mode)
			stats.Pending++
			continue
		}

		if status == "pending" {
			if _, _, err := s.postSchoolViewed(ctx, out, a, code, opts.Gap); err != nil {
				fmt.Fprintf(out, "[school2026] %s %s: viewed 激活失败: %v\n", uid8, code, err)
				stats.Fail++
				continue
			}
			fmt.Fprintf(out, "[school2026] %s %s: viewed 激活 pending->in_progress\n", uid8, code)
			if !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
				return
			}
		}

		currentProgress := int(number(task["progress"]))
		target := int(number(task["target_count"]))
		if target == 0 {
			target = 1
		}
		rounds := 1
		if mode == "report" && spec.ReportKind != "desktop_seq" {
			rounds = target - currentProgress
			if rounds < 1 {
				rounds = 1
			}
		}
		progressChanged := false
		for round := 0; round < rounds; round++ {
			if mode == "share" {
				st, body, err := s.postSchoolShare(ctx, out, a, opts.Gap)
				if err != nil {
					fmt.Fprintf(out, "[school2026] %s %s: share 触发 #%d 失败: %v\n", uid8, code, round+1, err)
					break
				}
				fmt.Fprintf(out, "[school2026] %s %s: share-complete %d code=%s\n",
					uid8, code, st, legacyValue(codeOf(body)))
			} else {
				events, host, headers, err := s.makeSchoolReportEvents(ctx, out, a, spec.ReportKind, opts.Gap)
				if err != nil {
					fmt.Fprintf(out, "[school2026] %s %s: report 触发 #%d 失败: %v\n", uid8, code, round+1, err)
					break
				}
				st, body, err := s.reportSchoolEvents(ctx, out, a, events, host, headers, opts.Gap)
				if err != nil {
					fmt.Fprintf(out, "[school2026] %s %s: report 触发 #%d 失败: %v\n", uid8, code, round+1, err)
					break
				}
				fmt.Fprintf(out, "[school2026] %s %s: report #%d/%d %d 事件 -> http=%d code=%s\n",
					uid8, code, round+1, rounds, len(events), st, legacyValue(codeOf(body)))
			}
			if !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
				return
			}
			current := refetch(code)
			if current == nil {
				continue
			}
			afterProgress := int(number(current["progress"]))
			after := strings.ToLower(stringValue(current["status"]))
			if afterProgress > currentProgress {
				currentProgress = afterProgress
				progressChanged = true
			}
			if after == "completed" || after == "claimed" || afterProgress >= target {
				break
			}
		}
		current := refetch(code)
		if current == nil {
			fmt.Fprintf(out, "[school2026] %s %s: 回读任务失败，跳过\n", uid8, code)
			stats.Fail++
			continue
		}
		after := strings.ToLower(stringValue(current["status"]))
		afterProgress := int(number(current["progress"]))
		if after == "completed" || after == "claimed" || progressChanged {
			fmt.Fprintf(out, "[school2026] %s %s: %s/%v -> %s/%d（点亮）\n",
				uid8, code, status, task["progress"], after, afterProgress)
			stats.OK++
		} else {
			fmt.Fprintf(out, "[school2026] %s %s: %s -> %s（未变化）\n", uid8, code, status, after)
			stats.Pending++
		}
		if after == "completed" {
			st, body, err := s.postSchoolClaim(ctx, out, a, code, opts.Gap)
			if err != nil {
				fmt.Fprintf(out, "[school2026] %s %s: claim 失败: %v（可稍后手动补领）\n", uid8, code, err)
				stats.Fail++
			} else {
				fmt.Fprintf(out, "[school2026] %s %s: claim %d code=%s\n",
					uid8, code, st, legacyValue(codeOf(body)))
				if !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
					return
				}
			}
		}
	}
	s.lotteryAccount(ctx, a, opts, stats, false, out)
}

func (s *Service) lotteryAccount(
	ctx context.Context,
	a Account,
	opts SchoolOptions,
	stats *Stats,
	countAccount bool,
	out io.Writer,
) {
	uid8 := a.UID8()
	if countAccount {
		stats.Accounts++
	}
	if !opts.Yes {
		_, chance, err := s.fetchLotteryConfig(ctx, out, a, opts.Gap)
		if err != nil {
			fmt.Fprintf(out, "[school2026] %s lottery config 失败: %v\n", uid8, err)
			stats.Fail++
			return
		}
		balance := int(number(chance["balance"]))
		if balance <= 0 {
			fmt.Fprintf(out, "[school2026] %s lottery balance=0，无需抽奖\n", uid8)
			return
		}
		fmt.Fprintf(out, "[school2026] %s lottery balance=%d（dry-run：将 POST /wheel/draw 抽 %d 次，间隔 ≥1s，不发请求）\n",
			uid8, balance, balance)
		stats.Pending++
		return
	}

	cfg, chance, err := s.fetchLotteryConfig(ctx, out, a, opts.Gap)
	if err != nil {
		fmt.Fprintf(out, "[school2026] %s lottery config 失败: %v\n", uid8, err)
		stats.Fail++
		return
	}
	if !truthy(cfg["in_period"]) {
		fmt.Fprintf(out, "[school2026] %s 活动非进行期（in_period=false），抽奖跳过\n", uid8)
		return
	}
	balance := int(number(chance["balance"]))
	totalEarned := int(number(chance["total_earned"]))
	limit := "None"
	if value, ok := chance["lottery_limit"]; ok && value != nil {
		limit = legacyValue(value)
	}
	if balance <= 0 {
		fmt.Fprintf(out, "[school2026] %s lottery balance=0（累计获得 %d 次），无需抽奖\n", uid8, totalEarned)
		return
	}
	fmt.Fprintf(out, "[school2026] %s lottery balance=%d total_earned=%d lottery_limit=%s（开始抽奖，抽到空为止）\n",
		uid8, balance, totalEarned, limit)

	type drawResult struct {
		Prize  string
		Credit int
		Label  string
	}
	results := []drawResult{}
	totalCredit := 0
	previous := balance
	stall := 0
	for balance > 0 {
		res, err := s.postLotteryDraw(ctx, out, a, s.Runtime.normalized().UUID(), opts.Gap)
		if err != nil {
			fmt.Fprintf(out, "[school2026] %s draw 失败: %v\n", uid8, err)
			stats.Fail++
			break
		}
		code := stringValue(codeOf(res.Body))
		if res.Status != 200 || !isOK(res.Body) {
			if code == "40900" {
				fmt.Fprintf(out, "[school2026] %s draw http=%d code=40900（次数耗尽：no chance），按边界正常结束\n",
					uid8, res.Status)
				balance = 0
				break
			}
			fmt.Fprintf(out, "[school2026] %s draw http=%d code=%s msg=%s（未中奖，停）\n",
				uid8, res.Status, code, legacyValue(res.Body["message"]))
			break
		}
		data := object(res.Body["data"])
		prize := fallback(stringValue(data["prize_code"]), "?")
		credit := int(number(data["credit_amount"]))
		if value, ok := data["chance_balance"]; ok {
			balance = int(number(value))
		}
		if balance >= previous {
			stall++
			if stall >= 3 {
				fmt.Fprintf(out, "[school2026] %s draw 余额未递减（服务端异常），提前停\n", uid8)
				break
			}
		} else {
			stall = 0
		}
		previous = balance
		label := lotteryPrizeText(prize, credit)
		results = append(results, drawResult{Prize: prize, Credit: credit, Label: label})
		totalCredit += credit
		fmt.Fprintf(out, "[school2026] %s draw -> %s（balance %d）\n", uid8, label, balance)
		if balance > 0 && !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
			break
		}
	}
	if len(results) > 0 {
		credits := make([]string, 0, len(results))
		for _, result := range results {
			credits = append(credits, stringValue(result.Credit))
		}
		fmt.Fprintf(out, "[school2026] %s lottery 汇总: %d 抽，积分增量 %d（%s）\n",
			uid8, len(results), totalCredit, strings.Join(credits, "+"))
		for _, result := range results {
			fmt.Fprintf(out, "    - %s\n", result.Label)
		}
		if balance == 0 {
			stats.OK++
		} else {
			stats.Pending++
		}
	} else {
		fmt.Fprintf(out, "[school2026] %s lottery 无一抽出奖（上次余额 %d）\n", uid8, balance)
	}
}

func (s *Service) printSchoolNotes(out io.Writer) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "[school2026] 说明：非人工任务（share/chat/desktop/expert）自动点亮并领奖；"+
		"task_student_verify 学生认证为人工环节，--run 跳过；"+
		"幸运大转盘将在任务收尾或 --lottery-only 时抽到余额为 0。")
}

func fallback(value, replacement string) string {
	if value == "" {
		return replacement
	}
	return value
}
