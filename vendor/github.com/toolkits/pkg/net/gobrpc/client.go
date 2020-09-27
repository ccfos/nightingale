package gobrpc

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"time"
)

type RPCClient struct {
	address     string
	rpcClient   *rpc.Client
	callTimeout time.Duration
}

func NewRawClient(network, address string, connTimeout time.Duration) (*rpc.Client, error) {
	conn, err := net.DialTimeout(network, address, connTimeout)
	if err != nil {
		return nil, err
	}
	return rpc.NewClient(conn), err
}

func NewRPCClient(address string, rpcClient *rpc.Client, callTimeout time.Duration) *RPCClient {
	return &RPCClient{
		address:     address,
		rpcClient:   rpcClient,
		callTimeout: callTimeout,
	}
}

func (c *RPCClient) Close() {
	if c.rpcClient != nil {
		c.rpcClient.Close()
		c.rpcClient = nil
	}
}

func (c *RPCClient) IsClose() bool {
	return c.rpcClient == nil
}

func (c *RPCClient) Call(method string, args interface{}, reply interface{}, callTimeout ...time.Duration) error {
	done := make(chan error, 1)

	go func() {
		err := c.rpcClient.Call(method, args, reply)
		done <- err
	}()

	timeout := c.callTimeout
	if len(callTimeout) > 0 {
		timeout = callTimeout[0]
	}

	select {
	case <-time.After(timeout):
		log.Printf("[W] rpc call timeout %v => %v", c.rpcClient, c.address)
		return fmt.Errorf("timeout")
	case err := <-done:
		return err
	}

	return nil
}
