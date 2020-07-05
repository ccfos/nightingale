package cron

import (
	"encoding/json"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/modules/monapi/config"
	"github.com/didi/nightingale/src/modules/monapi/mcache"
	"github.com/didi/nightingale/src/modules/monapi/redisc"
	"github.com/didi/nightingale/src/toolkits/stats"
)

func EventConsumer() {
	queues := config.Get().Queue.EventQueues
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
		consume(event)
	}
}

// 什么情况需要让上层for循环sleep呢？
// 1. 读取redis i/o超时，表示redis有问题，或者此时queue中压根就没有event
// 2. 访问数据库报错，此时继续玩命搞也没啥意义，sleep一下等数据库恢复
func popEvent(queues []interface{}) (*model.Event, bool) {
	// 1 是BRPOP的超时时间，1秒超时，理论上可以设置为0，但是每个redis连接
	// 有个read timeout在创建redis连接池的时候统一指定，所以，如果这里
	// 设置为0，并且queue里迟迟没有数据，因为read timeout的缘故，必然每次
	// 都会报出read timeout的超时，看着挺烦的，最佳实践这里设置为1s，read
	// timeout设置为3s
	queues = append(queues, 1)

	rc := redisc.RedisConnPool.Get()
	defer rc.Close()

	reply, err := redis.Strings(rc.Do("BRPOP", queues...))
	if err != nil {
		if err != redis.ErrNil {
			stats.Counter.Set("redis.pop.err", 1)
			logger.Warningf("get alarm event from redis failed, queues: %v, err: %v", queues, err)
		}
		return nil, true
	}
	stats.Counter.Set("event.pop", 1)

	if reply == nil {
		logger.Errorf("get alarm event from redis timeout")
		stats.Counter.Set("redis.pop.err", 1)
		return nil, true
	}

	event := new(model.Event)
	if err = json.Unmarshal([]byte(reply[1]), event); err != nil {
		logger.Errorf("unmarshal redis reply failed, err: %v", err)
		return nil, false
	}

	stra, has := mcache.StraCache.GetById(event.Sid)
	if !has {
		// 可能策略已经删了
		logger.Errorf("stra not found, stra id: %d, event: %+v", event.Sid, event)
		return nil, false
	}

	// 如果nid和endpoint的对应关系不正确，直接丢弃该event
	// 可能endpoint挪了节点
	endpoint, err := model.EndpointGet("ident", event.Endpoint)
	if err != nil {
		logger.Errorf("model.EndpointGet fail, event: %+v, err: %v", event, err)
		return nil, true
	}

	if endpoint == nil {
		logger.Errorf("endpoint[%s] not found, event: %+v", event.Endpoint, event)
		return nil, false
	}

	nodePath := ""

	node, err := model.NodeGet("id", stra.Nid)
	if err != nil {
		logger.Errorf("get node failed, node id: %v, event: %+v, err: %v", stra.Nid, event, err)
		return nil, true
	}

	if node == nil {
		logger.Errorf("get node by id return nil, node id: %v, event: %+v", stra.Nid, event)
		return nil, false
	}

	nodePath = node.Path

	leafIds, err := node.LeafIds()
	if err != nil {
		logger.Errorf("get node leaf ids failed, node id: %v, event: %+v, err: %v", stra.Nid, event, err)
		return nil, true
	}

	nodeIds, err := model.NodeIdsGetByEndpointId(endpoint.Id)
	if err != nil {
		logger.Errorf("get node_endpoint by endpoint_id fail: %v, event: %+v", err, event)
		return nil, true
	}

	if nodeIds == nil || len(nodeIds) == 0 {
		logger.Errorf("endpoint[%s] not bind any node, event: %+v", event.Endpoint, event)
		return nil, false
	}

	has = false
	for i := 0; i < len(nodeIds); i++ {
		for j := 0; j < len(leafIds); j++ {
			if nodeIds[i] == leafIds[j] {
				has = true
				break
			}
		}
	}

	if !has {
		logger.Errorf("endpoint(%s) not match nid(%v), event: %+v", event.Endpoint, stra.Nid, event)
		return nil, false
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

	alertUpgrade, err := model.EventAlertUpgradeMarshal(stra.AlertUpgrade)
	if err != nil {
		logger.Errorf("EventAlertUpgradeMarshal failed, err: %v, event: %+v", err, event)
		return nil, false
	}

	// 补齐event中的字段
	event.Sname = stra.Name
	event.EndpointAlias = endpoint.Alias
	event.Category = stra.Category
	event.Priority = stra.Priority
	event.Nid = stra.Nid
	event.Users = string(users)
	event.Groups = string(groups)
	event.NodePath = nodePath
	event.NeedUpgrade = stra.NeedUpgrade
	event.AlertUpgrade = alertUpgrade
	err = model.SaveEvent(event)
	if err != nil {
		stats.Counter.Set("event.save.err", 1)
		logger.Errorf("save event fail: %v, event: %+v", err, event)
		return event, true
	}

	if event.EventType == config.ALERT {
		eventCur := new(model.EventCur)
		if err = json.Unmarshal([]byte(reply[1]), eventCur); err != nil {
			logger.Errorf("unmarshal redis reply failed, err: %v, event: %+v", err, event)
		}

		eventCur.Sname = stra.Name
		eventCur.Category = stra.Category
		eventCur.Priority = stra.Priority
		eventCur.Nid = stra.Nid
		eventCur.Users = string(users)
		eventCur.Groups = string(groups)
		eventCur.NodePath = nodePath
		eventCur.NeedUpgrade = stra.NeedUpgrade
		eventCur.AlertUpgrade = alertUpgrade
		eventCur.EndpointAlias = endpoint.Alias
		eventCur.Status = 0
		eventCur.Claimants = "[]"
		err = model.SaveEventCur(eventCur)
		if err != nil {
			stats.Counter.Set("event.cur.save.err", 1)
			logger.Errorf("save event cur failed, err: %v, event: %+v", err, event)
			return event, true
		}
	} else {
		err = model.EventCurDel(event.HashId)
		if err != nil {
			stats.Counter.Set("event.cur.del.err", 1)
			logger.Errorf("del event cur failed, err: %v, event: %v", err, event)
			return event, true
		}
	}

	return event, false
}
