package alarm

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/acache"
	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/modules/monapi/redisc"

	"github.com/garyburd/redigo/redis"
	"github.com/toolkits/pkg/logger"
)

func ReadHighEvent() {
	queues := config.Get().Queue.High
	if len(queues) == 0 {
		return
	}

	duration := time.Duration(400) * time.Millisecond

	for {
		event, sleep := popEvent(queues)
		if sleep {
			time.Sleep(duration)
			continue
		}
		consume(event, true)
	}
}

func ReadLowEvent() {
	queues := config.Get().Queue.Low
	if len(queues) == 0 {
		return
	}

	duration := time.Duration(400) * time.Millisecond

	for {
		event, sleep := popEvent(queues)
		if sleep {
			time.Sleep(duration)
			continue
		}
		consume(event, false)
	}
}

func popEvent(queues []interface{}) (*models.Event, bool) {
	queues = append(queues, 1)

	rc := redisc.RedisConnPool.Get()
	defer rc.Close()

	reply, err := redis.Strings(rc.Do("BRPOP", queues...))
	if err != nil {
		if err != redis.ErrNil {
			logger.Warningf("get alarm event from redis failed, queues: %v, err: %v", queues, err)
		}
		return nil, true
	}

	if reply == nil {
		logger.Errorf("get alarm event from redis timeout")
		return nil, true
	}

	event := new(models.Event)
	if err = json.Unmarshal([]byte(reply[1]), event); err != nil {
		logger.Errorf("unmarshal redis reply failed, err: %v", err)
		return nil, false
	}

	stra, has := acache.StraCache.GetById(event.Sid)
	if !has {
		// 可能策略已经删除了
		logger.Errorf("stra not found, stra id: %d, event: %+v", event.Sid, event)
		return nil, false
	}

	var nodePath string
	var curNodePath string

	node, err := models.NodeGet("id=?", stra.Nid)
	if err != nil {
		logger.Warningf("get node failed, node id: %v, event: %+v, err: %v", stra.Nid, event, err)
	} else {
		nodePath = node.Path
	}

	if stra.Category == 2 {
		curNid, err := strconv.ParseInt(event.CurNid, 10, 64)
		if err != nil {
			logger.Errorf("get cur_node failed, node id: %v, event: %+v, err: %v", stra.Nid, event, err)
			return nil, true
		}

		CurNode, err := models.NodeGet("id=?", curNid)
		if err != nil {
			logger.Errorf("get cur_node failed, node id: %v, event: %+v, err: %v", stra.Nid, event, err)
			return nil, true
		}

		if CurNode == nil {
			logger.Errorf("get cur_node by id return nil, node id: %v, event: %+v", stra.Nid, event)
			return nil, false
		}

		curNodePath = CurNode.Path
	} else if stra.Category == 1 {
		// 如果nid和endpoint的对应关系不正确，直接丢弃该event，
		// 用户如果把机器挪节点了，但是judge那边没有及时的同步到，这边再做一次判断

		nids, err := models.GetLeafNidsForMon(stra.Nid, []int64{})
		if err != nil {
			logger.Errorf("err: %v,event: %+v", err, event)
		}

		rids, err := models.ResIdsGetByNodeIds(nids)
		if err != nil {
			logger.Errorf("err: %v,event: %+v", err, event)
		}

		idents, err := models.ResourceIdentsByIds(rids)
		if err != nil {
			logger.Errorf("err: %v,event: %+v", err, event)
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

	if event.EventType == config.ALERT {
		eventCur := new(models.EventCur)
		if err = json.Unmarshal([]byte(reply[1]), eventCur); err != nil {
			logger.Errorf("unmarshal redis reply failed, err: %v, event: %+v", err, event)
		}

		eventCur.Sname = stra.Name
		eventCur.Category = stra.Category
		eventCur.Priority = stra.Priority
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
