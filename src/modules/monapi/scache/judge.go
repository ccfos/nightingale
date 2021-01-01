package scache

import (
	"fmt"
	"strconv"
	"time"

	"github.com/didi/nightingale/src/common/report"
	"github.com/didi/nightingale/src/modules/monapi/config"

	"github.com/toolkits/pkg/consistent"
	"github.com/toolkits/pkg/logger"
)

func CheckJudgeNodes() {
	t1 := time.NewTicker(time.Duration(3 * time.Second))
	for {
		<-t1.C
		CheckJudge()
	}
}

func CheckJudge() error {
	judges, err := report.GetAlive("judge", "rdb")
	if err != nil {
		logger.Warning("get judge err:", err)
		return fmt.Errorf("report.GetAlive judge fail: %v", err)
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
	if ActiveJudgeNode.Len() != len(judgeNode) { //scache.ActiveJudgeNode中的node数量和新获取的不同，重新rehash
		rehash = true
	} else {
		for node, instance := range judgeNode {
			v, exists := ActiveJudgeNode.GetInstanceBy(node)
			if !exists || (exists && instance != v) {
				rehash = true
				break
			}
		}
	}
	if rehash {
		ActiveJudgeNode.Set(judgeNode)

		//重建judge hash环
		r := consistent.New()
		r.NumberOfReplicas = config.JudgesReplicas
		nodes := ActiveJudgeNode.GetNodes()
		for _, node := range nodes {
			r.Add(node)
		}
		logger.Warning("judge hash ring rebuild ", r.Members())
		JudgeHashRing.Set(r)
	}

	return nil
}
