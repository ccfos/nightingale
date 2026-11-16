package router

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/center/metas"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pushgw/idents"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestR8bHeartbeatPersistsAndCachesHostFields(t *testing.T) {
	rt := r8bRouter(t, &models.Target{}, &models.TargetBusiGroup{})
	cached := &models.Target{Ident: "host", GroupIds: []int64{5}, HostIp: "old", HostTags: []string{"env=old"}, Tags: "env=user keep=yes ", EngineName: "old", AgentVersion: "v1", OS: "old"}
	require.NoError(t, rt.Ctx.DB.Create(cached).Error)
	cache := &memsto.TargetCacheType{}
	cache.Set(map[string]*models.Target{"host": cached}, 1, 1)
	// These production constructors own their in-memory queues; nil Redis keeps
	// their periodic persistence independent of the test database.
	meta := metas.New(nil)
	ident := idents.New(rt.Ctx, nil, pconf.Pushgw{UpdateTargetBatchSize: 100, UpdateTargetRetryCount: 1, UpdateTargetTimeoutMills: 100})
	for i, query := range []string{"?gid=bad,5", "?overwrite_gids=true&gid=,bad,6", ""} {
		payload := models.HostMeta{Hostname: "host", HostIp: " 10.0.0.1 ", GlobalLabels: map[string]string{"env": "prod", "zone": "a"}, EngineName: "new", AgentVersion: "v2", OS: "linux", Offset: 123, RemoteAddr: "client"}
		data, err := json.Marshal(payload)
		require.NoError(t, err)
		var buf bytes.Buffer
		if i == 1 {
			zw := gzip.NewWriter(&buf)
			_, err = zw.Write(data)
			require.NoError(t, err)
			require.NoError(t, zw.Close())
		} else {
			buf.Write(data)
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/heartbeat"+query, &buf)
		if i == 1 {
			c.Request.Header.Set("Content-Encoding", "gzip")
		}
		got, err := HandleHeartbeat(c, rt.Ctx, "default", meta, ident, cache)
		require.NoError(t, err)
		require.Equal(t, int64(123), got.Offset)
		var stored models.Target
		require.NoError(t, rt.Ctx.DB.First(&stored, cached.Id).Error)
		require.Equal(t, "10.0.0.1", stored.HostIp)
		require.Equal(t, []string{"env=prod", "zone=a"}, stored.HostTags)
		require.Equal(t, "keep=yes ", stored.Tags)
		require.Equal(t, "new", stored.EngineName)
		require.Equal(t, "v2", stored.AgentVersion)
		require.Equal(t, "linux", stored.OS)
		updated, ok := cache.Get("host")
		require.True(t, ok)
		require.Equal(t, stored.HostTags, updated.HostTags)
		require.Equal(t, stored.UpdateAt, updated.UpdateAt)
	}
	var groups []models.TargetBusiGroup
	require.NoError(t, rt.Ctx.DB.Find(&groups).Error)
	require.Len(t, groups, 1)
	require.Equal(t, int64(6), groups[0].GroupId)
}
func TestR8bHeartbeatRejectsMalformedRequests(t *testing.T) {
	for _, tc := range []struct{ body, encoding, want string }{{"bad", "gzip", "invalid header"}, {"{", "", "unexpected end"}, {"{}", "", "hostname is required"}} {
		t.Run(tc.want, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/heartbeat", bytes.NewBufferString(tc.body))
			c.Request.Header.Set("Content-Encoding", tc.encoding)
			_, err := HandleHeartbeat(c, nil, "", nil, nil, nil)
			require.Error(t, err)
			if tc.encoding == "gzip" {
				require.Equal(t, 400, w.Code)
			} else {
				require.Contains(t, err.Error(), tc.want)
			}
		})
	}
}
