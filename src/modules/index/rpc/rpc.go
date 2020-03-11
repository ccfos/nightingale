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

type Index int

func Start() {
	addr := address.GetRPCListen("index")

	server := rpc.NewServer()
	server.Register(new(Index))

	l, e := net.Listen("tcp", addr)
	if e != nil {
		logger.Fatal("cannot listen ", addr, e)
		os.Exit(1)
	}
	logger.Info("listening ", addr)

	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

	for {
		conn, err := l.Accept()
		if err != nil {
			logger.Warning("listener accept error: ", err)
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
}
