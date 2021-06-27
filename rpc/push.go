package rpc

import (
	"github.com/didi/nightingale/v5/judge"
	"github.com/didi/nightingale/v5/trans"
	"github.com/didi/nightingale/v5/vos"
)

// 通过普通rpc的方式(msgpack)上报数据
func (*Server) PushToTrans(points []*vos.MetricPoint, reply *string) error {
	err := trans.Push(points)
	if err != nil {
		*reply = err.Error()
	}
	return nil
}

// server内部做数据重排，推送数据给告警引擎
func (*Server) PushToJudge(points []*vos.MetricPoint, reply *string) error {
	go judge.Send(points)
	return nil
}
