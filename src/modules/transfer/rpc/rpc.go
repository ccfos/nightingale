package rpc

import (
	"bufio"
	"io"
	"net"
	"net/rpc"
	"os"
	"reflect"
	"time"

	"github.com/didi/nightingale/src/common/address"

	"github.com/toolkits/pkg/logger"
	"github.com/ugorji/go/codec"
)

type Transfer int

func Start() {
	go consumer()
	addr := address.GetRPCListen("transfer")

	server := rpc.NewServer()
	server.Register(new(Transfer))

	l, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatalf("fail to connect address: [%s], error: %v", addr, err)
		os.Exit(1)
	}
	logger.Infof("server is available at:[%s]", addr)

	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

	for {
		conn, err := l.Accept()
		if err != nil {
			logger.Warningf("listener accept error: %v", err)
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
