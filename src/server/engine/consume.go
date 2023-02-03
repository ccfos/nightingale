package engine

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

func loopConsume(ctx context.Context) {
	sema := semaphore.NewSemaphore(config.C.Alerting.NotifyConcurrency)
	duration := time.Duration(100) * time.Millisecond
	for {
		events := EventQueue.PopBackBy(100)
		if len(events) == 0 {
			time.Sleep(duration)
			continue
		}
		consume(events, sema)
	}
}

func consume(events []interface{}, sema *semaphore.Semaphore) {
	for i := range events {
		if events[i] == nil {
			continue
		}

		event := events[i].(*models.AlertCurEvent)
		sema.Acquire()
		go func(event *models.AlertCurEvent) {
			defer sema.Release()
			consumeOne(event)
		}(event)
	}
}

func consumeOne(event *models.AlertCurEvent) {
	LogEvent(event, "consume")

	if err := event.ParseRule("rule_name"); err != nil {
		event.RuleName = fmt.Sprintf("failed to parse rule name: %v", err)
	}

	if err := event.ParseRule("rule_note"); err != nil {
		event.RuleNote = fmt.Sprintf("failed to parse rule note: %v", err)
	}

	persist(event)

	if event.IsRecovered && event.NotifyRecovered == 0 {
		return
	}

	HandleEventNotify(event, false)
}

func persist(event *models.AlertCurEvent) {
	has, err := models.AlertCurEventExists("hash=?", event.Hash)
	if err != nil {
		logger.Errorf("event_persist_check_exists_fail: %v rule_id=%d hash=%s", err, event.RuleId, event.Hash)
		return
	}

	his := event.ToHis()

	// 不管是告警还是恢复，全量告警里都要记录
	if err := his.Add(); err != nil {
		logger.Errorf(
			"event_persist_his_fail: %v rule_id=%d cluster:%s hash=%s tags=%v timestamp=%d value=%s",
			err,
			event.RuleId,
			event.Cluster,
			event.Hash,
			event.TagsJSON,
			event.TriggerTime,
			event.TriggerValue,
		)
	}

	if has {
		// 活跃告警表中有记录，删之
		err = models.AlertCurEventDelByHash(event.Hash)
		if err != nil {
			logger.Errorf("event_del_cur_fail: %v hash=%s", err, event.Hash)
			return
		}

		if !event.IsRecovered {
			// 恢复事件，从活跃告警列表彻底删掉，告警事件，要重新加进来新的event
			// use his id as cur id
			event.Id = his.Id
			if event.Id > 0 {
				if err := event.Add(); err != nil {
					logger.Errorf(
						"event_persist_cur_fail: %v rule_id=%d cluster:%s hash=%s tags=%v timestamp=%d value=%s",
						err,
						event.RuleId,
						event.Cluster,
						event.Hash,
						event.TagsJSON,
						event.TriggerTime,
						event.TriggerValue,
					)
				}
			}
		}

		return
	}

	if event.IsRecovered {
		// alert_cur_event表里没有数据，表示之前没告警，结果现在报了恢复，神奇....理论上不应该出现的
		return
	}

	// use his id as cur id
	event.Id = his.Id
	if event.Id > 0 {
		if err := event.Add(); err != nil {
			logger.Errorf(
				"event_persist_cur_fail: %v rule_id=%d cluster:%s hash=%s tags=%v timestamp=%d value=%s",
				err,
				event.RuleId,
				event.Cluster,
				event.Hash,
				event.TagsJSON,
				event.TriggerTime,
				event.TriggerValue,
			)
		}
	}
}

// for alerting
func fillUsers(e *models.AlertCurEvent) {
	gids := make([]int64, 0, len(e.NotifyGroupsJSON))
	for i := 0; i < len(e.NotifyGroupsJSON); i++ {
		gid, err := strconv.ParseInt(e.NotifyGroupsJSON[i], 10, 64)
		if err != nil {
			continue
		}
		gids = append(gids, gid)
	}

	e.NotifyGroupsObj = memsto.UserGroupCache.GetByUserGroupIds(gids)

	uids := make(map[int64]struct{})
	for i := 0; i < len(e.NotifyGroupsObj); i++ {
		ug := e.NotifyGroupsObj[i]
		for j := 0; j < len(ug.UserIds); j++ {
			uids[ug.UserIds[j]] = struct{}{}
		}
	}

	e.NotifyUsersObj = memsto.UserCache.GetByUserIds(mapKeys(uids))
}

func mapKeys(m map[int64]struct{}) []int64 {
	lst := make([]int64, 0, len(m))
	for k := range m {
		lst = append(lst, k)
	}
	return lst
}
