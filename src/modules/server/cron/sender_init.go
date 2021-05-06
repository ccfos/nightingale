package cron

var (
	SmsWorkerChan   chan int
	MailWorkerChan  chan int
	VoiceWorkerChan chan int
	ImWorkerChan    chan int
)

const (
	SMS_QUEUE_NAME   = "/queue/rdb/sms"
	MAIL_QUEUE_NAME  = "/queue/rdb/mail"
	VOICE_QUEUE_NAME = "/queue/rdb/voice"
	IM_QUEUE_NAME    = "/queue/rdb/im"
)

type SenderSection struct {
	Way    string `yaml:"way"`
	Worker int    `yaml:"worker"`
	API    string `yaml:"api"`
}

var Sender map[string]SenderSection

func InitWorker(sender map[string]SenderSection) {
	Sender = sender
	SmsWorkerChan = make(chan int, Sender["sms"].Worker)
	MailWorkerChan = make(chan int, Sender["mail"].Worker)
	VoiceWorkerChan = make(chan int, Sender["voice"].Worker)
	ImWorkerChan = make(chan int, Sender["im"].Worker)
}
