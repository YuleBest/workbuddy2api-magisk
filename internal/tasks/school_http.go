package tasks

import (
	"context"
	"fmt"
	"io"
	"time"
)

type schoolHTTPError struct {
	Message string
	Auth    bool
}

func (e *schoolHTTPError) Error() string { return e.Message }

func mpHeaders(a Account, extra map[string]string) map[string]string {
	h := map[string]string{
		"Authorization": "Bearer " + a.Token,
		"Accept":        "application/json",
		"Content-Type":  "application/json",
		"User-Agent":    mpUA,
	}
	return mergeHeaders(h, extra)
}

func interpretSchoolFailure(st int, body map[string]any) (string, bool) {
	code := ""
	if body != nil {
		code = stringValue(body["code"])
	}
	msg := ""
	if body != nil {
		msg = stringValue(body["message"])
		if msg == "" {
			msg = stringValue(body["msg"])
		}
		if msg == "" {
			msg = stringValue(body["raw"])
		}
	}
	switch {
	case st == 401 || code == "401" || code == "40100":
		return "401/40100 未绑微信或 token 失效（小程序会静默刷新，本脚本不刷新）", true
	case st == 403:
		return "403 forbidden", true
	case st == 404:
		return "404 路由不存在", false
	case code == "41000":
		return "41000 活动未开始或已结束", false
	case code == "40901":
		return "40901 任务暂不可领取（未完成或已领）", false
	case code == "83400":
		return "83400 验证码已过期", false
	case code == "50300":
		return "50300 学生认证服务暂不可用", false
	default:
		if len(msg) > 120 {
			msg = msg[:120]
		}
		return fmt.Sprintf("http=%d code=%s %s", st, code, msg), false
	}
}

func (s *Service) schoolRequestRetry(
	ctx context.Context,
	out io.Writer,
	a Account,
	method, url string,
	body any,
	extra map[string]string,
	gap float64,
) (httpResult, error) {
	const retries = 3
	var last error
	var result httpResult
	for i := 1; i <= retries; i++ {
		result, last = s.request(ctx, method, "", url, body, mpHeaders(a, extra), 30*time.Second)
		if last != nil {
			fmt.Fprintf(out, "  [warn] 网络错误重试 %d/%d: %v\n", i, retries, last)
			if i < retries {
				if !s.Runtime.sleep(ctx, time.Duration(gap*float64(i)*float64(time.Second))) {
					return httpResult{Status: -1}, ctx.Err()
				}
			}
			continue
		}
		if result.Status >= 500 && result.Status < 600 && i < retries {
			last = fmt.Errorf("http %d", result.Status)
			fmt.Fprintf(out, "  [warn] 5xx=%d 重试 %d/%d\n", result.Status, i, retries)
			if !s.Runtime.sleep(ctx, time.Duration(gap*float64(i)*float64(time.Second))) {
				return httpResult{Status: -1}, ctx.Err()
			}
			continue
		}
		return result, nil
	}
	if last != nil {
		return result, last
	}
	return result, nil
}

func schoolOperation(
	st int,
	body map[string]any,
	label string,
) (int, map[string]any, error) {
	if st != 200 || !isOK(body) {
		msg, authFailure := interpretSchoolFailure(st, body)
		return st, body, &schoolHTTPError{Message: label + " " + msg, Auth: authFailure}
	}
	return st, body, nil
}

func (s *Service) fetchSchoolTasks(
	ctx context.Context,
	out io.Writer,
	a Account,
	gap float64,
) ([]map[string]any, bool, error) {
	res, err := s.schoolRequestRetry(ctx, out, a, "GET", s.CloudAgent+"/portal/activity/school/tasks", nil, nil, gap)
	if err != nil {
		return nil, false, err
	}
	if _, _, err := schoolOperation(res.Status, res.Body, "school/tasks"); err != nil {
		return nil, false, err
	}
	data := object(res.Body["data"])
	raw := array(data["tasks"])
	tasks := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if task, ok := item.(map[string]any); ok {
			tasks = append(tasks, task)
		}
	}
	return tasks, truthy(data["in_period"]), nil
}

func (s *Service) fetchSchoolProfile(
	ctx context.Context,
	out io.Writer,
	a Account,
	which string,
	gap float64,
) (map[string]any, error) {
	base := s.CloudAgent + "/portal/activity/teacher"
	if which == "freshman" {
		base = s.CloudAgent + "/portal/activity/freshman"
	}
	res, err := s.schoolRequestRetry(ctx, out, a, "GET", base+"/profile", nil, nil, gap)
	if err != nil {
		return nil, err
	}
	if _, _, err := schoolOperation(res.Status, res.Body, which+"/profile"); err != nil {
		return nil, err
	}
	data := object(res.Body["data"])
	if len(data) == 0 {
		return nil, nil
	}
	return data, nil
}

func (s *Service) postSchoolViewed(
	ctx context.Context,
	out io.Writer,
	a Account,
	code string,
	gap float64,
) (int, map[string]any, error) {
	res, err := s.schoolRequestRetry(ctx, out, a, "POST",
		s.CloudAgent+"/portal/activity/school/tasks/"+code+"/viewed", nil, nil, gap)
	if err != nil {
		return -1, nil, err
	}
	return schoolOperation(res.Status, res.Body, "viewed "+code)
}

func (s *Service) postSchoolShare(
	ctx context.Context,
	out io.Writer,
	a Account,
	gap float64,
) (int, map[string]any, error) {
	res, err := s.schoolRequestRetry(ctx, out, a, "POST",
		s.CloudAgent+"/portal/activity/school/tasks/share-complete",
		map[string]any{"channel": "wechat"}, nil, gap)
	if err != nil {
		return -1, nil, err
	}
	return schoolOperation(res.Status, res.Body, "share-complete")
}

func (s *Service) postSchoolClaim(
	ctx context.Context,
	out io.Writer,
	a Account,
	code string,
	gap float64,
) (int, map[string]any, error) {
	res, err := s.schoolRequestRetry(ctx, out, a, "POST",
		s.CloudAgent+"/portal/activity/school/tasks/"+code+"/claim", nil, nil, gap)
	if err != nil {
		return -1, nil, err
	}
	return schoolOperation(res.Status, res.Body, "claim "+code)
}

func (s *Service) fetchLotteryConfig(
	ctx context.Context,
	out io.Writer,
	a Account,
	gap float64,
) (map[string]any, map[string]any, error) {
	res, err := s.schoolRequestRetry(ctx, out, a, "GET",
		s.CloudAgent+"/portal/activity/school/config", nil, nil, gap)
	if err != nil {
		return nil, nil, err
	}
	if _, _, err := schoolOperation(res.Status, res.Body, "config"); err != nil {
		return nil, nil, err
	}
	data := object(res.Body["data"])
	if data == nil {
		data = map[string]any{}
	}
	return data, object(data["chance"]), nil
}

func (s *Service) postLotteryDraw(
	ctx context.Context,
	out io.Writer,
	a Account,
	drawUUID string,
	gap float64,
) (httpResult, error) {
	return s.schoolRequestRetry(ctx, out, a, "POST",
		s.CloudAgent+"/portal/activity/school/wheel/draw",
		map[string]any{"draw_uuid": drawUUID}, nil, gap)
}

func (s *Service) reportSchoolEvents(
	ctx context.Context,
	out io.Writer,
	a Account,
	events []map[string]any,
	host string,
	extra map[string]string,
	gap float64,
) (int, map[string]any, error) {
	res, err := s.schoolRequestRetry(ctx, out, a, "POST", host+"/v2/report", events, extra, gap)
	if err != nil {
		return -1, nil, err
	}
	return res.Status, res.Body, nil
}

func (s *Service) fetchSchoolExpert(
	ctx context.Context,
	out io.Writer,
	a Account,
	gap float64,
) (string, string, string) {
	body := map[string]any{
		"edition_mode": "all,domestic", "page": 1, "page_size": 20,
		"sort_by": "use_count", "sort_order": "desc",
		"categories": []string{schoolExpertCategory}, "expert_type": "agent",
	}
	res, err := s.schoolRequestRetry(ctx, out, a, "POST",
		s.CloudAgent+"/v2/operation-platform/market/expert/list", body, nil, gap)
	if err == nil && res.Status == 200 && isOK(res.Body) {
		for _, raw := range array(object(res.Body["data"])["experts"]) {
			expert := object(raw)
			id := stringValue(expert["expert_id"])
			if id == "" {
				continue
			}
			display := expert["display_name_zh"]
			if m := object(display); m != nil {
				display = m["zh"]
			}
			profession := expert["profession_zh"]
			if m := object(profession); m != nil {
				profession = m["zh"]
			}
			name := stringValue(display)
			if name == "" {
				name = id
			}
			return id, name, stringValue(profession)
		}
	} else if err != nil {
		fmt.Fprintf(out, "  [warn] 拉取 BackToSchool 专家失败，回落已知专家: %v\n", err)
	} else {
		fmt.Fprintf(out, "  [warn] 拉取 BackToSchool 专家失败，回落已知专家: expert/list code=%s\n",
			legacyValue(codeOf(res.Body)))
	}
	if len(schoolExpertFallback) == 0 {
		panic("no school expert fallback")
	}
	item := schoolExpertFallback[0]
	meta := object(item.Meta)
	return item.ID, stringValue(meta["name"]), stringValue(meta["title"])
}

func (s *Service) makeSchoolReportEvents(
	ctx context.Context,
	out io.Writer,
	a Account,
	kind string,
	gap float64,
) ([]map[string]any, string, map[string]string, error) {
	now := s.Runtime.millis()
	switch kind {
	case "mini_chat":
		return []map[string]any{miniChatEvent(a, now, fmt.Sprintf("wbmp-%d", now), true)},
			s.CloudAgent, nil, nil
	case "expert":
		id, name, _ := s.fetchSchoolExpert(ctx, out, a, gap)
		conv := fmt.Sprintf("wbexp-%d", now)
		return []map[string]any{expertActualUseEvent(a, now, id, name, conv)},
			s.CloudAgent, nil, nil
	case "desktop_seq":
		return s.schoolDesktopReportEvents(a, now), s.Copilot,
			map[string]string{"X-Product": "SaaS", "User-Agent": desktopUA}, nil
	default:
		return nil, "", nil, fmt.Errorf("未知 report_kind: %s", kind)
	}
}
