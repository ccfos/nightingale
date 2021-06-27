package timer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/v5/models"
)

// 是个兜底扫描器，担心有些resource脱离id为1的preset的classpath
// 如果有发现，就把resource重新bind回来
func BindOrphanRes() {
	go loopBindOrphanRes()
}

func loopBindOrphanRes() {
	randtime := rand.Intn(10000)
	fmt.Printf("timer: bind orphan res: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	interval := time.Duration(10) * time.Second

	for {
		time.Sleep(interval)
		models.BindOrphanToPresetClasspath()
	}
}
