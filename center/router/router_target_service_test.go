package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
)

// TestTargetServiceEndpointsFromCache covers /v1/n9e/targets and
// /v1/n9e/statistic?name=target being answered from the center's TargetCache:
// edges poll both, so neither may touch the DB, and the stat must be the one
// the served targets were loaded at. Router has no Ctx here, so any DB access
// would panic.
func TestTargetServiceEndpointsFromCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tc := &memsto.TargetCacheType{}
	tc.Set(map[string]*models.Target{
		"host-1": {Ident: "host-1", HostIp: "10.0.0.1", GroupIds: []int64{1}, TagsJSON: []string{"env=prod"}},
		"host-2": {Ident: "host-2", HostIp: "10.0.0.2"},
	}, 2, 1700000000)
	rt := &Router{TargetCache: tc}

	t.Run("statistic", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/n9e/statistic?name=target", nil)

		rt.statistic(c)

		var resp struct {
			Dat *models.Statistics `json:"dat"`
			Err string             `json:"err"`
		}
		decodeServiceResp(t, w, &resp)
		if resp.Err != "" || resp.Dat == nil {
			t.Fatalf("err = %q, dat = %v", resp.Err, resp.Dat)
		}
		if resp.Dat.Total != 2 || resp.Dat.LastUpdated != 1700000000 {
			t.Fatalf("stat = %+v, want {2 1700000000}", *resp.Dat)
		}
	})

	t.Run("targets", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/n9e/targets", nil)

		rt.targetGetsByService(c)

		var resp struct {
			Dat []*models.Target `json:"dat"`
			Err string           `json:"err"`
		}
		decodeServiceResp(t, w, &resp)
		if resp.Err != "" {
			t.Fatalf("err = %q", resp.Err)
		}
		sort.Slice(resp.Dat, func(i, j int) bool { return resp.Dat[i].Ident < resp.Dat[j].Ident })
		if len(resp.Dat) != 2 || resp.Dat[0].Ident != "host-1" || resp.Dat[1].Ident != "host-2" {
			t.Fatalf("targets = %+v, want host-1, host-2", resp.Dat)
		}
		if got := resp.Dat[0]; got.HostIp != "10.0.0.1" || len(got.GroupIds) != 1 || got.GroupIds[0] != 1 ||
			len(got.TagsJSON) != 1 || got.TagsJSON[0] != "env=prod" {
			t.Fatalf("host-1 = %+v, fields not carried over", got)
		}
	})
}

// TestTargetCacheSnapshotIsolated makes sure Snapshot hands out copies: the
// cache keeps mutating its own objects (BeatTime/meta refresh) while a caller
// encodes the snapshot, so they must not share the struct.
func TestTargetCacheSnapshotIsolated(t *testing.T) {
	tc := &memsto.TargetCacheType{}
	tc.Set(map[string]*models.Target{"host-1": {Ident: "host-1", BeatTime: 1}}, 1, 10)

	lst, total, lastUpdated := tc.Snapshot()
	if len(lst) != 1 || total != 1 || lastUpdated != 10 {
		t.Fatalf("snapshot = %d targets, stat {%d %d}", len(lst), total, lastUpdated)
	}
	lst[0].BeatTime = 99

	cached, _ := tc.Get("host-1")
	if cached.BeatTime != 1 {
		t.Fatalf("cached BeatTime = %d, snapshot write leaked into cache", cached.BeatTime)
	}
}

func decodeServiceResp(t *testing.T, w *httptest.ResponseRecorder, v interface{}) {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("unmarshal body %s: %v", w.Body.String(), err)
	}
}
