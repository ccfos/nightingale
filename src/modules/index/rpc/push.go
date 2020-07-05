package rpc

import (
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/index/cache"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

func (idx *Index) Ping(args string, reply *string) error {
	*reply = args
	return nil
}

func (idx *Index) IncrPush(args []*dataobj.IndexModel, reply *dataobj.IndexResp) error {
	push(args, reply)
	stats.Counter.Set("index.incr.in", len(args))
	return nil
}

func (idx *Index) Push(args []*dataobj.IndexModel, reply *dataobj.IndexResp) error {
	push(args, reply)
	stats.Counter.Set("index.all.in", len(args))

	return nil
}

func push(args []*dataobj.IndexModel, reply *dataobj.IndexResp) {
	start := time.Now()
	reply.Invalid = 0
	now := time.Now().Unix()
	for _, item := range args {
		logger.Debugf("<---index %v", item)
		cache.IndexDB.Push(*item, now)
	}

	reply.Total = len(args)
	reply.Latency = (time.Now().UnixNano() - start.UnixNano()) / 1000000
}
