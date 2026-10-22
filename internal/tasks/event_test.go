package tasks

import "testing"

func TestGrowthEventBuilders(t *testing.T) {
	account := testAccount()
	now := int64(1234567890)
	tests := []struct {
		name   string
		kind   string
		id     string
		meta   any
		assert func(*testing.T, map[string]any)
	}{
		{
			name: "canvas", kind: "canvas", id: "wbx-canvas-1",
			assert: func(t *testing.T, event map[string]any) {
				if event["eventCode"] != "wbx_design_canvas_task_create" ||
					event["id"] != "wbx-canvas-1" || event["userId"] != account.UID {
					t.Errorf("canvas event=%v", event)
				}
			},
		},
		{
			name: "template", kind: "template", id: "2", meta: "视频生成",
			assert: func(t *testing.T, event map[string]any) {
				name, ok := event["name"].(map[string]any)
				if event["eventCode"] != "agent_task_created_with_template" ||
					!ok || name["name"] != "视频生成" {
					t.Errorf("template event=%v", event)
				}
			},
		},
		{
			name: "expert", kind: "expert", id: "ex_1",
			meta: map[string]any{"name": "专家", "category": "cat", "expertType": "agent", "version": "1"},
			assert: func(t *testing.T, event map[string]any) {
				if event["eventCode"] != "expert_actual_use" || event["name"] != "专家" ||
					event["type"] != "cat" || event["expertType"] != "agent" {
					t.Errorf("expert event=%v", event)
				}
			},
		},
		{
			name: "team", kind: "team", id: "team_1", meta: map[string]any{"name": "团队"},
			assert: func(t *testing.T, event map[string]any) {
				if event["expertType"] != "team" {
					t.Errorf("team event=%v", event)
				}
			},
		},
		{
			name: "skill", kind: "skill", id: "skill_1", meta: "ppt",
			assert: func(t *testing.T, event map[string]any) {
				if event["eventCode"] != "skill_info" || event["skillName"] != "ppt" {
					t.Errorf("skill event=%v", event)
				}
			},
		},
		{
			name: "automation", kind: "automation",
			assert: func(t *testing.T, event map[string]any) {
				schedule := object(event["schedule"])
				if event["eventCode"] != "automated_task_create_suc" ||
					schedule["rrule"] != "FREQ=WEEKLY;BYDAY=FR;BYHOUR=9;BYMINUTE=0" {
					t.Errorf("automation event=%v", event)
				}
			},
		},
		{
			name: "playbook", kind: "playbook", id: "case",
			meta: map[string]any{
				"title": "案例", "artifact_type": "other", "categories": []any{"cat"},
				"skills":  []any{map[string]any{"skill_id": "s1", "name": "PPT"}},
				"experts": []any{map[string]any{"id": "e1", "name": "专家"}},
				"prompt":  "你好",
			},
			assert: func(t *testing.T, event map[string]any) {
				if event["eventCode"] != "playbook_prompt_send" ||
					event["promptLength"] != 2 || event["skills"] != "s1" ||
					event["expertId"] != "e1" {
					t.Errorf("playbook event=%v", event)
				}
			},
		},
		{
			name: "skin", kind: "skin", id: "theme",
			meta: map[string]any{"name": "主题", "vipLevel": "free", "series": "craft"},
			assert: func(t *testing.T, event map[string]any) {
				if event["eventCode"] != "appearance_skin_apply" || event["name"] != "主题" {
					t.Errorf("skin event=%v", event)
				}
			},
		},
		{
			name: "glm", kind: "glmchat",
			assert: func(t *testing.T, event map[string]any) {
				if event["eventCode"] != "chat_request_send" ||
					event["requestModelId"] != "glm-5.2" || event["mode"] != "craft" {
					t.Errorf("glm event=%v", event)
				}
			},
		},
		{
			name: "cat", kind: "cat",
			assert: func(t *testing.T, event map[string]any) {
				if event["requestModelId"] != "glm-5.2" || event["mode"] != "night" {
					t.Errorf("cat event=%v", event)
				}
			},
		},
		{
			name: "minichat", kind: "minichat",
			assert: func(t *testing.T, event map[string]any) {
				if event["source"] != "mini_program" || event["extVersion"] != "2.4.0" {
					t.Errorf("minichat event=%v", event)
				}
				if _, ok := event["activityId"]; ok {
					t.Errorf("minichat must not include activityId: %v", event)
				}
			},
		},
		{
			name: "schoolseason", kind: "schoolseason",
			assert: func(t *testing.T, event map[string]any) {
				if event["activityId"] != schoolActivityID || event["extVersion"] != "SaaS" {
					t.Errorf("schoolseason event=%v", event)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			event := buildGrowthEvent(account, now, tc.kind, tc.id, tc.meta, 1)
			tc.assert(t, event)
		})
	}
}
