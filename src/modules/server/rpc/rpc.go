package rpc

import (
	"bufio"
	"io"
	"net"
	"net/rpc"
	"os"
	"reflect"
	"time"

	"github.com/didi/nightingale/v4/src/common/address"

	"github.com/toolkits/pkg/logger"
	"github.com/ugorji/go/codec"
)

type Server int

func Start() {
	addr := address.GetRPCListen("server")

	server := rpc.NewServer()
	server.Register(new(Server))

	l, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Fatalf("fail to connect address: [%s], error: %v", addr, err)
		os.Exit(1)
	}
	logger.Infof("server is available at:[%s]", addr)

	go consumer()

	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

	for {
		conn, err := l.Accept()
		if err != nil {
			logger.Warningf("listener accept error: %v", err)
			time.Sleep(time.Duration(100) * time.Millisecond)
			continue
		}

		var bufConn = struct {
			io.Closer
			*bufio.Reader
			*bufio.Writer
		}{conn, bufio.NewReader(conn), bufio.NewWriter(conn)}

		go server.ServeCodec(codec.MsgpackSpecRpc.ServerCodec(bufConn, &mh))
	}
}
