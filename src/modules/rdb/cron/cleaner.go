package cron

import (
	"time"

	"github.com/didi/nightingale/src/models"
)

const cleanerInterval = 3600 * time.Second

func CleanerLoop() {
	tc := time.Tick(cleanerInterval)

	for {
		models.AuthState{}.CleanUp()
		models.Captcha{}.CleanUp()
		<-tc
	}
}
