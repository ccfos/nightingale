package dscache

import (
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	ckds "github.com/ccfos/nightingale/v6/datasource/ck"
	"github.com/stretchr/testify/require"
)

type refreshClosingDatasource struct {
	*cacheTestDatasource
	closed chan struct{}
}

func (d *refreshClosingDatasource) Close() error {
	d.closeCalls++
	close(d.closed)
	return errors.New("old pool close failed")
}

func TestPutDatasources_ReplacesConfiguredPluginAndRemovesStaleSibling(t *testing.T) {
	previous := DsCache
	DsCache = *newBehaviorCache()
	t.Cleanup(func() { DsCache = previous })
	old := &refreshClosingDatasource{cacheTestDatasource: &cacheTestDatasource{}, closed: make(chan struct{})}
	stale := &cacheTestDatasource{}
	DsCache.Put("ck", 1, old)
	DsCache.Put("ck", 2, stale)

	PutDatasources([]datasource.DatasourceInfo{{
		Id: 1, Name: "logs", Type: "ck.logging", Settings: map[string]interface{}{
			"ck.nodes": []string{"127.0.0.1:8123"}, "ck.user": "reader", "ck.protocol": "http",
		},
	}}, true)
	// The old instance is closed after the asynchronous replacement has been published.
	<-old.closed
	got, found := DsCache.Get("ck", 1)

	require.True(t, found)
	client, ok := got.(*ckds.Clickhouse)
	require.True(t, ok)
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	require.Equal(t, "reader", client.User)
	require.Equal(t, "http", client.Protocol)
	require.Equal(t, 10000, client.Timeout)
	require.Equal(t, 500, client.MaxQueryRows)
	require.NotNil(t, client.ClientByHTTP)
	require.Equal(t, 1, old.closeCalls)
	require.Equal(t, 1, stale.closeCalls)
	_, found = DsCache.Get("ck", 2)
	require.False(t, found)
	require.Equal(t, map[string][]int64{"ck": {1}}, DsCache.GetAllIds())
}
