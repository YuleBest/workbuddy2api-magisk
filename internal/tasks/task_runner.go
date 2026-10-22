package tasks

import (
	"context"
	"fmt"
	"io"
	"time"
)

type TaskRunnerOptions struct {
	Yes       bool
	OnlyCodes []string
	OnlyClaim bool
	Gap       float64
}

const libraryDocURL = "https://www.workbuddy.cn/space/d/o0KWYeynteVv06UnAZqIFm"

func (s *Service) RunTaskRunnerAccounts(
	ctx context.Context,
	accounts []Account,
	opts TaskRunnerOptions,
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
		stats.Accounts++
		if account.IsGlobal() {
			fmt.Fprintf(out, "[skip] %s global realm 不适用 CN 任务\n", account.UID8())
			stats.Skip++
			continue
		}
		s.processAccount(ctx, account, opts, &stats, out)
	}
	s.printTaskRunnerSummary(stats, out)
	return stats
}

func (s *Service) RunCatAll(ctx context.Context, out io.Writer) Stats {
	accounts, loadErrs, err := s.allAccountResults()
	if err != nil {
		fmt.Fprintf(out, "ERR: %v\n", err)
	}
	for _, loadErr := range loadErrs {
		fmt.Fprintf(out, "ERR: %v\n", loadErr)
	}
	return s.RunTaskRunnerAccounts(ctx, accounts, TaskRunnerOptions{
		Yes: true, OnlyCodes: []string{"black_cat"}, Gap: 1,
	}, out)
}

func (s *Service) processAccount(
	ctx context.Context,
	a Account,
	opts TaskRunnerOptions,
	stats *Stats,
	out io.Writer,
) {
	uid8 := a.UID8()
	fmt.Fprintf(out, "== %s (%s) ==\n", uid8, a.Nickname)
	stEnergy, bodyEnergy := s.doGrowth(ctx, a, "GET", s.ChatBase, "/v2/activity/growth/energy", nil, nil)
	if stEnergy == 200 {
		code, numeric := numericNumber(codeOf(bodyEnergy))
		if numeric && code == 0 {
			balance := any("?")
			if value, ok := object(bodyEnergy["data"])["balance"]; ok {
				balance = value
			}
			fmt.Fprintf(out, "[task_runner] %s query energy balance=%v\n", uid8, balance)
		}
	}

	tasks, err := s.listTasks(ctx, a, false)
	if err != nil {
		fmt.Fprintf(out, "ERR: [task_runner] %s query list_tasks 失败: %v\n", uid8, err)
		stats.Fail++
		return
	}
	byCode := map[string]map[string]any{}
	for _, task := range tasks {
		byCode[stringValue(task["task_code"])] = task
	}

	special := map[string]bool{}
	for _, code := range schoolCodes {
		special[code] = true
	}
	for _, code := range minichatCodes {
		special[code] = true
	}

	codes := []string{}
	processSchool := false
	processMinichat := false
	if len(opts.OnlyCodes) > 0 {
		for _, code := range opts.OnlyCodes {
			if !special[code] {
				codes = append(codes, code)
			}
		}
		for _, code := range schoolCodes {
			if contains(opts.OnlyCodes, code) {
				processSchool = true
			}
		}
		for _, code := range minichatCodes {
			if contains(opts.OnlyCodes, code) {
				processMinichat = true
			}
		}
	} else {
		for _, spec := range taskSpecs {
			if spec.Kind != "school" && spec.Kind != "minichat" && spec.Kind != "schoolseason" {
				codes = append(codes, spec.Code)
			}
		}
		processSchool, processMinichat = true, true
		for _, task := range tasks {
			code := stringValue(task["task_code"])
			if code == "" {
				continue
			}
			if _, ok := specByCode(code); ok {
				continue
			}
			stats.Total++
			if stringValue(task["accept_status"]) == "claimed" {
				fmt.Fprintf(out, "[task_runner] %s %s: query claimed -> 已领，跳过(非映射任务)\n", uid8, code)
				stats.Already++
			} else {
				fmt.Fprintf(out, "[task_runner] %s %s: query %s -> 非映射任务，skip\n",
					uid8, code, stringValue(task["accept_status"]))
				stats.Skip++
			}
		}
	}

	for _, code := range codes {
		task := byCode[code]
		if task == nil {
			fmt.Fprintf(out, "[task_runner] %s %s: query 任务不存在\n", uid8, code)
			stats.Total++
			stats.Skip++
			continue
		}
		s.processTask(ctx, a, code, task, opts, stats, out)
	}

	if processMinichat && !(opts.OnlyClaim && !opts.Yes) {
		mpCodes := minichatCodes
		if len(opts.OnlyCodes) > 0 {
			mpCodes = nil
			for _, code := range opts.OnlyCodes {
				if contains(minichatCodes, code) {
					mpCodes = append(mpCodes, code)
				}
			}
		}
		for _, code := range mpCodes {
			task, err := s.mpTaskStatus(ctx, a, code)
			if err != nil {
				fmt.Fprintf(out, "[task_runner] %s %s: mp 查询失败: %v\n", uid8, code, err)
				stats.Fail++
				continue
			}
			status := ""
			if task != nil {
				status = stringValue(task["accept_status"])
			}
			if opts.OnlyClaim && status != "completed" && status != "claimed" {
				fmt.Fprintf(out, "[task_runner] %s %s: only_claim 跳过（未 completed）\n", uid8, code)
			} else {
				s.processMinichatTask(ctx, a, code, task, opts, stats, out)
			}
		}
	}

	if processSchool && !opts.OnlyClaim {
		s.processSchoolTasks(ctx, a, opts, stats, out)
	}
}

func (s *Service) processTask(
	ctx context.Context,
	a Account,
	code string,
	task map[string]any,
	opts TaskRunnerOptions,
	stats *Stats,
	out io.Writer,
) {
	uid8 := a.UID8()
	spec, ok := specByCode(code)
	if !ok {
		fmt.Fprintf(out, "[task_runner] %s %s: query 非映射任务，skip\n", uid8, code)
		stats.Total++
		stats.Skip++
		return
	}
	status := stringValue(task["accept_status"])
	progress := object(task["progress"])
	current := int(number(progress["current"]))
	target := int(number(progress["target"]))
	if target == 0 {
		target = spec.Target
	}
	if target == 0 {
		target = 1
	}
	stats.Total++

	if spec.Unforgeable {
		fmt.Fprintf(out, "[task_runner] %s %s: query %s(%d/%d) -> 不可伪造(%s)，skip\n",
			uid8, code, status, current, target, spec.Reason)
		stats.Skip++
		return
	}
	if opts.OnlyClaim {
		if status != "claimed" && current < target && status != "completed" {
			fmt.Fprintf(out, "[task_runner] %s %s: query %s(%d/%d) -> 未 completed，only_claim 跳过\n",
				uid8, code, status, current, target)
			stats.Pending++
			return
		}
		if !opts.Yes {
			fmt.Fprintf(out, "[task_runner] %s %s: query %s(%d/%d) -> only_claim dry-run 跳过\n",
				uid8, code, status, current, target)
			stats.Pending++
			return
		}
		s.claimOne(ctx, a, code, uid8, stats, opts.Gap, false, out)
		return
	}
	if status == "claimed" {
		fmt.Fprintf(out, "[task_runner] %s %s: query claimed(%d/%d) -> 已领，跳过\n",
			uid8, code, current, target)
		stats.Already++
		return
	}
	if current >= target || status == "completed" {
		if !opts.Yes {
			fmt.Fprintf(out, "[task_runner] %s %s: query completed(%d/%d) -> 可领(claim)，dry-run 跳过\n",
				uid8, code, current, target)
			stats.Pending++
			return
		}
		s.claimOne(ctx, a, code, uid8, stats, opts.Gap, false, out)
		return
	}
	if code == "black_cat" {
		if !s.withinNightWindow() {
			fmt.Fprintf(out, "[task_runner] %s %s: query in_progress(%d/%d) -> 非夜猫窗口(23-08 CST)，skip pending\n",
				uid8, code, current, target)
			stats.Pending++
			return
		}
		if !opts.Yes {
			fmt.Fprintf(out, "[task_runner] %s %s: query in_progress(%d/%d) -> 夜猫窗口内，可补 1 次，dry-run 跳过\n",
				uid8, code, current, target)
			stats.Pending++
			return
		}
		s.lightUp(ctx, a, code, spec, current, target, opts, stats, 1, out)
		return
	}
	need := target - current
	if need <= 0 {
		if !opts.Yes {
			fmt.Fprintf(out, "[task_runner] %s %s: query %s(%d/%d) -> 可领(claim)，dry-run 跳过\n",
				uid8, code, status, current, target)
			stats.Already++
			return
		}
		s.claimOne(ctx, a, code, uid8, stats, opts.Gap, false, out)
		return
	}
	if !opts.Yes {
		ids := s.idsFor(ctx, a, spec.Kind, need, current, out)
		shown := []string{}
		for i, item := range ids {
			if i >= 6 {
				break
			}
			shown = append(shown, item.ID)
		}
		fmt.Fprintf(out, "[task_runner] %s %s: query %s(%d/%d) -> 可点亮 need=%d id源=%s ids=%v，dry-run 跳过\n",
			uid8, code, status, current, target, need, spec.Source, shown)
		stats.Pending++
		return
	}
	s.lightUp(ctx, a, code, spec, current, target, opts, stats, 0, out)
}

func (s *Service) withinNightWindow() bool {
	now := s.Runtime.normalized().Now().In(time.FixedZone("CST", 8*60*60))
	return now.Hour() >= 23 || now.Hour() < 8
}

func (s *Service) lightUp(
	ctx context.Context,
	a Account,
	code string,
	spec taskSpec,
	current, target int,
	opts TaskRunnerOptions,
	stats *Stats,
	cap int,
	out io.Writer,
) {
	uid8 := a.UID8()
	task, _ := s.taskStatus(ctx, a, code)
	acceptStatus := "not_accepted"
	if task != nil && stringValue(task["accept_status"]) != "" {
		acceptStatus = stringValue(task["accept_status"])
	}
	if acceptStatus == "not_accepted" {
		if !s.acceptWithVerify(ctx, a, code, uid8, opts.Gap, out) {
			fmt.Fprintf(out, "[task_runner] %s %s: accept 未登记生效，本轮跳过待下次\n", uid8, code)
			stats.Fail++
			return
		}
	}
	currentTask, _ := s.taskStatus(ctx, a, code)
	if currentTask == nil {
		currentTask = map[string]any{}
	}
	progress := object(currentTask["progress"])
	if value := int(number(progress["current"])); value != 0 {
		current = value
	}
	if value := int(number(progress["target"])); value != 0 {
		target = value
	}
	need := target - current
	if need < 0 {
		need = 0
	}
	if cap > 0 && need > cap {
		need = cap
	}

	switch {
	case need <= 0:
		fmt.Fprintf(out, "[task_runner] %s %s: report 无需上报（%d/%d）\n", uid8, code, current, target)
	case spec.Kind == "buddyfirst":
		reports := s.reportActivity(ctx, a, 1, opts.Gap, "deepseek-v4-flash", "DeepSeek V4 Flash", "craft")
		if len(reports) > 0 {
			fmt.Fprintf(out, "[task_runner] %s %s: report %v code=%s 前置解锁\n",
				uid8, code, reports[0][0], legacyValue(reports[0][1]))
		}
		if !s.Runtime.sleep(ctx, 2*time.Second) {
			return
		}
		stA, agreement := s.doGrowth(ctx, a, "POST", s.ChatBase,
			"/activity/growth/buddy/agreement", map[string]any{"agree": true}, nil)
		fmt.Fprintf(out, "[task_runner] %s %s: buddy/agreement -> %d code=%s\n",
			uid8, code, stA, legacyValue(codeOf(agreement)))
		if !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
			return
		}
		stB, first := s.doGrowth(ctx, a, "POST", s.ChatBase,
			"/activity/growth/buddy/first", map[string]any{}, nil)
		data := object(first["data"])
		credit := int(number(data["credit"]))
		energy := int(number(data["energy"]))
		fmt.Fprintf(out, "[task_runner] %s %s: buddy/first -> %d %s credit=+%d energy=+%d\n",
			uid8, code, stB, stringValue(first["msg"]), credit, energy)
		if stB == 200 && codeZeroOrNil(codeOf(first)) {
			stats.OK++
			stats.Credit += credit
			stats.Energy += energy
		}
	case spec.Kind == "richmeow":
		for i := 0; i < need; i++ {
			now := s.Runtime.millis()
			conv := fmt.Sprintf("wb-run-rm-%d-%d", now, i)
			requestID := fmt.Sprintf("wb-run-rm-req-%d-%d", now, i)
			messageID := fmt.Sprintf("req-%d-%d-user", now, i)
			events := desktopChatSequence(a, now, conv, requestID, messageID, "fast-model", "fast-model")
			st, codeValue := s.reportDesktopEvents(ctx, a, events)
			fmt.Fprintf(out, "[task_runner] %s %s: report %d/%d %d code=%s deskchain conv=%s\n",
				uid8, code, i+1, need, st, legacyValue(codeValue), conv)
			if i < need-1 && !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
				return
			}
		}
	case spec.Kind == "buddy5":
		buddyID, buddyName := "cb_y5Dy46tPQGGWtueMxXbe", "企鹅教师助手"
		for i := 0; i < need; i++ {
			events := desktopBuddy5Sequence(a, buddyID, buddyName)
			st, codeValue := s.reportDesktopEvents(ctx, a, events)
			codeNumber, numeric := numericNumber(codeValue)
			if st != 200 || !numeric || codeNumber != 0 {
				now := s.Runtime.millis()
				single := map[string]any{
					"eventCode": "buddyapp_enter_click", "timestamp": now,
					"reportDelay": 0, "elementId": buddyID,
					"elementName": buddyName, "position": 1,
					"isFirstPage": "0", "userId": a.UID,
				}
				st2, code2 := s.reportDesktopEvents(ctx, a, []map[string]any{single})
				fmt.Fprintf(out, "[task_runner] %s %s: report %d/%d %d code=%s buddy5 失败 -> 降级单发 %d code=%s\n",
					uid8, code, i+1, need, st, legacyValue(codeValue), st2, legacyValue(code2))
			} else {
				fmt.Fprintf(out, "[task_runner] %s %s: report %d/%d %d code=%s buddy5 id=%s\n",
					uid8, code, i+1, need, st, legacyValue(codeValue), buddyID)
			}
			if i < need-1 && !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
				return
			}
		}
	case spec.Kind == "library":
		for i := 0; i < need; i++ {
			st, codeValue := s.reportWebEvent(ctx, a, "web_element_click", libraryDocURL,
				"library_doc_intro_click", "WorkBuddy资料库介绍")
			fmt.Fprintf(out, "[task_runner] %s %s: report %d/%d %d code=%s web_element_click\n",
				uid8, code, i+1, need, st, legacyValue(codeValue))
			if i < need-1 && !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
				return
			}
		}
	default:
		ids := s.idsFor(ctx, a, spec.Kind, need, current, out)
		if len(ids) == 0 {
			fmt.Fprintf(out, "[task_runner] %s %s: report 无可用对象 id，skip\n", uid8, code)
			stats.Fail++
			return
		}
		for i, item := range ids {
			event := buildGrowthEvent(a, s.Runtime.millis(), spec.Kind, item.ID, item.Meta, i)
			st, body := s.doGrowth(ctx, a, "POST", s.BillingBase, "/v2/report",
				[]map[string]any{event}, nil)
			fmt.Fprintf(out, "[task_runner] %s %s: report %d/%d %d code=%s id=%s\n",
				uid8, code, i+1, need, st, legacyValue(codeOf(body)), item.ID)
			if i < len(ids)-1 && !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
				return
			}
		}
	}
	if !s.Runtime.sleep(ctx, 2*time.Second) {
		return
	}

	currentTask, _ = s.taskStatus(ctx, a, code)
	progress = object(currentTask["progress"])
	current2 := int(number(progress["current"]))
	target2 := int(number(progress["target"]))
	if target2 == 0 {
		target2 = target
	}
	accept2 := "?"
	if currentTask != nil {
		accept2 = fallback(stringValue(currentTask["accept_status"]), "?")
	}
	fmt.Fprintf(out, "[task_runner] %s %s: query re-read %d/%d accept_status=%s\n",
		uid8, code, current2, target2, accept2)
	if accept2 != "claimed" && current2 >= target2 {
		s.claimOne(ctx, a, code, uid8, stats, opts.Gap, false, out)
	} else if accept2 != "claimed" {
		fmt.Fprintf(out, "[task_runner] %s %s: report 未达 target（%d/%d），WARN 待下次\n",
			uid8, code, current2, target2)
	}
}

func (s *Service) printTaskRunnerSummary(stats Stats, out io.Writer) {
	fmt.Fprintf(out, "task_runner done: accounts=%d total=%d ok=%d already=%d skipped=%d pending=%d fail=%d credit=+%d energy=+%d\n",
		stats.Accounts, stats.Total, stats.OK, stats.Already, stats.Skip, stats.Pending,
		stats.Fail, stats.Credit, stats.Energy)
}
