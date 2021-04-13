package rpc

import (
	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/didi/nightingale/v4/src/modules/server/judge"
)

func (*Server) Send(items []*dataobj.JudgeItem, resp *dataobj.SimpleRpcResponse) error {
	// 把当前时间的计算放在最外层，是为了减少获取时间时的系统调用开销
	judge.Send(items)

	return nil
}
