package memsto

import (
	"os"

	"github.com/toolkits/pkg/logger"
)

func exit(code int) {
	logger.Close()
	os.Exit(code)
}

func Sync() {
	SyncBusiGroups()
	SyncUsers()
	SyncUserGroups()
	SyncAlertMutes()
	SyncAlertSubscribes()
	SyncAlertRules()
	SyncTargets()
	SyncRecordingRules()
}
