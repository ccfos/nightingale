package worker

type WorkerSection struct {
	WorkerNum    int `yaml:"workerNum"`
	QueueSize    int `yaml:"queueSize"`
	PushInterval int `yaml:"pushInterval"`
	WaitPush     int `yaml:"waitPush"`
}

var WorkerConfig WorkerSection

func Init(cfg WorkerSection) {
	WorkerConfig = cfg
}
