package engine

import (
	"github.com/didi/nightingale/v5/src/models"
	"github.com/toolkits/pkg/logger"
)

func logEvent(event *models.AlertCurEvent, location string, err ...error) {
	status := "triggered"
	if event.IsRecovered {
		status = "recovered"
	}

	message := ""
	if len(err) > 0 && err[0] != nil {
		message = "error_message: " + err[0].Error()
	}

	logger.Infof(
		"event(%s %s) %s: rule_id=%d %v%s@%d %s",
		event.Hash,
		status,
		location,
		event.RuleId,
		event.TagsJSON,
		event.TriggerValue,
		event.TriggerTime,
		message,
	)
}
