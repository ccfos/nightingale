package aconf

import (
	"path"

	"github.com/ccfos/nightingale/v6/pkg/evallog"
)

type Alert struct {
	Disable     bool
	EngineDelay int64
	Heartbeat   HeartbeatConfig
	Alerting    Alerting
	EvalLog     evallog.Config
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

// PreCheck 填充默认值。logDir 是 [Log] Dir 的值，评估执行记录默认落在它下面的
// evallog 子目录里——运维一般已经给日志目录做了磁盘规划，记录跟着走比落到进程
// cwd 更可控；显式配了 [Alert.EvalLog] Dir 则以配置为准。
func (a *Alert) PreCheck(configDir, logDir string) {
	if a.Alerting.TemplatesDir == "" {
		a.Alerting.TemplatesDir = path.Join(configDir, "template")
	}

	if a.EvalLog.Dir == "" && logDir != "" {
		a.EvalLog.Dir = path.Join(logDir, "evallog")
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
}
