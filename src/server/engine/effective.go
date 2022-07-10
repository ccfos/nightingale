package engine

import (
	"strconv"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/src/models"
)

func isNoneffective(timestamp int64, alertRule *models.AlertRule) bool {
	if alertRule.Disabled == 1 {
		return true
	}

	tm := time.Unix(timestamp, 0)
	triggerTime := tm.Format("15:04")
	triggerWeek := strconv.Itoa(int(tm.Weekday()))

	if alertRule.EnableStime <= alertRule.EnableEtime {
		if triggerTime < alertRule.EnableStime || triggerTime > alertRule.EnableEtime {
			return true
		}
	} else {
		if triggerTime < alertRule.EnableStime && triggerTime > alertRule.EnableEtime {
			return true
		}
	}

	alertRule.EnableDaysOfWeek = strings.Replace(alertRule.EnableDaysOfWeek, "7", "0", 1)

	return !strings.Contains(alertRule.EnableDaysOfWeek, triggerWeek)
}
