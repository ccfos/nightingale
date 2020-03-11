package plugins

import (
	"time"

	"github.com/didi/nightingale/src/modules/collector/sys"
)

func Detect() {
	detect()
	go loopDetect()
}

func loopDetect() {
	for {
		time.Sleep(time.Second * 10)
		detect()
	}
}

func detect() {
	ps := ListPlugins(sys.Config.Plugin)
	DelNoUsePlugins(ps)
	AddNewPlugins(ps)
}
