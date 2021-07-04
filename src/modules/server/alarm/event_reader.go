package alarm

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"
	"github.com/didi/nightingale/v4/src/modules/server/config"
	"github.com/didi/nightingale/v4/src/modules/server/redisc"

	"github.com/toolkits/pkg/logger"
)

func ReadHighEvent() {
	queues := config.Config.Monapi.Queue.High
	if len(queues) == 0 {
		return
	}

	for {
		time.Sleep(time.Millisecond * 500)
		events := redisc.PopEvent(50, queues)
		if len(events) == 0 {
			continue
		}
		go processEvents(events, true)
	}
}

func ReadLowEvent() {
	queues := config.Config.Monapi.Queue.Low
	if len(queues) == 0 {
		return
	}

	for {
		time.Sleep(time.Millisecond * 500)
		events := redisc.PopEvent(100, queues)
		if len(events) == 0 {
			continue
		}
		go processEvents(events, false)
	}
}

func processEvents(events []*models.Event, isHigh bool){
	for _, one := range events {
		event, sleep := processEvent(one)
		logger.Debugf("process event: %+v, sleep: %t", event, sleep)

		if sleep {
			time.Sleep(time.Millisecond * 500)
			continue
		}

		consume(event, isHigh)
	}
}

func processEvent(event *models.Event) (*models.Event, bool) {
	stra, has := cache.AlarmStraCache.GetById(event.Sid)
	if !has {
		// 可能策略已经删除了
		logger.Errorf("stra not found, stra id: %d, event: %+v", event.Sid, event)
		return nil, false
	}

	var nodePath string
	var curNodePath string

	node := cache.TreeNodeCache.GetBy(stra.Nid)
	if node == nil {
		logger.Warningf("get node failed, node id: %v, event: %+v, TreeNodeCache no such node", stra.Nid, event)
		return nil, true
	} else {
		nodePath = node.Path
	}

	if stra.Category == 2 {
		curNid, err := strconv.ParseInt(event.CurNid, 10, 64)
		if err != nil {
			logger.Errorf("get cur_node failed, node id: %v, event: %+v, err: %v", stra.Nid, event, err)
			return nil, true
		}

		CurNode := cache.TreeNodeCache.GetBy(curNid)
		if CurNode == nil {
			logger.Errorf("get cur_node by id return nil, node id: %v, event: %+v", stra.Nid, event)
			return nil, false
		}

		curNodePath = CurNode.Path
	} else if stra.Category == 1 {
		// 如果nid和endpoint的对应关系不正确，直接丢弃该event，
		// 用户如果把机器挪节点了，但是judge那边没有及时的同步到，这边再做一次判断

		idents := cache.NodeIdentsMapCache.GetBy(stra.Nid)
		if len(idents) == 0 {
			logger.Errorf("error! not any ident of node id:%d, event: %+v", stra.Nid, event)
			return nil, true
		}

		has := false
		for _, ident := range idents {
			if ident == event.Endpoint {
				has = true
				break
			}
		}

		if !has {
			logger.Errorf("endpoint(%s) not match nid(%v), event: %+v", event.Endpoint, stra.Nid, event)
			return nil, false
		}

		curNodePath = nodePath
	}

	users, err := json.Marshal(stra.NotifyUser)
	if err != nil {
		logger.Errorf("users marshal failed, err: %v, event: %+v", err, event)
		return nil, false
	}

	groups, err := json.Marshal(stra.NotifyGroup)
	if err != nil {
		logger.Errorf("groups marshal failed, err: %v, event: %+v", err, event)
		return nil, false
	}

	if len(stra.WorkGroups) != 0 {
		event.WorkGroups = stra.WorkGroups
	}

	alertUpgrade, err := models.EventAlertUpgradeMarshal(stra.AlertUpgrade)
	if err != nil {
		logger.Errorf("EventAlertUpgradeMarshal failed, err: %v, event: %+v", err, event)
		return nil, false
	}

	// 补齐event中的字段
	event.Sname = stra.Name
	event.Category = stra.Category
	event.Priority = stra.Priority
	event.Nid = stra.Nid
	event.Users = string(users)
	event.Groups = string(groups)
	event.Runbook = stra.Runbook
	event.NodePath = nodePath
	event.CurNodePath = curNodePath
	event.NeedUpgrade = stra.NeedUpgrade
	event.AlertUpgrade = alertUpgrade
	err = models.SaveEvent(event)
	if err != nil {
		return event, true
	}

	if event.EventType == models.ALERT {
		eventCur := new(models.EventCur)
		eventCur.Sid = event.Sid
		eventCur.Sname = stra.Name
		eventCur.Endpoint = event.Endpoint
		eventCur.Category = stra.Category
		eventCur.Priority = stra.Priority
		eventCur.EventType = event.EventType
		eventCur.Nid = stra.Nid
		eventCur.CurNid = event.CurNid
		eventCur.Users = string(users)
		eventCur.Groups = string(groups)
		eventCur.WorkGroups = event.WorkGroups
		eventCur.Runbook = stra.Runbook
		eventCur.NodePath = nodePath
		eventCur.CurNodePath = curNodePath
		eventCur.NeedUpgrade = stra.NeedUpgrade
		eventCur.AlertUpgrade = alertUpgrade
		eventCur.Status = 0
		eventCur.HashId = event.HashId
		eventCur.Etime = event.Etime
		eventCur.Value = event.Value
		eventCur.Info = event.Info
		eventCur.Detail = event.Detail
		eventCur.Claimants = "[]"
		err = models.SaveEventCur(eventCur)
		if err != nil {
			logger.Errorf("save event cur failed, err: %v, event: %+v", err, event)
			return event, true
		}
	} else {
		err = models.EventCurDel(event.HashId)
		if err != nil {
			logger.Errorf("del event cur failed, err: %v, event: %v", err, event)
			return event, true
		}
	}

	return event, false
}
