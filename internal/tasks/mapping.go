package tasks

type taskSpec struct {
	Code        string
	Kind        string
	Target      int
	Source      string
	Unforgeable bool
	Reason      string
}

var taskSpecs = []taskSpec{
	{Code: "create_canvas", Kind: "canvas", Target: 1, Source: "自造 wb-<ms>"},
	{Code: "template_5", Kind: "template", Target: 5, Source: "scenes 场景 id"},
	{Code: "expert_5", Kind: "expert", Target: 5, Source: "专家市场 ex_ id"},
	{Code: "Expert_team_use_3", Kind: "team", Target: 3, Source: "团队 id"},
	{Code: "skill_1", Kind: "skill", Target: 1, Source: "skillId"},
	{Code: "automation_1", Kind: "automation", Target: 1, Source: "rrule 虚拟对象"},
	{Code: "playbook_prompt", Kind: "playbook", Target: 1, Source: "playbook 案例 id"},
	{Code: "Expert_lighthouse", Kind: "lighthouse", Target: 1, Source: "轻量云专家 id"},
	{Code: "Buddy_App", Kind: "buddy5", Target: 1, Source: "buddy 应用 id"},
	{Code: "Buddy_App_QQ", Kind: "buddy5", Target: 1, Source: "企鹅教师助手 id"},
	{Code: "Hp_Appearance", Kind: "skin", Target: 1, Source: "主题 resourceKey"},
	{Code: "chat_5", Kind: "chat", Target: 5, Source: "无(独立会话)"},
	{Code: "Model_chat_GLM5.2", Kind: "glmchat", Target: 1, Source: "无(glm-5.2)"},
	{Code: "black_cat", Kind: "cat", Target: 3, Source: "无(glm-5.2 夜猫)"},
	{Code: "first_buddy", Kind: "buddyfirst", Target: 1, Source: "无(report→agreement→first)"},
	{Code: "RichMeow_Chat", Kind: "richmeow", Target: 1, Source: "无(桌面指纹对话链)"},
	{Code: "Library_read", Kind: "library", Target: 1, Source: "无(资料库介绍点击)"},
	{Code: "chat_3_times", Kind: "school", Target: 3, Source: "无(mini_chat×3+activityId)"},
	{Code: "expert_use", Kind: "school", Target: 1, Source: "BackToSchool 专家 id"},
	{Code: "share_invite", Kind: "school", Target: 1, Source: "无(share-complete)"},
	{Code: "desktop_chat_1_time", Kind: "school", Target: 1, Source: "无(桌面 6 连+activityId)"},
	{Code: "Sequential_Tasks_1", Kind: "minichat", Target: 1, Source: "无(mini 对话无activityId)"},
	{Code: "school_season", Kind: "schoolseason", Target: 1, Source: "无(mini chat+activityId)"},
	{Code: "Expert_Philanthropy", Unforgeable: true, Target: 1, Reason: "真实捐款动作(M8)"},
}

var schoolReportKind = map[string]string{
	"chat_3_times":        "mini_chat",
	"expert_use":          "expert",
	"share_invite":        "share",
	"desktop_chat_1_time": "desktop_seq",
}

var schoolCodes = []string{
	"chat_3_times",
	"expert_use",
	"share_invite",
	"desktop_chat_1_time",
}

var minichatCodes = []string{
	"Sequential_Tasks_1",
	"school_season",
}

func specByCode(code string) (taskSpec, bool) {
	for _, spec := range taskSpecs {
		if spec.Code == code {
			return spec, true
		}
	}
	return taskSpec{}, false
}

func isSpecialCode(code string) bool {
	spec, ok := specByCode(code)
	return ok && (spec.Kind == "school" || spec.Kind == "minichat" || spec.Kind == "schoolseason")
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func intersect(values []string, allowed map[string]bool) bool {
	for _, value := range values {
		if allowed[value] {
			return true
		}
	}
	return false
}
