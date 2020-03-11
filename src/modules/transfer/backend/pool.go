package backend

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

// 每个后端backend对应一个ConnPool
type ConnPools struct {
	sync.RWMutex
	M           map[string]*pool.ConnPool
	MaxConns    int
	MaxIdle     int
	ConnTimeout int
	CallTimeout int
}

func CreateConnPools(maxConns, maxIdle, connTimeout, callTimeout int, cluster []string) *ConnPools {
	cp := &ConnPools{M: make(map[string]*pool.ConnPool), MaxConns: maxConns, MaxIdle: maxIdle,
		ConnTimeout: connTimeout, CallTimeout: callTimeout}

	ct := time.Duration(cp.ConnTimeout) * time.Millisecond
	for _, address := range cluster {
		if _, exist := cp.M[address]; exist {
			continue
		}
		cp.M[address] = createOnePool(address, address, ct, maxConns, maxIdle)
	}

	return cp
}

func createOnePool(name string, address string, connTimeout time.Duration, maxConns int, maxIdle int) *pool.ConnPool {
	p := pool.NewConnPool(name, address, maxConns, maxIdle)
	p.New = func(connName string) (pool.NConn, error) {
		//校验地址是否正确
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

		var bufconn = struct { // bufconn here is a buffered io.ReadWriteCloser
			io.Closer
			*bufio.Reader
			*bufio.Writer
		}{conn, bufio.NewReader(conn), bufio.NewWriter(conn)}

		rpcCodec := codec.MsgpackSpecRpc.ClientCodec(bufconn, &mh)
		return RpcClient{cli: rpc.NewClientWithCodec(rpcCodec), name: connName}, nil
	}
	return p
}

func (cp *ConnPools) Update(cluster []string) {
	cp.Lock()
	defer cp.Unlock()

	maxConns := Config.MaxConns
	maxIdle := Config.MaxIdle
	ct := time.Duration(cp.ConnTimeout) * time.Millisecond
	newCluster := make(map[string]struct{})
	for _, address := range cluster {
		newCluster[address] = struct{}{}
		if _, exist := cp.M[address]; exist {
			continue
		}
		cp.M[address] = createOnePool(address, address, ct, maxConns, maxIdle)
	}

	for address, _ := range cp.M {
		if _, exists := newCluster[address]; !exists {
			delete(cp.M, address)
		}
	}
}

// 同步发送, 完成发送或超时后 才能返回
func (this *ConnPools) Call(addr, method string, args interface{}, resp interface{}) error {
	connPool, exists := this.Get(addr)
	if !exists {
		return fmt.Errorf("%s has no connection pool", addr)
	}

	conn, err := connPool.Fetch()
	if err != nil {
		return fmt.Errorf("%s get connection fail: conn %v, err %v. proc: %s", addr, conn, err, connPool.Proc())
	}

	rpcClient := conn.(RpcClient)
	callTimeout := time.Duration(this.CallTimeout) * time.Millisecond

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

func (this *ConnPools) Get(address string) (*pool.ConnPool, bool) {
	this.RLock()
	defer this.RUnlock()
	p, exists := this.M[address]
	return p, exists
}

// RpcCient, 要实现io.Closer接口
type RpcClient struct {
	cli  *rpc.Client
	name string
}

func (this RpcClient) Name() string {
	return this.name
}

func (this RpcClient) Closed() bool {
	return this.cli == nil
}

func (this RpcClient) Close() error {
	if this.cli != nil {
		err := this.cli.Close()
		this.cli = nil
		return err
	}
	return nil
}

func (this RpcClient) Call(method string, args interface{}, reply interface{}) error {
	return this.cli.Call(method, args, reply)
}
