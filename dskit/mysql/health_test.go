package mysql

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/pool"
	"github.com/stretchr/testify/require"
)

func poolSize() int {
	n := 0
	pool.PoolClient.Range(func(_, _ interface{}) bool { n++; return true })
	return n
}

func TestCheckHealthClosedPort(t *testing.T) {
	m := &MySQL{Shards: []Shard{{Addr: "127.0.0.1:1", User: "root", Password: "root"}}}
	before := poolSize()

	err := m.CheckHealth(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "mysql connect failed")
	require.Equal(t, before, poolSize(), "health check must not populate the shared pool")
}

func TestCheckHealthHonorsContextDeadline(t *testing.T) {
	// Accept TCP but never speak the MySQL protocol, so only ctx can end the handshake.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
		}
	}()

	m := &MySQL{Shards: []Shard{{Addr: ln.Addr().String(), User: "root"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	require.Error(t, m.CheckHealth(ctx))
	require.Less(t, time.Since(start), 5*time.Second)
}

func TestCheckHealthInvalidConfig(t *testing.T) {
	require.Error(t, (&MySQL{}).CheckHealth(context.Background()))
	require.Error(t, (&MySQL{Shards: []Shard{{User: "root"}}}).CheckHealth(context.Background()))
	require.Error(t, (&MySQL{Shards: []Shard{{Addr: "127.0.0.1:1", User: "root", DsnExtraParams: "bad"}}}).CheckHealth(context.Background()))
}
