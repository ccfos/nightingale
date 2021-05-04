package cron

import (
	"fmt"
	"strconv"
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/toolkits/pkg/consistent"
	"github.com/toolkits/pkg/logger"
)

func CheckJudgeNodes() {
	if err := CheckJudge(); err != nil {
		logger.Errorf("check judge fail: %v", err)
	}

	t1 := time.NewTicker(time.Duration(3 * time.Second))
	for {
		<-t1.C
		CheckJudge()
	}
}

func CheckJudge() error {
	judges, err := models.GetAllInstances("server", 1)
	if err != nil {
		logger.Warning("get judge err:", err)
		return fmt.Errorf("GetAlive judge fail: %v", err)
	}

	if len(judges) < 1 {
		logger.Warningf("judges count is zero")
		return nil
	}

	judgeNode := make(map[string]string, 0)
	for _, j := range judges {
		if j.Active {
			judgeNode[strconv.FormatInt(j.Id, 10)] = j.Identity + ":" + j.RPCPort
		}
	}

	rehash := false
	if cache.ActiveJudgeNode.Len() != len(judgeNode) { //scache.cache.ActiveJudgeNode中的node数量和新获取的不同，重新rehash
		rehash = true
	} else {
		for node, instance := range judgeNode {
			v, exists := cache.ActiveJudgeNode.GetInstanceBy(node)
			if !exists || (exists && instance != v) {
				rehash = true
				break
			}
		}
	}
	if rehash {
		cache.ActiveJudgeNode.Set(judgeNode)

		//重建judge hash环
		r := consistent.New()
		r.NumberOfReplicas = cache.JudgesReplicas
		nodes := cache.ActiveJudgeNode.GetNodes()
		for _, node := range nodes {
			r.Add(node)
		}
		logger.Warning("judge hash ring rebuild ", r.Members())
		cache.JudgeHashRing.Set(r)
	}

	return nil
}
