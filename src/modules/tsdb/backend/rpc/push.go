package rpc

import (
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

const (
	ALLINDEX  = 0
	INCRINDEX = 1
)

func Push2Index(mode int, items []*dataobj.TsdbItem, indexAddrs []string) {
	for _, addr := range indexAddrs {
		//TODO 改为并发
		push(mode, addr, items)
	}
}

func push(mode int, addr string, tsdbItems []*dataobj.TsdbItem) {
	resp := &dataobj.IndexResp{}
	var err error
	sendOk := false

	if len(tsdbItems) == 0 {
		return
	}

	itemCount := int64(len(tsdbItems))

	bodyList := make([]*dataobj.IndexModel, itemCount)
	for i, item := range tsdbItems {
		logger.Debugf("mode:%d push index:%v to:%s", mode, item, addr)

		var tmp dataobj.IndexModel
		tmp.Endpoint = item.Endpoint
		tmp.Metric = item.Metric
		tmp.Step = item.Step
		tmp.DsType = item.DsType
		if len(item.TagsMap) == 0 {
			tmp.Tags = make(map[string]string)
		} else {
			tmp.Tags = item.TagsMap
		}
		tmp.Timestamp = item.Timestamp
		bodyList[i] = &tmp
	}

	for i := 0; i < 3; i++ { //最多重试3次
		if mode == INCRINDEX {
			err = IndexConnPools.Call(addr, "Index.IncrPush", bodyList, resp)
			stats.Counter.Set("index.out.incr", int(itemCount))
		} else {
			err = IndexConnPools.Call(addr, "Index.Push", bodyList, resp)
			stats.Counter.Set("index.out", int(itemCount))
		}
		if err == nil {
			sendOk = true
			break
		}
		if resp.Msg != "" {
			logger.Warning(resp.Msg)
		}
		time.Sleep(time.Millisecond * 10)
	}

	if !sendOk {
		stats.Counter.Set("index.out.err", int(itemCount))

		logger.Errorf("send %v to index %s fail: %v", bodyList, addr, err)
	}
}
