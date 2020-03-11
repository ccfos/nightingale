package rpc

import (
	"bufio"
	"io"
	"net"
	"net/rpc"
	"os"
	"reflect"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/ugorji/go/codec"

	"github.com/didi/nightingale/src/toolkits/address"
)

var Close_chan, Close_done_chan chan int

func init() {
	Close_chan = make(chan int, 1)
	Close_done_chan = make(chan int, 1)
}

func Start() {
	addr := address.GetRPCListen("judge")

	server := rpc.NewServer()
	server.Register(new(Judge))

	l, e := net.Listen("tcp", addr)
	if e != nil {
		logger.Fatal("cannot listen ", addr, e)
		os.Exit(1)
	}
	logger.Info("rpc listening ", addr)

	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				logger.Error("listener accept error: ", err)
				time.Sleep(time.Duration(100) * time.Millisecond)
				continue
			}

			var bufconn = struct {
				io.Closer
				*bufio.Reader
				*bufio.Writer
			}{conn, bufio.NewReader(conn), bufio.NewWriter(conn)}

			go server.ServeCodec(codec.MsgpackSpecRpc.ServerCodec(bufconn, &mh))
		}
	}()

	select {
	case <-Close_chan:
		logger.Info("rpc, recv sigout and exiting...")
		l.Close()
		Close_done_chan <- 1

		return
	}
}
