package pools

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/rpc"
	"reflect"
	"sync"
	"time"

	"github.com/toolkits/pkg/pool"

	"github.com/ugorji/go/codec"
)

// ConnPools is responsible for the Connection Pool lifecycle management.
type ConnPools struct {
	sync.RWMutex
	P           map[string]*pool.ConnPool
	MaxConns    int
	MaxIdle     int
	ConnTimeout int
	CallTimeout int
}

func NewConnPools(maxConns, maxIdle, connTimeout, callTimeout int, cluster []string) *ConnPools {
	cp := &ConnPools{
		P:           make(map[string]*pool.ConnPool),
		MaxConns:    maxConns,
		MaxIdle:     maxIdle,
		ConnTimeout: connTimeout,
		CallTimeout: callTimeout,
	}

	ct := time.Duration(cp.ConnTimeout) * time.Millisecond
	for _, address := range cluster {
		if _, exist := cp.P[address]; exist {
			continue
		}
		cp.P[address] = createOnePool(address, address, ct, maxConns, maxIdle)
	}
	return cp
}

func createOnePool(name, address string, connTimeout time.Duration, maxConns, maxIdle int) *pool.ConnPool {
	p := pool.NewConnPool(name, address, maxConns, maxIdle)
	p.New = func(connName string) (pool.NConn, error) {
		// valid address
		_, err := net.ResolveTCPAddr("tcp", p.Address)
		if err != nil {
			return nil, err
		}

		conn, err := net.DialTimeout("tcp", p.Address, connTimeout)
		if err != nil {
			return nil, err
		}
		var mh codec.MsgpackHandle
		mh.MapType = reflect.TypeOf(map[string]interface{}(nil))

		// bufconn here is a buffered io.ReadWriteCloser
		var bufconn = struct {
			io.Closer
			*bufio.Reader
			*bufio.Writer
		}{Closer: conn, Reader: bufio.NewReader(conn), Writer: bufio.NewWriter(conn)}

		rpcCodec := codec.MsgpackSpecRpc.ClientCodec(bufconn, &mh)
		return RpcClient{cli: rpc.NewClientWithCodec(rpcCodec), name: connName}, nil
	}
	return p
}

// Call will block until request failed or timeout.
func (cp *ConnPools) Call(addr, method string, args interface{}, resp interface{}) error {
	var selectedPool *pool.ConnPool
	var exists bool

	// if address is empty, we will select a available pool from cp.P randomly.
	// map-range function gets random keys order every time.
	if addr == "" {
		for _, p := range cp.P {
			if p != nil {
				selectedPool = p
				break
			}
		}
	} else {
		selectedPool, exists = cp.Get(addr)
		if !exists {
			return fmt.Errorf("%s has no connection pool", addr)
		}
	}

	// make sure the selected pool alive.
	if selectedPool == nil {
		return fmt.Errorf("no connection pool available")
	}

	connPool := selectedPool
	conn, err := connPool.Fetch()
	if err != nil {
		return fmt.Errorf("%s get connection fail: conn %v, err %v. proc: %s", addr, conn, err, connPool.Proc())
	}

	rpcClient := conn.(RpcClient)
	callTimeout := time.Duration(cp.CallTimeout) * time.Millisecond

	done := make(chan error, 1)
	go func() {
		done <- rpcClient.Call(method, args, resp)
	}()

	select {
	case <-time.After(callTimeout):
		connPool.ForceClose(conn)
		return fmt.Errorf("%s, call timeout", addr)
	case err = <-done:
		if err != nil {
			connPool.ForceClose(conn)
			err = fmt.Errorf("%s, call failed, err %v. proc: %s", addr, err, connPool.Proc())
		} else {
			connPool.Release(conn)
		}
		return err
	}
}

func (cp *ConnPools) Get(address string) (*pool.ConnPool, bool) {
	cp.RLock()
	defer cp.RUnlock()

	p, exists := cp.P[address]
	return p, exists
}

func (cp *ConnPools) UpdatePools(addrs []string) []string {
	cp.Lock()
	defer cp.Unlock()

	newAddrs := make([]string, 0)
	if len(addrs) == 0 {
		cp.P = make(map[string]*pool.ConnPool)
		return newAddrs
	}
	addrMap := make(map[string]struct{})

	ct := time.Duration(cp.ConnTimeout) * time.Millisecond
	for _, addr := range addrs {
		addrMap[addr] = struct{}{}
		_, exists := cp.P[addr]
		if exists {
			continue
		}
		newAddrs = append(newAddrs, addr)
		cp.P[addr] = createOnePool(addr, addr, ct, cp.MaxConns, cp.MaxIdle)
	}

	// remove a pool from cp.P
	for addr := range cp.P {
		if _, exists := addrMap[addr]; !exists {
			delete(cp.P, addr)
		}
	}

	return newAddrs
}

// RpcClient implements the io.Closer interface
type RpcClient struct {
	cli  *rpc.Client
	name string
}

func (rc RpcClient) Name() string {
	return rc.name
}

func (rc RpcClient) Closed() bool {
	return rc.cli == nil
}

func (rc RpcClient) Close() error {
	if rc.cli != nil {
		err := rc.cli.Close()
		rc.cli = nil
		return err
	}
	return nil
}

func (rc RpcClient) Call(method string, args, reply interface{}) error {
	return rc.cli.Call(method, args, reply)
}
