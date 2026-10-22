package tasks

const (
	defaultChatBase    = "https://copilot.tencent.com"
	defaultBillingBase = "https://www.codebuddy.cn"
	defaultCloudAgent  = "https://www.codebuddy.cn"
	defaultCopilot     = "https://copilot.tencent.com"
	defaultWebBase     = "https://www.workbuddy.cn"
	defaultCOSExpert   = "https://acc-1258344699.cos.accelerate.myqcloud.com/workbuddy/expert-marketplace/expert_center.json"
	defaultPlaybook    = "https://static.workbuddy.cn/workbuddy/playbook"
)

type Service struct {
	Runtime Runtime

	ChatBase    string
	BillingBase string
	CloudAgent  string
	Copilot     string
	WebBase     string
	COSExpert   string
	Playbook    string
	AuthsDir    string
}

func NewService() *Service {
	return &Service{
		Runtime:     DefaultRuntime(),
		ChatBase:    defaultChatBase,
		BillingBase: defaultBillingBase,
		CloudAgent:  defaultCloudAgent,
		Copilot:     defaultCopilot,
		WebBase:     defaultWebBase,
		COSExpert:   defaultCOSExpert,
		Playbook:    defaultPlaybook,
		AuthsDir:    resolveAuthsDir(),
	}
}

func (s *Service) normalize() {
	d := NewService()
	if s.Runtime.HTTP == nil {
		s.Runtime.HTTP = d.Runtime.HTTP
	}
	if s.Runtime.Now == nil {
		s.Runtime.Now = d.Runtime.Now
	}
	if s.Runtime.Sleep == nil {
		s.Runtime.Sleep = d.Runtime.Sleep
	}
	if s.Runtime.UUID == nil {
		s.Runtime.UUID = d.Runtime.UUID
	}
	if s.ChatBase == "" {
		s.ChatBase = defaultChatBase
	}
	if s.BillingBase == "" {
		s.BillingBase = defaultBillingBase
	}
	if s.CloudAgent == "" {
		s.CloudAgent = defaultCloudAgent
	}
	if s.Copilot == "" {
		s.Copilot = defaultCopilot
	}
	if s.WebBase == "" {
		s.WebBase = defaultWebBase
	}
	if s.COSExpert == "" {
		s.COSExpert = defaultCOSExpert
	}
	if s.Playbook == "" {
		s.Playbook = defaultPlaybook
	}
	if s.AuthsDir == "" {
		s.AuthsDir = resolveAuthsDir()
	}
}
