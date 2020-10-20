package gobrpc

import (
	"fmt"
	"log"
	"math/rand"
	"net/rpc"
	"strings"
	"sync"
	"time"
)

type Clients struct {
	sync.RWMutex
	clients     map[string]*RPCClient
	addresses   []string
	callTimeout time.Duration
}

func NewClients(addresses []string, callTimeout time.Duration) *Clients {
	cs := &Clients{}
	cs.addresses = addresses
	cs.clients = make(map[string]*RPCClient)

	count := len(addresses)
	if count == 0 {
		log.Fatalln("[F] addresses are empty")
	}

	for i := 0; i < count; i++ {
		endpoint := addresses[i]
		client, err := NewRawClient("tcp", endpoint, callTimeout)
		if err != nil {
			log.Printf("[E] cannot connect to", endpoint)
			cs.clients[endpoint] = nil
			continue
		}
		cs.clients[endpoint] = NewRPCClient(endpoint, client, callTimeout)
	}

	return cs
}

func (cs *Clients) SetClients(clients map[string]*RPCClient) {
	cs.Lock()
	cs.clients = clients
	cs.Unlock()
}

func (cs *Clients) PutClient(addr string, client *RPCClient) {
	cs.Lock()
	c, has := cs.clients[addr]
	if has && c != nil {
		c.Close()
	}

	cs.clients[addr] = client
	cs.Unlock()
}

func (cs *Clients) GetClient(addr string) (*RPCClient, bool) {
	cs.RLock()
	c, has := cs.clients[addr]
	cs.RUnlock()
	return c, has
}

func (cs *Clients) Call(method string, args, reply interface{}, callTimeout time.Duration) error {
	l := len(cs.addresses)
	for _, i := range rand.Perm(l) {
		addr := cs.addresses[i]
		client, has := cs.GetClient(addr)
		if !has {
			log.Println("[W]", addr, "has no client")
			continue
		}

		if client.IsClose() {
			rawClient, err := NewRawClient("tcp", addr, callTimeout)
			if err != nil {
				log.Println("[W]", addr, "is dead")
				continue
			}

			client = NewRPCClient(addr, rawClient, cs.callTimeout)
			cs.PutClient(addr, client)
		}

		err := client.Call(method, args, reply, callTimeout)
		if err == nil {
			return nil
		}

		client.Close()

		if err == rpc.ErrShutdown || strings.Contains(err.Error(), "connection refused") {
			continue
		} else {
			return err
		}
	}

	return fmt.Errorf("[E] all backends are dead")
}
