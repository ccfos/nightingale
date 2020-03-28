package funcs

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/rpc"
	"reflect"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/ugorji/go/codec"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/cache"
	"github.com/didi/nightingale/src/toolkits/address"
	"github.com/didi/nightingale/src/toolkits/identity"
)

func Push(metricItems []*dataobj.MetricValue) error {
	var err error
	var items []*dataobj.MetricValue
	for _, item := range metricItems {
		logger.Debug("->recv: ", item)
		if item.Endpoint == "" {
			item.Endpoint = identity.Identity
		}
		err = item.CheckValidity()
		if err != nil {
			msg := fmt.Errorf("metric:%v err:%v", item, err)
			logger.Warning(msg)
			return msg
		}
		if item.CounterType == dataobj.COUNTER {
			if err := CounterToGauge(item); err != nil {
				//旧值不存在则不推送
				logger.Warning(err)
				continue
			}
		}

		items = append(items, item)
	}

	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

	addrs := address.GetRPCAddresses("transfer")
	count := len(addrs)
	retry := 0
	for {
		for _, i := range rand.Perm(count) {
			addr := addrs[i]
			var conn net.Conn
			conn, err = net.DialTimeout("tcp", addr, time.Millisecond*3000)
			if err != nil {
				logger.Error("dial transfer err:", err)
				continue
			}

			var bufconn = struct { // bufconn here is a buffered io.ReadWriteCloser
				io.Closer
				*bufio.Reader
				*bufio.Writer
			}{conn, bufio.NewReader(conn), bufio.NewWriter(conn)}

			rpcCodec := codec.MsgpackSpecRpc.ClientCodec(bufconn, &mh)
			client := rpc.NewClientWithCodec(rpcCodec)

			var reply dataobj.TransferResp
			err = client.Call("Transfer.Push", items, &reply)
			client.Close()
			if err != nil {
				logger.Error(err)
				continue
			} else {
				if reply.Msg != "ok" {
					err = fmt.Errorf("some item push err", reply)
					logger.Error(err)
				}
				return err
			}
		}
		time.Sleep(time.Millisecond * 500)

		retry += 1
		if retry == 3 {
			retry = 0
			break
		}
	}
	return err
}

func CounterToGauge(item *dataobj.MetricValue) error {
	key := item.PK()
	old, exists := cache.MetricHistory.Get(key)
	if !exists {
		cache.MetricHistory.Set(key, *item)
		return fmt.Errorf("not found old item:%v", item)
	}

	cache.MetricHistory.Set(key, *item)
	if old.Value > item.Value {
		return fmt.Errorf("item:%v old value:%v greater than new value:%v", item, old.Value, item.Value)
	}
	item.ValueUntyped = item.Value - old.Value
	item.CounterType = dataobj.GAUGE
	return nil
}
