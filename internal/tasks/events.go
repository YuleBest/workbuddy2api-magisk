package tasks

import (
	"context"
	"fmt"
	"unicode/utf8"
)

func cloneMap(src map[string]any) map[string]any {
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func desktopFingerprint(a Account, nowMillis int64) map[string]any {
	return map[string]any{
		"timezone":     "Asia/Shanghai",
		"reportDelay":  2000,
		"userId":       a.UID,
		"username":     a.Nickname,
		"userNickname": a.Nickname,
		"product":      "SaaS",
		"releaseDate":  int64(1789036585355),
		"commit":       "5f9692923c93033111c51ad7b003eb80204a9b75",
		"ideName":      "WorkBuddy",
		"ideType":      "WorkBuddy",
		"ideVersion":   "5.5.6",
		"machineId":    deriveID(a, "machine"),
		"sessionId":    deriveID(a, "session"),
		"extName":      "workbuddy-desktop",
		"extVersion":   "5.5.6",
		"os":           "win32",
		"arch":         "x64",
		"osVersion":    "10.0.26220",
		"cpuCores":     20,
		"memorySize":   24,
		"timestamp":    nowMillis,
		"presentAt":    nowMillis,
	}
}

func desktopChatSequence(
	a Account,
	nowMillis int64,
	conversationID, requestID, messageID string,
	modelID, modelName string,
) []map[string]any {
	out := make([]map[string]any, 0, 6)
	add := func(code string, extra map[string]any) {
		e := map[string]any{"eventCode": code}
		for key, value := range extra {
			e[key] = value
		}
		out = append(out, e)
	}
	add("agent_task_created", map[string]any{
		"source": "LOCAL", "name": "working", "task_target": "local", "mode": "craft",
		"requestModelId": modelID, "requestModelName": modelName,
		"has_repo": false, "repo_type": "none", "workspace_type": "empty",
		"has_connector": false, "connector_types": []any{},
		"has_mention": false, "mention_types": []any{},
		"has_template": false, "action": "", "template_name": "",
		"has_expert": false, "expert_id": "", "expert_name": "", "expert_industry_id": "",
		"has_skill": false, "skill_names": []any{},
		"conversationId": conversationID, "messageId": messageID,
		"buddyId": "", "buddyName": "",
	})
	add("chat_message_send", map[string]any{
		"messageId": messageID + "-assistant", "historyCount": 0,
		"isContextTruncated": false, "currentStepCount": 1,
		"traceId": requestID, "rootRequestId": requestID,
		"parentConversationId": conversationID,
		"agentName":            "cli", "agentType": "main",
	})
	add("chat_request_send", map[string]any{
		"inputLength": 24, "isPlan": false, "isAutoExecuteTerminal": false,
		"isAutoModify": false, "codebaseEnable": false, "maxToken": 0,
		"maxSteps": 500, "temperature": 0, "maxRetries": 0,
		"mentionContexts": []any{}, "knowledgeId": []any{}, "knowledgeName": []any{},
		"codebaseId": "", "mentionContextCount": 0, "command": "",
		"recommendId": "", "skillId": "", "skillCount": 0, "totalCount": 0,
		"traceId": requestID, "rootRequestId": requestID,
		"parentConversationId": conversationID,
		"agentName":            "cli", "agentType": "main",
		"codebuddy.session_id":              conversationID,
		"codebuddy.conversation_request_id": requestID,
	})
	add("chat_message_response", map[string]any{
		"messageId": messageID + "-assistant", "responseModelId": modelID,
		"inputToken": 120, "outputToken": 80, "totalToken": 200,
		"cachedTokens": 0, "cachedWriteTokens": 0, "cachedMissTokens": 0,
		"isSuccessful": true, "messageErrorCode": "", "finishReason": "stop",
		"firstTokenAt": nowMillis, "traceId": requestID,
		"conversationId": conversationID,
		"rootRequestId":  requestID, "parentConversationId": conversationID,
		"agentName": "cli", "agentType": "main",
		"codebuddy.session_id":              conversationID,
		"codebuddy.conversation_request_id": requestID,
	})
	add("chat_message_status", map[string]any{
		"messageId": messageID + "-assistant", "messageErrorCode": "0",
		"traceId": requestID, "rootRequestId": requestID,
		"parentConversationId": conversationID,
		"agentName":            "cli", "agentType": "main",
	})
	add("chat_request_response", map[string]any{
		"mode": "craft", "toolCallCount": 0,
		"inputToken": 120, "outputToken": 80, "totalToken": 200,
		"cachedTokens": 0, "cachedWriteTokens": 0, "cachedMissTokens": 0,
		"isSuccessful": true, "messageErrorCode": "", "finishReason": "stop",
		"rootRequestId": requestID, "parentConversationId": conversationID,
	})
	return out
}

func desktopBuddy5Sequence(a Account, buddyID, buddyName string) []map[string]any {
	out := make([]map[string]any, 0, 5)
	add := func(code string, extra map[string]any) {
		e := map[string]any{
			"eventCode": code,
			"mode":      "LOCAL",
			"buddyId":   buddyID,
			"buddyName": buddyName,
		}
		for key, value := range extra {
			e[key] = value
		}
		out = append(out, e)
	}
	add("buddyapp_discover_click", map[string]any{})
	add("buddyapp_show", map[string]any{
		"elementId": buddyID, "elementName": buddyName, "position": 2,
	})
	add("buddyapp_enter_click", map[string]any{
		"elementId": buddyID, "elementName": buddyName, "position": 2, "isFirstPage": "1",
	})
	add("buddyapp_auth_confirm_click", map[string]any{
		"elementId": buddyID, "elementName": buddyName,
	})
	add("buddyapp_bindaccount_skip_click", map[string]any{
		"elementId": buddyID, "elementName": buddyName,
	})
	return out
}

func (s *Service) desktopHeaders(a Account, nowMillis int64) map[string]string {
	return map[string]string{
		"Accept":       "application/json, text/plain, */*",
		"Content-Type": "application/json;charset=UTF-8",
		"User-Agent":   desktopUA,
		"X-Domain":     s.ChatBase,
		"X-Product":    "SaaS",
		"X-Request-ID": deriveID(a, "req") + fmt.Sprint(nowMillis%1000000),
		"X-User-Id":    a.UID,
	}
}

func (s *Service) webEventHeaders(a Account, pageURL string) map[string]string {
	return map[string]string{
		"Accept":            "application/json",
		"Content-Type":      "application/json",
		"x-client-platform": "web",
		"Origin":            s.WebBase,
		"Referer":           pageURL,
		"User-Agent":        webUA,
		"X-User-Id":         a.UID,
		"X-Domain":          s.WebBase,
	}
}

func (s *Service) webClaimHeaders(a Account) map[string]string {
	headers := s.webEventHeaders(a, s.WebBase+"/profile/growth-center")
	headers["Accept"] = "application/json, text/plain, */*"
	return headers
}

func (s *Service) reportDesktopEvents(
	ctx context.Context,
	a Account,
	events []map[string]any,
) (int, any) {
	now := s.Runtime.millis()
	fp := desktopFingerprint(a, now)
	arr := make([]map[string]any, 0, len(events))
	for _, event := range events {
		m := cloneMap(event)
		for key, value := range fp {
			m[key] = value
		}
		arr = append(arr, m)
	}
	st, body := s.doGrowth(ctx, a, "POST", s.ChatBase, "/v2/report", arr, s.desktopHeaders(a, now))
	return st, codeOf(body)
}

func (s *Service) reportWebEvent(
	ctx context.Context,
	a Account,
	eventCode, pageURL, elementID, elementName string,
) (int, any) {
	now := s.Runtime.millis()
	ev := map[string]any{
		"eventCode": eventCode, "timestamp": now, "reportDelay": 0,
		"pageURL": pageURL, "elementId": elementID, "elementName": elementName,
		"os": "Win32", "arch": "", "osVersion": "10.0", "userAgent": webUA,
		"machineId": deriveID(a, "webmachine"), "userId": a.UID,
		"userNickname": a.Nickname,
	}
	st, body := s.doGrowth(ctx, a, "POST", s.WebBase, "/v2/report", []map[string]any{ev},
		s.webEventHeaders(a, pageURL))
	return st, codeOf(body)
}

func chatEvent(a Account, nowMillis int64, conversationID, modelID, modelName, mode string) map[string]any {
	if conversationID == "" {
		conversationID = fmt.Sprintf("task-%d", nowMillis)
	}
	return map[string]any{
		"eventCode": "chat_request_send", "timestamp": nowMillis, "reportDelay": 0,
		"mode": mode, "conversationId": conversationID, "requestId": conversationID,
		"inputLength": 12, "requestModelId": modelID,
		"requestModelName": modelName, "isPlan": false,
		"isAutoExecuteTerminal": false, "isAutoModify": false,
		"codebaseEnable": false, "maxToken": 0, "maxSteps": 0, "temperature": 0,
		"maxRetries": 0, "mentionContexts": []any{}, "knowledgeId": []any{},
		"knowledgeName": []any{}, "codebaseId": "", "mentionContextCount": 0,
		"command": "", "expertId": "", "recommendId": "", "skillId": "",
		"skillCount": 0, "totalCount": 0, "fileUri": "", "presentAt": nowMillis,
		"traceId": "", "rootRequestId": conversationID, "parentConversationId": conversationID,
		"agentName": "default", "agentType": "conversation", "userId": a.UID,
	}
}

func (s *Service) reportActivity(
	ctx context.Context,
	a Account,
	count int,
	gapSeconds float64,
	modelID, modelName, mode string,
) [][2]any {
	out := make([][2]any, 0, count)
	for i := 0; i < count; i++ {
		ev := chatEvent(a, s.Runtime.millis(), "", modelID, modelName, mode)
		st, body := s.doGrowth(ctx, a, "POST", s.BillingBase, "/v2/report",
			[]map[string]any{ev}, nil)
		out = append(out, [2]any{st, codeOf(body)})
		if i < count-1 {
			if !s.Runtime.sleep(ctx, seconds(gapSeconds)) {
				break
			}
		}
	}
	return out
}

type catalogItem struct {
	ID   string
	Meta any
}

func buildGrowthEvent(a Account, nowMillis int64, kind, objID string, meta any, idx int) map[string]any {
	cid := fmt.Sprintf("wb-run-%d-%d", nowMillis, idx)
	rid := fmt.Sprintf("%s-%d", cid, nowMillis)

	var m map[string]any
	switch x := meta.(type) {
	case map[string]any:
		m = x
	case string:
		m = map[string]any{"name": x}
	default:
		m = map[string]any{}
	}
	mget := func(key, fallback string) string {
		if value := stringValue(m[key]); value != "" {
			return value
		}
		return fallback
	}

	switch kind {
	case "canvas":
		id := objID
		if id == "" {
			id = fmt.Sprintf("wbx-canvas-%d", nowMillis)
		}
		return map[string]any{
			"eventCode": "wbx_design_canvas_task_create", "timestamp": nowMillis,
			"reportDelay": 0, "conversationId": cid, "requestId": rid,
			"source": "summon_keyword", "isCustomModel": false, "name": "",
			"inputLength": 12, "id": id, "cost": 0, "isSuccessful": true, "userId": a.UID,
		}
	case "template":
		var name any = ""
		if truthy(m) {
			name = m // Preserve the original script's nested-map name shape.
		}
		return map[string]any{
			"eventCode": "agent_task_created_with_template", "timestamp": nowMillis,
			"reportDelay": 0, "isCustomModel": true, "id": objID,
			"name": name, "requestId": rid, "conversationId": cid, "userId": a.UID,
		}
	case "expert", "team", "lighthouse":
		expertType := mget("expertType", "agent")
		if kind == "team" {
			expertType = "team"
		}
		return map[string]any{
			"eventCode": "expert_actual_use", "timestamp": nowMillis, "reportDelay": 0,
			"mode": "CLOUD", "id": objID, "name": mget("name", objID),
			"expertTitle": mget("name", ""), "type": mget("category", ""),
			"expertType": expertType, "source": "builtin",
			"version": mget("version", ""), "cost": 0, "characterCount": 12,
			"conversationId": cid, "requestId": rid, "messageId": rid,
			"requestModelId": "deepseek-v4-flash", "requestModelName": "DeepSeek V4 Flash",
			"userId": a.UID,
		}
	case "skill":
		name := objID
		switch x := meta.(type) {
		case string:
			if x != "" {
				name = x
			}
		case map[string]any:
			if x["name"] != nil && stringValue(x["name"]) != "" {
				name = stringValue(x["name"])
			}
		}
		return map[string]any{
			"eventCode": "skill_info", "timestamp": nowMillis, "reportDelay": 0,
			"skillId": objID, "skillName": name, "skillVersion": "", "action": "use",
			"conversationId": cid, "requestId": rid, "userId": a.UID,
		}
	case "automation":
		return map[string]any{
			"eventCode": "automated_task_create_suc", "timestamp": nowMillis,
			"reportDelay": 0, "name": "每周五自动生成周报", "source": "manually",
			"modelId": "deepseek-v4-flash", "modelIsThinking": false,
			"expertId": "", "expertMarketplace": "", "connectorIds": "",
			"connectorCount": 0, "skills": "", "skillCount": 0,
			"scheduleType": "recurring", "pushToWeChat": false, "pushToWecomBot": false,
			"conversationId": cid, "requestId": rid,
			"schedule": map[string]any{
				"type":  "recurring",
				"rrule": "FREQ=WEEKLY;BYDAY=FR;BYHOUR=9;BYMINUTE=0",
			},
			"prompt": "每周五自动整理本周工作，生成一份周报。", "userId": a.UID,
		}
	case "playbook":
		skills := array(m["skills"])
		experts := array(m["experts"])
		catID := ""
		if cats := array(m["categories"]); len(cats) > 0 {
			catID = stringValue(cats[0])
		}
		skillIDs, skillNames := []string{}, []string{}
		for _, raw := range skills {
			skill := object(raw)
			if id := stringValue(skill["skill_id"]); id != "" {
				skillIDs = append(skillIDs, id)
			}
			if name := stringValue(skill["name"]); name != "" {
				skillNames = append(skillNames, name)
			}
		}
		expertID, expertName := "", ""
		if len(experts) > 0 {
			expert := object(experts[0])
			expertID = stringValue(expert["expert_id"])
			if expertID == "" {
				expertID = stringValue(expert["id"])
			}
			expertName = stringValue(expert["name"])
		}
		return map[string]any{
			"eventCode": "playbook_prompt_send", "timestamp": nowMillis, "reportDelay": 0,
			"id": objID, "name": mget("title", objID),
			"type":         mget("artifact_type", "other"),
			"promptLength": utf8.RuneCountInString(stringValue(m["prompt"])), "isOfficial": 1,
			"skills": joinStrings(skillIDs), "skillNames": joinStrings(skillNames),
			"expertId": expertID, "expertName": expertName,
			"categoryId": catID, "categoryName": "", "query": "",
			"source": "discover", "conversationId": cid, "requestId": rid,
			"ext1": "discover", "userId": a.UID,
		}
	case "skin":
		return map[string]any{
			"eventCode": "appearance_skin_apply", "timestamp": nowMillis, "reportDelay": 0,
			"action": "apply", "source": "settings_close", "id": objID,
			"vipLevel": mget("vipLevel", "free"), "series": mget("series", "craft"),
			"type": "unknown", "name": mget("name", objID), "userId": a.UID,
		}
	case "chat", "glmchat", "cat":
		modelID, modelName := "deepseek-v4-flash", "DeepSeek V4 Flash"
		if kind == "glmchat" || kind == "cat" {
			modelID, modelName = "glm-5.2", "GLM-5.2"
		}
		mode := "craft"
		if kind == "cat" {
			mode = "night"
		}
		return chatEvent(a, nowMillis, cid, modelID, modelName, mode)
	case "minichat":
		return map[string]any{
			"eventCode": "chat_request_send", "timestamp": nowMillis, "reportDelay": 0,
			"source": "mini_program", "ideName": "wx_app_cloud",
			"ideType": "WorkBuddy_MP", "extName": "workbuddy-mp",
			"extVersion": "2.4.0", "mode": "chat",
			"conversationId": cid, "requestId": cid, "inputLength": 12,
			"mentionContexts": []any{}, "mentionContextCount": 0, "userId": a.UID,
		}
	case "schoolseason":
		return miniChatEvent(a, nowMillis, cid, true)
	}
	panic("unknown growth event kind: " + kind)
}

func joinStrings(values []string) string {
	out := ""
	for i, value := range values {
		if i > 0 {
			out += ","
		}
		out += value
	}
	return out
}
