package client

import (
	"net"
	"net/rpc"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/gobrpc"

	"github.com/didi/nightingale/src/common/address"
)

var cli *gobrpc.RPCClient

func getCli() *gobrpc.RPCClient {
	if cli != nil {
		return cli
	}

	servers := address.GetRPCAddresses("job")

	// detect the fastest server
	var (
		address  string
		client   *rpc.Client
		duration int64 = 999999999999
	)

	// auto close other slow server
	acm := make(map[string]*rpc.Client)

	l := len(servers)
	for i := 0; i < l; i++ {
		addr := servers[i]
		begin := time.Now()
		conn, err := net.DialTimeout("tcp", addr, time.Second*5)
		if err != nil {
			logger.Warningf("dial %s fail: %s", addr, err)
			continue
		}

		c := rpc.NewClient(conn)
		acm[addr] = c

		var out string
		err = c.Call("Scheduler.Ping", "", &out)
		if err != nil {
			logger.Warningf("ping %s fail: %s", addr, err)
			continue
		}
		use := time.Since(begin).Nanoseconds()

		if use < duration {
			address = addr
			client = c
			duration = use
		}
	}

	if address == "" {
		logger.Errorf("no job server found")
		return nil
	}

	logger.Infof("choose server: %s, duration: %dms", address, duration/1000000)

	for addr, c := range acm {
		if addr == address {
			continue
		}
		c.Close()
	}

	cli = gobrpc.NewRPCClient(address, client, 5*time.Second)
	return cli
}

// GetCli 探测所有server端的延迟，自动选择最快的
func GetCli() *gobrpc.RPCClient {
	for {
		c := getCli()
		if c != nil {
			return c
		}

		time.Sleep(time.Second * 10)
	}
}

// CloseCli 关闭客户端连接
func CloseCli() {
	if cli != nil {
		cli.Close()
		cli = nil
	}
}
