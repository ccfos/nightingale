package aconf

import (
	"path"
)

type Alert struct {
	Disable     bool
	EngineDelay int64
	Heartbeat   HeartbeatConfig
	Alerting    Alerting
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

type Ibex struct {
	Address       string
	BasicAuthUser string
	BasicAuthPass string
	Timeout       int64
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

	if a.Heartbeat.EngineName == "" {
		a.Heartbeat.EngineName = "default"
	}

	if a.EngineDelay == 0 {
		a.EngineDelay = 30
	}
}
