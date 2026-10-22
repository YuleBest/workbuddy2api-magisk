package tasks

const schoolActivityID = "school_open_day_2026"

const schoolExpertCategory = "16-BackToSchool"

var schoolExpertFallback = []catalogItem{
	{ID: "ex_jB0dyFIQJEWa", Meta: map[string]any{"name": "论小舟", "title": "论文写作导师"}},
	{ID: "ex_lQjkerakvIex", Meta: map[string]any{"name": "英语学习教练", "title": "大学英语学习教练"}},
}

var lotteryPrizeLabels = map[string]struct {
	Label string
	Type  string
}{
	"school_credit_6":        {"6积分", "credit"},
	"school_credit_66":       {"66积分", "credit"},
	"school_voucher_luckin":  {"瑞幸咖啡15元券", "voucher"},
	"school_voucher_kfc_ok":  {"肯德基OK餐券", "voucher"},
	"school_voucher_kfc_ice": {"肯德基冰淇淋券", "voucher"},
	"school_voucher_kugou":   {"酷狗会员月卡券", "voucher"},
}

func miniChatEvent(a Account, nowMillis int64, conversationID string, withActivity bool) map[string]any {
	event := map[string]any{
		"eventCode": "chat_request_send", "timestamp": nowMillis, "reportDelay": 0,
		"source": "mini_program", "ideName": "wx_app_cloud", "ideType": "WorkBuddy_MP",
		"extName": "workbuddy-mp", "extVersion": "SaaS", "mode": "chat",
		"conversationId": conversationID, "requestId": conversationID,
		"inputLength":     12,
		"mentionContexts": []any{}, "mentionContextCount": 0,
		"userId": a.UID,
	}
	if withActivity {
		event["activityId"] = schoolActivityID
	}
	return event
}

func expertActualUseEvent(a Account, nowMillis int64, expertID, expertName, conversationID string) map[string]any {
	return map[string]any{
		"eventCode": "expert_actual_use", "timestamp": nowMillis, "reportDelay": 0,
		"source": "mini_program", "ideName": "wx_app_cloud", "ideType": "WorkBuddy_MP",
		"extName": "workbuddy-mp", "extVersion": "SaaS",
		"machineId": deriveID(a, "machine"), "os": "android", "osVersion": "14",
		"arch": "arm64", "timezone": "Asia/Shanghai",
		"userId": a.UID, "userNickname": a.Nickname,
		"id": expertID, "name": expertID, "expertTitle": expertName,
		"type": "send_message", "characterCount": 12, "expertType": "agent",
		"conversationId": conversationID,
		"activityId":     schoolActivityID,
	}
}

func (s *Service) schoolDesktopReportEvents(a Account, nowMillis int64) []map[string]any {
	conv := "wbdesk-" + stringValue(nowMillis)
	seq := desktopChatSequence(a, nowMillis, conv, conv, conv, "fast-model", "fast-model")
	fp := desktopFingerprint(a, nowMillis)
	out := make([]map[string]any, 0, len(seq))
	for _, event := range seq {
		m := cloneMap(event)
		for key, value := range fp {
			m[key] = value
		}
		m["activityId"] = schoolActivityID
		out = append(out, m)
	}
	return out
}

func lotteryPrizeText(prizeCode string, creditAmount int) string {
	info, ok := lotteryPrizeLabels[prizeCode]
	if !ok {
		return prizeCode
	}
	if info.Type == "credit" {
		return info.Label + "（+" + stringValue(creditAmount) + " Credit）"
	}
	return info.Label
}
