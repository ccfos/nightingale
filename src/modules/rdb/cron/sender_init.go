package cron

import "github.com/didi/nightingale/src/modules/rdb/config"

var (
	SmsWorkerChan   chan int
	MailWorkerChan  chan int
	VoiceWorkerChan chan int
	ImWorkerChan    chan int
)

func InitWorker() {
	if !config.Config.Redis.Enable {
		return
	}

	SmsWorkerChan = make(chan int, config.Config.Sender["sms"].Worker)
	MailWorkerChan = make(chan int, config.Config.Sender["mail"].Worker)
	VoiceWorkerChan = make(chan int, config.Config.Sender["voice"].Worker)
	ImWorkerChan = make(chan int, config.Config.Sender["im"].Worker)
}
