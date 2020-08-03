package scache

import (
	"fmt"
	"strconv"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/model"
)

var JudgeHashRing *ConsistentHashRing
var JudgeActiveNode = NewNodeMap()

func Init() {
	// 初始化默认参数
	StraCache = NewStraCache()
	CollectCache = NewCollectCache()
	JudgeHashRing = NewConsistentHashRing(500, []string{})

	go SyncStras()
	go SyncCollects()
}

func SyncStras() {
	t1 := time.NewTicker(time.Duration(10) * time.Second)

	syncStras()
	logger.Info("[cron] sync stras start...")
	for {
		<-t1.C
		syncStras()
	}
}

func syncStras() {
	stras, err := model.EffectiveStrasList()
	if err != nil {
		logger.Error("sync stras err:", err)
		return
	}
	strasMap := make(map[string][]*model.Stra)
	for _, stra := range stras {
		//增加叶子节点nid
		stra.LeafNids, err = GetLeafNids(stra.Nid, stra.ExclNid)
		if err != nil {
			logger.Warningf("get LeafNids err:%v %v", err, stra)
			continue
		}

		endpoints, err := model.EndpointUnderLeafs(stra.LeafNids)
		if err != nil {
			logger.Warningf("get endpoints err:%v %v", err, stra)
			continue
		}

		for _, e := range endpoints {
			stra.Endpoints = append(stra.Endpoints, e.Ident)
		}

		node, err := JudgeHashRing.GetNode(strconv.FormatInt(stra.Id, 10))
		if err != nil {
			logger.Warningf("get node err:%v %v", err, stra)
		}

		if _, exists := strasMap[node]; exists {
			strasMap[node] = append(strasMap[node], stra)
		} else {
			strasMap[node] = []*model.Stra{stra}
		}
	}

	StraCache.SetAll(strasMap)
}

func SyncCollects() {
	t1 := time.NewTicker(time.Duration(10) * time.Second)

	syncCollects()
	logger.Info("[cron] sync collects start...")
	for {
		<-t1.C
		syncCollects()
	}
}

func syncCollects() {
	collectMap := make(map[string]*model.Collect)

	ports, err := model.GetPortCollects()
	if err != nil {
		logger.Warningf("get port collects err:%v", err)
	}

	for _, p := range ports {
		leafNids, err := GetLeafNids(p.Nid, []int64{})
		if err != nil {
			logger.Warningf("get LeafNids err:%v %v", err, p)
			continue
		}

		endpoints, err := model.EndpointUnderLeafs(leafNids)
		if err != nil {
			logger.Warningf("get endpoints err:%v %v", err, p)
			continue
		}

		for _, endpoint := range endpoints {
			name := endpoint.Ident
			c, exists := collectMap[name]
			if !exists {
				c = model.NewCollect()
			}
			c.Ports[p.Port] = p

			collectMap[name] = c
		}
	}

	procs, err := model.GetProcCollects()
	if err != nil {
		logger.Warningf("get port collects err:%v", err)
	}

	for _, p := range procs {
		leafNids, err := GetLeafNids(p.Nid, []int64{})
		if err != nil {
			logger.Warningf("get LeafNids err:%v %v", err, p)
			continue
		}

		endpoints, err := model.EndpointUnderLeafs(leafNids)
		if err != nil {
			logger.Warningf("get endpoints err:%v %v", err, p)
			continue
		}

		for _, endpoint := range endpoints {
			name := endpoint.Ident
			c, exists := collectMap[name]
			if !exists {
				c = model.NewCollect()
			}
			c.Procs[p.Target] = p
			collectMap[name] = c
		}
	}

	logConfigs, err := model.GetLogCollects()
	if err != nil {
		logger.Warningf("get log collects err:%v", err)
	}

	for _, l := range logConfigs {
		l.Decode()
		leafNids, err := GetLeafNids(l.Nid, []int64{})
		if err != nil {
			logger.Warningf("get LeafNids err:%v %v", err, l)
			continue
		}

		Endpoints, err := model.EndpointUnderLeafs(leafNids)
		if err != nil {
			logger.Warningf("get endpoints err:%v %v", err, l)
			continue
		}

		for _, endpoint := range Endpoints {
			name := endpoint.Ident
			c, exists := collectMap[name]
			if !exists {
				c = model.NewCollect()
			}
			c.Logs[l.Name] = l
			collectMap[name] = c
		}
	}

	pluginConfigs, err := model.GetPluginCollects()
	if err != nil {
		logger.Warningf("get log collects err:%v", err)
	}

	for _, p := range pluginConfigs {
		leafNids, err := GetLeafNids(p.Nid, []int64{})
		if err != nil {
			logger.Warningf("get LeafNids err:%v %v", err, p)
			continue
		}

		Endpoints, err := model.EndpointUnderLeafs(leafNids)
		if err != nil {
			logger.Warningf("get endpoints err:%v %v", err, p)
			continue
		}

		for _, endpoint := range Endpoints {
			name := endpoint.Ident
			c, exists := collectMap[name]
			if !exists {
				c = model.NewCollect()
			}

			key := fmt.Sprintf("%s-%d", p.Name, p.Nid)
			c.Plugins[key] = p
			collectMap[name] = c
		}
	}

	CollectCache.SetAll(collectMap)
}

func GetLeafNids(nid int64, exclNid []int64) ([]int64, error) {
	leafIds := []int64{}
	idsMap := make(map[int64]bool)
	node, err := model.NodeGet("id", nid)
	if err != nil {
		return leafIds, err
	}

	if node == nil {
		return nil, fmt.Errorf("no such node[%d]", nid)
	}

	ids, err := node.LeafIds()
	if err != nil {
		return leafIds, err
	}
	//排除节点为空，直接将所有叶子节点返回
	if len(exclNid) == 0 {
		return ids, nil
	}

	exclLeafIds, err := GetExclLeafIds(exclNid)
	if err != nil {
		return leafIds, err
	}

	for _, id := range ids {
		idsMap[id] = true
	}
	for _, id := range exclLeafIds {
		delete(idsMap, id)
	}

	for id := range idsMap {
		leafIds = append(leafIds, id)
	}
	return leafIds, err
}

func removeDuplicateElement(addrs []string) []string {
	result := make([]string, 0, len(addrs))
	temp := map[string]struct{}{}
	for _, item := range addrs {
		if _, ok := temp[item]; !ok {
			temp[item] = struct{}{}
			result = append(result, item)
		}
	}
	return result
}

// GetExclLeafIds 获取排除节点下的叶子节点
func GetExclLeafIds(exclNid []int64) (leafIds []int64, err error) {
	for _, nid := range exclNid {
		node, err := model.NodeGet("id", nid)
		if err != nil {
			return leafIds, err
		}

		if node == nil {
			logger.Warningf("no such node[%d]", nid)
			continue
		}

		ids, err := node.LeafIds()
		if err != nil {
			return leafIds, err
		}
		leafIds = append(leafIds, ids...)
	}
	return leafIds, nil
}
