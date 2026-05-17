package aconf

import (
	"path"
)

type Alert struct {
	Disable     bool
	EngineDelay int64
	Heartbeat   HeartbeatConfig
	Alerting    Alerting
	AIAgent     AIAgent
}

// AIAgent 配置在 alert/edge 进程下执行 ai_runner 事件处理器所需的运行期参数。
// Enable=false 时不向 airunner 注入运行期，告警链路上配置了 ai_runner 节点
// 会直接报错（设计上：边缘节点默认不承担 AI 调用，需要运维显式开启）。
type AIAgent struct {
	Enable     bool
	SkillsPath string
}

type SMTPConfig struct {
	Host               string
	Port               int
	User               string
	Pass               string
	From               string
	InsecureSkipVerify bool
	Batch              int
}

type HeartbeatConfig struct {
	IP         string
	Interval   int64
	Endpoint   string
	EngineName string
}

type Alerting struct {
	Timeout           int64
	TemplatesDir      string
	NotifyConcurrency int
	WebhookBatchSend  bool
	GlobalWebhook     GlobalWebhook
}

type GlobalWebhook struct {
	Enable        bool
	Url           string
	BasicAuthUser string
	BasicAuthPass string
	Timeout       int
	Headers       []string
	SkipVerify    bool
}

type CallPlugin struct {
	Enable     bool
	PluginPath string
	Caller     string
}

type RedisPub struct {
	Enable        bool
	ChannelPrefix string
	ChannelKey    string
}

func (a *Alert) PreCheck(configDir string) {
	if a.Alerting.TemplatesDir == "" {
		a.Alerting.TemplatesDir = path.Join(configDir, "template")
	}

	if a.Alerting.NotifyConcurrency == 0 {
		a.Alerting.NotifyConcurrency = 10
	}

	if a.Heartbeat.Interval == 0 {
		a.Heartbeat.Interval = 1000
	}

	if a.EngineDelay == 0 {
		a.EngineDelay = 30
	}

	if a.AIAgent.Enable && a.AIAgent.SkillsPath == "" {
		a.AIAgent.SkillsPath = path.Join(configDir, "skill")
	}
}
