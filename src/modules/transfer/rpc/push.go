package rpc

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/backend"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

func (t *Transfer) Ping(args string, reply *string) error {
	*reply = args
	return nil
}

func (t *Transfer) Push(args []*dataobj.MetricValue, reply *dataobj.TransferResp) error {
	start := time.Now()
	reply.Invalid = 0

	items := make([]*dataobj.MetricValue, 0)
	for _, v := range args {
		logger.Debug("->recv: ", v)
		stats.Counter.Set("points.in", 1)
		if err := v.CheckValidity(start.Unix()); err != nil {
			stats.Counter.Set("points.in.err", 1)
			msg := fmt.Sprintf("illegal item:%s err:%v", v, err)
			logger.Warningf(msg)
			reply.Invalid += 1
			reply.Msg += msg
			continue
		}

		items = append(items, v)
	}

	if backend.Config.Enabled {
		backend.Push2TsdbSendQueue(items)
	}

	if backend.Config.Enabled {
		backend.Push2JudgeSendQueue(items)
	}

	if reply.Invalid == 0 {
		reply.Msg = "ok"
	}
	reply.Total = len(args)
	reply.Latency = (time.Now().UnixNano() - start.UnixNano()) / 1000000
	return nil
}
