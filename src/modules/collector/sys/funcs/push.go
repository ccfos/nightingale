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

	addrs := address.GetRPCAddresses("transfer")
	count := len(addrs)
	retry := 0
	for {
		for _, i := range rand.Perm(count) {
			addr := addrs[i]
			reply, err := rpcCall(addr, items)
			if err != nil {
				logger.Error(err)
				continue
			} else {
				if reply.Msg != "ok" {
					err = fmt.Errorf("some item push err: %s", reply.Msg)
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

func rpcCall(addr string, items []*dataobj.MetricValue) (dataobj.TransferResp, error) {
	var reply dataobj.TransferResp
	var err error

	affected := false
	client := rpcClients.Get(addr)
	if client == nil {
		client, err = rpcClient(addr)
		if err != nil {
			return reply, err
		} else {
			affected = rpcClients.Put(addr, client)
		}
	}

	timeout := time.Duration(8) * time.Second
	done := make(chan error, 1)

	go func() {
		err := client.Call("Transfer.Push", items, &reply)
		done <- err
	}()

	select {
	case <-time.After(timeout):
		logger.Warningf("rpc call timeout, transfer addr: %s", addr)
		rpcClients.Put(addr, nil)
		client.Close()
		return reply, fmt.Errorf("%s rpc call timeout", addr)
	case err := <-done:
		if err != nil {
			rpcClients.Put(addr, nil)
			client.Close()
			return reply, err
		}
	}

	if !affected {
		// 我尝试把自己这个client塞进map失败，说明已经有一个client塞进去了，那我自己用完了就关闭
		client.Close()
	}

	return reply, nil
}

func rpcClient(addr string) (*rpc.Client, error) {
	conn, err := net.DialTimeout("tcp", addr, time.Second*3)
	if err != nil {
		err = fmt.Errorf("dial transfer %s fail: %v", addr, err)
		logger.Error(err)
		return nil, err
	}

	var bufConn = struct {
		io.Closer
		*bufio.Reader
		*bufio.Writer
	}{conn, bufio.NewReader(conn), bufio.NewWriter(conn)}

	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

	rpcCodec := codec.MsgpackSpecRpc.ClientCodec(bufConn, &mh)
	client := rpc.NewClientWithCodec(rpcCodec)
	return client, nil
}

func CounterToGauge(item *dataobj.MetricValue) error {
	key := item.PK()

	old, exists := cache.MetricHistory.Get(key)
	cache.MetricHistory.Set(key, *item)

	if !exists {
		return fmt.Errorf("not found old item:%v", item)
	}

	if old.Value > item.Value {
		return fmt.Errorf("item:%v old value:%v greater than new value:%v", item, old.Value, item.Value)
	}

	if old.Timestamp >= item.Timestamp {
		return fmt.Errorf("item:%v old timestamp:%v greater than new timestamp:%v", item, old.Timestamp, item.Timestamp)
	}

	item.ValueUntyped = (item.Value - old.Value) / float64(item.Timestamp-old.Timestamp)
	item.CounterType = dataobj.GAUGE
	return nil
}
