package ports

import (
	"time"

	"github.com/didi/nightingale/src/modules/collector/stra"
)

func Detect() {
	detect()
	go loopDetect()
}

func detect() {
	ps := stra.GetPortCollects()
	DelNoPortCollect(ps)
	AddNewPortCollect(ps)
}

func loopDetect() {
	for {
		time.Sleep(time.Second * 10)
		detect()
	}
}
