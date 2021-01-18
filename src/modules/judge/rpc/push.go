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
	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/judge/cache"
	"github.com/didi/nightingale/src/modules/judge/judge"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/logger"
)

type Judge int

func (j *Judge) Ping(req dataobj.NullRpcRequest, resp *dataobj.SimpleRpcResponse) error {
	return nil
}

func (j *Judge) Send(items []*dataobj.JudgeItem, resp *dataobj.SimpleRpcResponse) error {
	// 把当前时间的计算放在最外层，是为了减少获取时间时的系统调用开销

	for _, item := range items {
		now := item.Timestamp
		pk := item.MD5()
		logger.Debugf("recv-->%+v", item)
		stats.Counter.Set("push.in", 1)

		go judge.ToJudge(cache.HistoryBigMap[pk[0:2]], pk, item, now)
	}

	return nil
}
