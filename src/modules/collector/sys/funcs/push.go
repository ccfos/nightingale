package funcs

import (
	"bufio"
	"io"
	"math/rand"
	"net"
	"net/rpc"
	"reflect"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/ugorji/go/codec"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/toolkits/address"
)

func Push(items []*dataobj.MetricValue) {
	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

	addrs := address.GetRPCAddresses("transfer")
	count := len(addrs)
	retry := 0
	for {
		for _, i := range rand.Perm(count) {
			addr := addrs[i]
			conn, err := net.DialTimeout("tcp", addr, time.Millisecond*3000)
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
				logger.Info("push succ, reply: ", reply)
				return
			}
		}
		time.Sleep(time.Millisecond * 500)

		retry += 1
		if retry == 3 {
			retry = 0
			break
		}
	}
}
