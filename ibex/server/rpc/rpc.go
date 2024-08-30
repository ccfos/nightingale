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

	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
	"github.com/ugorji/go/codec"
)

type Server int

var ctxC *ctx.Context

func Start(listen string, ctx *ctx.Context) {
	ctxC = ctx
	go serve(listen)
}

func serve(listen string) {
	server := rpc.NewServer()
	server.Register(new(Server))

	l, err := net.Listen("tcp", listen)
	if err != nil {
		fmt.Printf("fail to listen on: %s, error: %v\n", listen, err)
		os.Exit(1)
	}

	fmt.Println("rpc.listening:", listen)

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
