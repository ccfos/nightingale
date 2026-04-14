package dispatch

import (
	"github.com/ccfos/nightingale/v6/models"

	"github.com/toolkits/pkg/logger"
)

func LogEvent(event *models.AlertCurEvent, location string, err ...error) {
	status := "triggered"
	if event.IsRecovered {
		status = "recovered"
	}

	message := ""
	if len(err) > 0 && err[0] != nil {
		message = "error_message: " + err[0].Error()
	}

	logger.Infof(
		"alert_eval_%d event(%s %s) %s: sub_id:%d notify_rule_ids:%v cluster:%s %v%s@%d last_eval_time:%d %s",
		event.RuleId,
		event.Hash,
		status,
		location,
		event.SubRuleId,
		event.NotifyRuleIds,
		event.Cluster,
		event.TagsJSON,
		event.TriggerValue,
		event.TriggerTime,
		event.LastEvalTime,
		message,
	)
}
