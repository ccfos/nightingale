package cron

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/toolkits/pkg/logger"
)

func SyncCollects() {
	t1 := time.NewTicker(time.Duration(cache.CHECK_INTERVAL) * time.Second)

	syncCollects()
	logger.Info("[cron] sync collects start...")
	for {
		<-t1.C
		syncCollects()
	}
}

func syncCollects() {
	collectMap := make(map[string]*models.Collect)

	ports, err := models.GetPortCollects()
	if err != nil {
		logger.Warningf("get port collects err:%v %v", err)
	}

	for _, p := range ports {
		idents, err := HostUnderNode(p.Nid)

		if err != nil {
			logger.Warningf("get hosts err:%v %v", err, p)
			continue
		}

		for _, ident := range idents {
			var name string
			name = ident

			c, exists := collectMap[name]
			if !exists {
				c = models.NewCollect()
			}
			c.Ports[p.Port] = p

			collectMap[name] = c
		}
	}

	procs, err := models.GetProcCollects()
	if err != nil {
		logger.Warningf("get port collects err:%v %v", err)
	}

	for _, p := range procs {
		idents, err := HostUnderNode(p.Nid)
		if err != nil {
			logger.Warningf("get hosts err:%v %v", err, p)
			continue
		}

		for _, ident := range idents {
			var name string
			name = ident

			c, exists := collectMap[name]
			if !exists {
				c = models.NewCollect()
			}
			c.Procs[p.Target] = p
			collectMap[name] = c
		}
	}

	logConfigs, err := models.GetLogCollects()
	if err != nil {
		logger.Warningf("get log collects err:%v %v", err)
	}

	for _, l := range logConfigs {
		l.Decode()
		idents, err := HostUnderNode(l.Nid)
		if err != nil {
			logger.Warningf("get endpoints err:%v %v", err, l)
			continue
		}

		for _, ident := range idents {
			var name string

			name = ident

			c, exists := collectMap[name]
			if !exists {
				c = models.NewCollect()
			}

			key := fmt.Sprintf("%s-%d", l.Name, l.Nid)
			c.Logs[key] = l
			collectMap[name] = c
		}
	}

	pluginCollects, err := models.GetPluginCollects()
	if err != nil {
		logger.Warningf("get log collects err:%v %v", err)
	}

	for _, p := range pluginCollects {
		idents, err := HostUnderNode(p.Nid)
		if err != nil {
			logger.Warningf("get endpoints err:%v %v", err, p)
			continue
		}

		for _, ident := range idents {
			c, exists := collectMap[ident]
			if !exists {
				c = models.NewCollect()
			}

			key := fmt.Sprintf("%s-%d", p.Name, p.Nid)
			c.Plugins[key] = p
			collectMap[ident] = c
		}
	}

	cache.CollectCache.SetAll(collectMap)

}

func CleanCollectLoop() {
	duration := time.Second * time.Duration(300)
	for {
		time.Sleep(duration)
		cleanCollect()
	}
}

//定期清理没有找到nid的采集策略
func cleanCollect() {
	var list []interface{}
	collects, err := models.GetPortCollects()
	if err != nil {
		logger.Warningf("get collect err: %v", err)
	}
	for _, collect := range collects {
		list = append(list, collect)
	}

	procCollects, err := models.GetProcCollects()
	if err != nil {
		logger.Warningf("get collect err: %v", err)
	}
	for _, collect := range procCollects {
		list = append(list, collect)
	}

	logCollects, err := models.GetLogCollects()
	if err != nil {
		logger.Warningf("get collect err: %v", err)
	}
	for _, collect := range logCollects {
		list = append(list, collect)
	}

	pluginCollects, err := models.GetPluginCollects()
	if err != nil {
		logger.Warningf("get collect err: %v", err)
	}
	for _, collect := range pluginCollects {
		list = append(list, collect)
	}

	for _, collect := range list {
		var nid, id int64
		var collectType string

		switch collect.(type) {
		case *models.ProcCollect:
			nid = collect.(*models.ProcCollect).Nid
			id = collect.(*models.ProcCollect).Id
			collectType = collect.(*models.ProcCollect).CollectType

		case *models.PortCollect:
			nid = collect.(*models.PortCollect).Nid
			id = collect.(*models.PortCollect).Id
			collectType = collect.(*models.PortCollect).CollectType

		case *models.LogCollect:
			nid = collect.(*models.LogCollect).Nid
			id = collect.(*models.LogCollect).Id
			collectType = collect.(*models.LogCollect).CollectType

		case *models.PluginCollect:
			nid = collect.(*models.PluginCollect).Nid
			id = collect.(*models.PluginCollect).Id
			collectType = collect.(*models.PluginCollect).CollectType

		case *models.ApiCollect:
			nid = collect.(*models.ApiCollect).Nid
			id = collect.(*models.ApiCollect).Id
			collectType = collect.(*models.ApiCollect).CollectType

		case *models.SnmpCollect:
			nid = collect.(*models.SnmpCollect).Nid
			id = collect.(*models.SnmpCollect).Id
			collectType = collect.(*models.SnmpCollect).CollectType
		}

		node, err := models.NodeGet("id=?", nid)
		if err != nil {
			logger.Warningf("get node failed, node id: %d, err: %v", nid, err)
			continue
		}

		if node == nil {
			logger.Infof("delete collect: %+v", collect)
			if err := models.DeleteCollectById(collectType, "sys", id); err != nil {
				logger.Warningf("delete collect %s: %d, err: %v", collectType, id, err)
			}
		}
	}

}

func HostUnderNode(nid int64) ([]string, error) {
	nids, err := cache.GetLeafNidsForMon(nid, []int64{})
	if err != nil {
		return []string{}, err
	}
	rids := cache.NodeResourceCache.GetByNids(nids)
	resources := cache.ResourceCache.GetByIds(rids)
	var idents []string
	for i := range resources {
		idents = append(idents, resources[i].Ident)
	}

	return idents, err
}
