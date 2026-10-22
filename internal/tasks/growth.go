package tasks

import (
	"context"
	"fmt"
	"io"
	"time"
)

type Stats struct {
	Accounts int
	Total    int
	OK       int
	Already  int
	Skip     int
	Pending  int
	Fail     int
	Credit   int
	Energy   int
}

func seconds(v float64) time.Duration {
	return time.Duration(v * float64(time.Second))
}

func (s *Service) listTasks(ctx context.Context, a Account, mp bool) ([]map[string]any, error) {
	var headers map[string]string
	if mp {
		headers = map[string]string{"X-Client-Platform": "miniprogram"}
	}
	st, body := s.doGrowth(ctx, a, "GET", s.ChatBase, "/v2/activity/growth/tasks", nil, headers)
	if st != 200 {
		return nil, fmt.Errorf("list_tasks http=%d", st)
	}
	return taskList(body), nil
}

func taskList(body map[string]any) []map[string]any {
	data := object(body["data"])
	raw := array(data["tasks"])
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func findTask(tasks []map[string]any, code string) map[string]any {
	for _, task := range tasks {
		if stringValue(task["task_code"]) == code {
			return task
		}
	}
	return nil
}

func (s *Service) taskStatus(ctx context.Context, a Account, code string) (map[string]any, error) {
	tasks, err := s.listTasks(ctx, a, false)
	if err != nil {
		return nil, err
	}
	return findTask(tasks, code), nil
}

func (s *Service) acceptTasks(ctx context.Context, a Account, codes []string, mp bool) (int, map[string]any) {
	var headers map[string]string
	if mp {
		headers = map[string]string{"X-Client-Platform": "miniprogram"}
	}
	return s.doGrowth(ctx, a, "POST", s.ChatBase, "/v2/activity/growth/tasks/accept",
		map[string]any{"task_codes": codes}, headers)
}

func (s *Service) mpTaskStatus(ctx context.Context, a Account, code string) (map[string]any, error) {
	tasks, err := s.listTasks(ctx, a, true)
	if err != nil {
		return nil, err
	}
	return findTask(tasks, code), nil
}

func (s *Service) acceptWithVerify(
	ctx context.Context,
	a Account,
	code, uid8 string,
	gap float64,
	out io.Writer,
) bool {
	for attempt := 1; attempt <= 2; attempt++ {
		st, body := s.acceptTasks(ctx, a, []string{code}, false)
		status := ""
		if body != nil {
			results := array(object(body["data"])["results"])
			if len(results) > 0 {
				status = stringValue(object(results[0])["status"])
			} else {
				status = stringValue(body["msg"])
			}
		}
		task, _ := s.taskStatus(ctx, a, code)
		acceptStatus := "not_accepted"
		if task != nil && stringValue(task["accept_status"]) != "" {
			acceptStatus = stringValue(task["accept_status"])
		}
		ok := st == 200 && status == "accepted" && acceptStatus != "not_accepted"
		suffix := ""
		if ok {
			suffix = " -> 生效"
		}
		fmt.Fprintf(out, "[task_runner] %s %s: accept 尝试%d %d status=%s 回读=%s%s\n",
			uid8, code, attempt, st, status, acceptStatus, suffix)
		if ok {
			return true
		}
		if !s.Runtime.sleep(ctx, seconds(gap)) {
			return false
		}
	}
	return false
}

func (s *Service) claimOne(
	ctx context.Context,
	a Account,
	code, uid8 string,
	stats *Stats,
	gap float64,
	mp bool,
	out io.Writer,
) int {
	var headers map[string]string
	if mp {
		headers = map[string]string{"X-Client-Platform": "miniprogram"}
	}
	path := "/activity/growth/tasks/" + code + "/claim"
	st, body := s.doGrowth(ctx, a, "POST", s.ChatBase, path, nil, headers)
	if st != 200 || !isOK(body) {
		var msg any
		if body != nil {
			msg = body["msg"]
		}
		if st == 400 {
			fmt.Fprintf(out, "[task_runner] %s %s: claim 400 %s -> 降级 web 域\n",
				uid8, code, legacyValue(msg))
			return s.claimViaWeb(ctx, a, code, uid8, stats, gap, out)
		}
		fmt.Fprintf(out, "[task_runner] %s %s: claim %d %s -> ERR\n", uid8, code, st, legacyValue(msg))
		stats.Fail++
		return 0
	}
	return s.claimParse(ctx, a, code, uid8, stats, gap, st, body, false, out)
}

func (s *Service) claimViaWeb(
	ctx context.Context,
	a Account,
	code, uid8 string,
	stats *Stats,
	gap float64,
	out io.Writer,
) int {
	path := "/activity/growth/tasks/" + code + "/claim"
	st, body := s.doGrowth(ctx, a, "POST", s.WebBase, path, nil, s.webClaimHeaders(a))
	return s.claimParse(ctx, a, code, uid8, stats, gap, st, body, true, out)
}

func (s *Service) claimParse(
	ctx context.Context,
	a Account,
	code, uid8 string,
	stats *Stats,
	gap float64,
	st int,
	body map[string]any,
	viaWeb bool,
	out io.Writer,
) int {
	tag := ""
	if viaWeb {
		tag = " web"
	}
	if st != 200 || !isOK(body) {
		msg := legacyValue(body["msg"])
		fmt.Fprintf(out, "[task_runner] %s %s: claim%s %d %s -> ERR\n", uid8, code, tag, st, msg)
		stats.Fail++
		return 0
	}
	data := object(body["data"])
	credit := int(number(data["credit"]))
	energy := int(number(data["energy"]))
	if truthy(data["already_claimed"]) {
		fmt.Fprintf(out, "[task_runner] %s %s: claim%s 200 already_claimed(credit=+%d energy=+%d)\n",
			uid8, code, tag, credit, energy)
		stats.Already++
		s.Runtime.sleep(ctx, seconds(gap))
		return 2
	}
	fmt.Fprintf(out, "[task_runner] %s %s: claim%s 200 ok(credit=+%d energy=+%d)\n",
		uid8, code, tag, credit, energy)
	stats.OK++
	stats.Credit += credit
	stats.Energy += energy
	s.Runtime.sleep(ctx, seconds(gap))
	return 1
}
