package tasks

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

func (s *Service) processSchoolTasks(
	ctx context.Context,
	a Account,
	opts TaskRunnerOptions,
	stats *Stats,
	out io.Writer,
) {
	uid8 := a.UID8()
	tasks, inPeriod, err := s.fetchSchoolTasks(ctx, out, a, opts.Gap)
	if err != nil {
		fmt.Fprintf(out, "[task_runner] %s school/tasks 拉取失败: %v\n", uid8, err)
		stats.Fail++
		return
	}
	if !inPeriod {
		fmt.Fprintf(out, "[task_runner] %s school 活动非进行期（in_period=false），school 段跳过\n", uid8)
		return
	}
	fmt.Fprintf(out, "[task_runner] %s school/tasks in_period=%s tasks=%d\n",
		uid8, legacyValue(inPeriod), len(tasks))
	for _, code := range schoolCodes {
		task := findTask(tasks, code)
		if task == nil {
			continue
		}
		if len(opts.OnlyCodes) > 0 && !contains(opts.OnlyCodes, code) {
			continue
		}
		s.processSchoolTask(ctx, a, code, task, opts, stats, out)
	}
}

func (s *Service) processSchoolTask(
	ctx context.Context,
	a Account,
	code string,
	task map[string]any,
	opts TaskRunnerOptions,
	stats *Stats,
	out io.Writer,
) {
	uid8 := a.UID8()
	refetch := func() map[string]any {
		tasks, _, err := s.fetchSchoolTasks(ctx, out, a, opts.Gap)
		if err != nil {
			return nil
		}
		return findTask(tasks, code)
	}
	status := strings.ToLower(fallback(stringValue(task["status"]), "?"))
	current := int(number(task["progress"]))
	target := int(number(task["target_count"]))
	if target == 0 {
		target = 1
	}
	reward := int(number(task["reward_credit"]))
	stats.Total++

	if status == "completed" || status == "claimed" {
		fmt.Fprintf(out, "[task_runner] %s %s: query %s(%d/%d) -> 已完成/已领，跳过\n",
			uid8, code, status, current, target)
		stats.Already++
		return
	}
	reportKind, ok := schoolReportKind[code]
	if !ok {
		fmt.Fprintf(out, "[task_runner] %s %s: query %s -> school 未映射(人工/未知)，skip\n",
			uid8, code, status)
		stats.Skip++
		return
	}
	if !opts.Yes {
		fmt.Fprintf(out, "[task_runner] %s %s: query %s(%d/%d) reward=%d -> school %s 判据（dry-run 跳过）\n",
			uid8, code, status, current, target, reward, reportKind)
		stats.Pending++
		return
	}
	if status == "pending" {
		st, body, err := s.postSchoolViewed(ctx, out, a, code, opts.Gap)
		if err != nil {
			fmt.Fprintf(out, "[task_runner] %s %s: viewed 失败: %v\n", uid8, code, err)
			stats.Fail++
			return
		}
		fmt.Fprintf(out, "[task_runner] %s %s: viewed 激活 %d code=%s\n",
			uid8, code, st, legacyValue(codeOf(body)))
		if !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
			return
		}
	}

	rounds := 1
	if reportKind != "share" && reportKind != "desktop_seq" {
		rounds = target - current
		if rounds < 1 {
			rounds = 1
		}
	}
	progressed := false
	for round := 0; round < rounds; round++ {
		if reportKind == "share" {
			st, body, err := s.postSchoolShare(ctx, out, a, opts.Gap)
			if err != nil {
				fmt.Fprintf(out, "[task_runner] %s %s: report 失败 #%d: %v\n", uid8, code, round+1, err)
				stats.Fail++
				break
			}
			fmt.Fprintf(out, "[task_runner] %s %s: report %d/%d %d code=%s (%s)\n",
				uid8, code, round+1, rounds, st, legacyValue(codeOf(body)), reportKind)
		} else {
			events, host, headers, err := s.makeSchoolReportEvents(ctx, out, a, reportKind, opts.Gap)
			if err != nil {
				fmt.Fprintf(out, "[task_runner] %s %s: report 失败 #%d: %v\n", uid8, code, round+1, err)
				stats.Fail++
				break
			}
			st, body, err := s.reportSchoolEvents(ctx, out, a, events, host, headers, opts.Gap)
			if err != nil {
				fmt.Fprintf(out, "[task_runner] %s %s: report 失败 #%d: %v\n", uid8, code, round+1, err)
				stats.Fail++
				break
			}
			fmt.Fprintf(out, "[task_runner] %s %s: report %d/%d %d code=%s (%s)\n",
				uid8, code, round+1, rounds, st, legacyValue(codeOf(body)), reportKind)
		}
		if !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
			return
		}
		currentTask := refetch()
		if currentTask == nil {
			continue
		}
		after := strings.ToLower(stringValue(currentTask["status"]))
		afterProgress := int(number(currentTask["progress"]))
		if afterProgress > current {
			current = afterProgress
			progressed = true
		}
		if after == "completed" || after == "claimed" || afterProgress >= target {
			break
		}
	}

	currentTask := refetch()
	if currentTask == nil {
		currentTask = task
	}
	after := strings.ToLower(stringValue(currentTask["status"]))
	afterProgress := int(number(currentTask["progress"]))
	if after == "claimed" {
		fmt.Fprintf(out, "[task_runner] %s %s: %s/%d -> claimed（本轮已入账）\n",
			uid8, code, status, current)
		stats.Already++
		return
	}
	if after == "completed" || afterProgress >= target {
		fmt.Fprintf(out, "[task_runner] %s %s: %s -> %s/%d（点亮）\n",
			uid8, code, status, after, afterProgress)
		stats.OK++
		st, body, err := s.postSchoolClaim(ctx, out, a, code, opts.Gap)
		if err != nil {
			fmt.Fprintf(out, "[task_runner] %s %s: claim 失败: %v（可稍后补领）\n", uid8, code, err)
			stats.Fail++
		} else {
			fmt.Fprintf(out, "[task_runner] %s %s: claim %d code=%s\n",
				uid8, code, st, legacyValue(codeOf(body)))
			if !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
				return
			}
		}
		return
	}
	if progressed {
		fmt.Fprintf(out, "[task_runner] %s %s: %s -> %s/%d（部分点亮，未达 target）\n",
			uid8, code, status, after, afterProgress)
		stats.OK++
	} else {
		fmt.Fprintf(out, "[task_runner] %s %s: %s -> %s/%d（未变化）\n",
			uid8, code, status, after, afterProgress)
		stats.Pending++
	}
}

func (s *Service) processMinichatTask(
	ctx context.Context,
	a Account,
	code string,
	task map[string]any,
	opts TaskRunnerOptions,
	stats *Stats,
	out io.Writer,
) {
	uid8 := a.UID8()
	currentTask, err := s.mpTaskStatus(ctx, a, code)
	if err != nil {
		fmt.Fprintf(out, "[task_runner] %s %s: mp list_tasks 失败: %v\n", uid8, code, err)
		stats.Fail++
		return
	}
	task = currentTask
	if task == nil {
		fmt.Fprintf(out, "[task_runner] %s %s: mp 口径任务不存在，skip\n", uid8, code)
		stats.Skip++
		return
	}
	spec, ok := specByCode(code)
	if !ok {
		spec = taskSpec{Kind: "minichat"}
	}
	status := stringValue(task["accept_status"])
	progress := object(task["progress"])
	current := int(number(progress["current"]))
	target := int(number(progress["target"]))
	if target == 0 {
		target = 1
	}
	stats.Total++

	if status == "claimed" {
		fmt.Fprintf(out, "[task_runner] %s %s: query claimed(%d/%d) -> 已领，跳过\n",
			uid8, code, current, target)
		stats.Already++
		return
	}
	if status == "completed" || current >= target {
		if !opts.Yes {
			fmt.Fprintf(out, "[task_runner] %s %s: query completed(%d/%d) -> 可领(claim)，dry-run 跳过\n",
				uid8, code, current, target)
			stats.Pending++
			return
		}
		s.claimOne(ctx, a, code, uid8, stats, opts.Gap, true, out)
		return
	}
	if !opts.Yes {
		fmt.Fprintf(out, "[task_runner] %s %s: query %s(%d/%d) -> 可点亮(%s)，dry-run 跳过\n",
			uid8, code, status, current, target, spec.Kind)
		stats.Pending++
		return
	}
	if status == "not_accepted" {
		st, body := s.acceptTasks(ctx, a, []string{code}, true)
		results := array(object(body["data"])["results"])
		result := ""
		if len(results) > 0 {
			result = stringValue(object(results[0])["status"])
		}
		fmt.Fprintf(out, "[task_runner] %s %s: accept %d %s\n", uid8, code, st, result)
		if !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
			return
		}
		if st != 200 || result != "accepted" {
			stats.Fail++
			return
		}
	}

	need := target - current
	if need < 1 {
		need = 1
	}
	for i := 0; i < need; i++ {
		event := buildGrowthEvent(a, s.Runtime.millis(), spec.Kind, "", map[string]any{}, i)
		st, body, err := s.reportSchoolEvents(ctx, out, a, []map[string]any{event}, s.CloudAgent, nil, opts.Gap)
		if err != nil {
			fmt.Fprintf(out, "[task_runner] %s %s: report %d/%d 失败: %v\n", uid8, code, i+1, need, err)
			stats.Fail++
			return
		}
		fmt.Fprintf(out, "[task_runner] %s %s: report %d/%d %d code=%s (mini growth)\n",
			uid8, code, i+1, need, st, legacyValue(codeOf(body)))
		if !s.Runtime.sleep(ctx, seconds(opts.Gap)) {
			return
		}
	}
	if !s.Runtime.sleep(ctx, 2*time.Second) {
		return
	}

	currentTask, _ = s.mpTaskStatus(ctx, a, code)
	if currentTask == nil {
		currentTask = task
	}
	after := stringValue(currentTask["accept_status"])
	afterProgress := object(currentTask["progress"])
	current2 := int(number(afterProgress["current"]))
	target2 := int(number(afterProgress["target"]))
	if target2 == 0 {
		target2 = target
	}
	fmt.Fprintf(out, "[task_runner] %s %s: query re-read %d/%d accept_status=%s\n",
		uid8, code, current2, target2, after)
	if after == "claimed" {
		fmt.Fprintf(out, "[task_runner] %s %s: %s -> claimed（本轮已入账）\n", uid8, code, status)
		stats.Already++
		return
	}
	if after == "completed" || current2 >= target2 {
		s.claimOne(ctx, a, code, uid8, stats, opts.Gap, true, out)
		return
	}
	fmt.Fprintf(out, "[task_runner] %s %s: report 未达 target（%d/%d），WARN 待下次\n",
		uid8, code, current2, target2)
	stats.Pending++
}
