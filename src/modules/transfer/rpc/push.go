// Copyright 2017 Xiaomi, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpc

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/aggr"
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
			msg := fmt.Sprintf("illegal item:%+v err:%v", v, err)
			logger.Warningf(msg)
			reply.Invalid += 1
			reply.Msg += msg
			continue
		}

		items = append(items, v)
	}

	// send to judge
	backend.Push2JudgeQueue(items)

	if aggr.AggrConfig.Enabled {
		go aggr.SendToAggr(items)
	}

	// send to push endpoints
	pushEndpoints, err := backend.GetPushEndpoints()
	if err != nil {
		logger.Errorf("could not find pushendpoint")
		return err
	} else {
		for _, pushendpoint := range pushEndpoints {
			pushendpoint.Push2Queue(items)
		}
	}

	if reply.Invalid == 0 {
		reply.Msg = "ok"
	}
	reply.Total = len(args)
	reply.Latency = (time.Now().UnixNano() - start.UnixNano()) / 1000000
	return nil
}

func PushData(args []*dataobj.MetricValue) (int, string) {
	start := time.Now()

	items := make([]*dataobj.MetricValue, 0)
	var errCount int
	var errMsg string
	for _, v := range args {
		logger.Debug("->recv: ", v)
		stats.Counter.Set("points.in", 1)
		if err := v.CheckValidity(start.Unix()); err != nil {
			stats.Counter.Set("points.in.err", 1)
			msg := fmt.Sprintf("illegal item:%+v err:%v", v, err)
			logger.Warningf(msg)
			errCount += 1
			errMsg += msg
			continue
		}

		items = append(items, v)
	}

	// send to judge
	backend.Push2JudgeQueue(items)

	if aggr.AggrConfig.Enabled {
		go aggr.SendToAggr(items)
	}

	// send to push endpoints
	pushEndpoints, err := backend.GetPushEndpoints()
	if err != nil {
		errMsg += fmt.Sprintf("could not find pushendpoint:%v", err)
	} else {
		for _, pushendpoint := range pushEndpoints {
			pushendpoint.Push2Queue(items)
		}
	}

	return errCount, errMsg
}
