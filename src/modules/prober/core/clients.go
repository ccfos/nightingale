package core

import (
	"net/rpc"
	"sync"
)

type RpcClientContainer struct {
	M map[string]*rpc.Client
	sync.RWMutex
}

var rpcClients *RpcClientContainer

func InitRpcClients() {
	rpcClients = &RpcClientContainer{
		M: make(map[string]*rpc.Client),
	}
}

func (rcc *RpcClientContainer) Get(addr string) *rpc.Client {
	rcc.RLock()
	defer rcc.RUnlock()

	client, has := rcc.M[addr]
	if !has {
		return nil
	}

	return client
}

// Put 返回的bool表示affected，确实把自己塞进去了
func (rcc *RpcClientContainer) Put(addr string, client *rpc.Client) bool {
	rcc.Lock()
	defer rcc.Unlock()

	oc, has := rcc.M[addr]
	if has && oc != nil {
		return false
	}

	rcc.M[addr] = client
	return true
}

func (rcc *RpcClientContainer) Del(addr string) {
	rcc.Lock()
	defer rcc.Unlock()
	delete(rcc.M, addr)
}
