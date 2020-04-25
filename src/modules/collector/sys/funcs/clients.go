package funcs

import (
	"net/rpc"
	"sync"
)

type RpcClients struct {
	Clients map[string]*rpc.Client
	sync.RWMutex
}

var rpcClients *RpcClients

func InitRpcClients() {
	rpcClients = &RpcClients{
		Clients: make(map[string]*rpc.Client),
	}
}

// Get returns a Client from rc.Clients if existed.
func (rc *RpcClients) Get(addr string) *rpc.Client {
	rc.RLock()
	defer rc.RUnlock()

	client, has := rc.Clients[addr]
	if !has {
		return nil
	}

	return client
}

// Put returns true if the client is placed into rcc.Clients.
func (rc *RpcClients) Put(addr string, client *rpc.Client) bool {
	rc.Lock()
	defer rc.Unlock()

	oc, has := rc.Clients[addr]
	if has && oc != nil {
		return false
	}

	rc.Clients[addr] = client
	return true
}

// Delete deletes a client from rcc.Clients.
func (rc *RpcClients) Del(addr string) {
	rc.Lock()
	defer rc.Unlock()
	delete(rc.Clients, addr)
}
