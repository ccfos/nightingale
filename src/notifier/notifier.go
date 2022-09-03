package notifier

type Notifier interface {
	Descript() string
	Notify([]byte)
	NotifyMaintainer([]byte)
}

var Instance Notifier
