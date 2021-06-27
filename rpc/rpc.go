package rpc

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"os"
	"reflect"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/ugorji/go/codec"

	"github.com/didi/nightingale/v5/config"
)

type Server int

func Start() {
	go serve()
}

func serve() {
	addr := config.Config.RPC.Listen

	server := rpc.NewServer()
	server.Register(new(Server))

	l, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Printf("fail to listen on: %s, error: %v\n", addr, err)
		os.Exit(1)
	}

	fmt.Println("rpc.listening:", addr)

	var mh codec.MsgpackHandle
	mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

	duration := time.Duration(100) * time.Millisecond

	for {
		conn, err := l.Accept()
		if err != nil {
			logger.Warningf("listener accept error: %v", err)
			time.Sleep(duration)
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
