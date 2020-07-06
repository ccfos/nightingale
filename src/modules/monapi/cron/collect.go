package cron

import (
	"time"

	"github.com/didi/nightingale/src/model"
	"github.com/toolkits/pkg/logger"
)

func CleanCollectLoop() {
	duration := time.Second * time.Duration(300)
	for {
		time.Sleep(duration)
		CleanCollect()
	}
}

//定期清理没有找到nid的采集策略
func CleanCollect() {
	var list []interface{}
	collects, err := model.GetPortCollects()
	if err != nil {
		logger.Warningf("get collect err: %v", err)
	}
	for _, collect := range collects {
		list = append(list, collect)
	}

	procCollects, err := model.GetProcCollects()
	if err != nil {
		logger.Warningf("get collect err: %v", err)
	}
	for _, collect := range procCollects {
		list = append(list, collect)
	}

	logCollects, err := model.GetLogCollects()
	if err != nil {
		logger.Warningf("get collect err: %v", err)
	}
	for _, collect := range logCollects {
		list = append(list, collect)
	}

	pluginCollects, err := model.GetPluginCollects()
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
		case *model.ProcCollect:
			nid = collect.(*model.ProcCollect).Nid
			id = collect.(*model.ProcCollect).Id
			collectType = collect.(*model.ProcCollect).CollectType

		case *model.PortCollect:
			nid = collect.(*model.PortCollect).Nid
			id = collect.(*model.PortCollect).Id
			collectType = collect.(*model.PortCollect).CollectType

		case *model.LogCollect:
			nid = collect.(*model.LogCollect).Nid
			id = collect.(*model.LogCollect).Id
			collectType = collect.(*model.LogCollect).CollectType

		case *model.PluginCollect:
			nid = collect.(*model.PluginCollect).Nid
			id = collect.(*model.PluginCollect).Id
			collectType = collect.(*model.PluginCollect).CollectType
		}

		node, err := model.NodeGet("id", nid)
		if err != nil {
			logger.Warningf("get node failed, node id: %d, err: %v", nid, err)
			continue
		}

		if node == nil {
			logger.Infof("delete collect: %+v", collect)
			if err := model.DeleteCollectById(collectType, "sys", id); err != nil {
				logger.Warningf("delete collect %s: %d, err: %v", collectType, id, err)
			}
		}
	}

}
