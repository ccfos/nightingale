package timer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/models"
)

// CleanExpireMute 清理过期的告警屏蔽
// 1. mute表：如果屏蔽结束时间小于当前时间，说明已经过了屏蔽时间了，这条屏蔽记录就可以被干掉
// 2. resource表：也有个屏蔽结束时间，需要和mute表做相同的判断和清理逻辑
func CleanExpireMute() {
	go loopCleanExpireMute()
}

func loopCleanExpireMute() {
	randtime := rand.Intn(2000)
	fmt.Printf("timer: clean expire mute: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	interval := time.Duration(10) * time.Second

	for {
		time.Sleep(interval)
		cleanExpireMute()
	}
}

func cleanExpireMute() {
	err := models.MuteCleanExpire()
	if err != nil {
		logger.Warningf("MuteCleanExpire fail: %v", err)
	}
}

func CleanExpireResource() {
	go loopCleanExpireResource()
}

func loopCleanExpireResource() {
	randtime := rand.Intn(2000)
	fmt.Printf("timer: clean expire resource: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	interval := time.Duration(10) * time.Second

	for {
		time.Sleep(interval)
		cleanExpireResource()
	}
}

func cleanExpireResource() {
	err := models.ResourceCleanExpire()
	if err != nil {
		logger.Warningf("ResourceCleanExpire fail: %v", err)
	}
}
