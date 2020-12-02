package rpc

import (
	"fmt"
	"net"
	"net/rpc"
	"os"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/common/address"
)

// Scheduler rpc cursor
type Scheduler int

// Start rpc server
func Start() {
	addr := address.GetRPCListen("job")

	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		fmt.Println("net.ResolveTCPAddr fail:", err)
		os.Exit(2)
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		fmt.Printf("listen %s fail: %s\n", addr, err)
		os.Exit(3)
	} else {
		fmt.Println("rpc.listening:", addr)
	}

	rpc.Register(new(Scheduler))

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Warning("listener.Accept occur error:", err)
			continue
		}
		go rpc.ServeConn(conn)
	}
}
