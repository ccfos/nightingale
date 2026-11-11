package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/n9e/targets?from_cache=1", nil)

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

// TestTargetGetsByServiceWithoutFromCacheReadsDB covers edges that predate
// from_cache=1: they store the stat from /v1/n9e/statistic as is, so they must
// keep getting the list from the DB rather than from a possibly older cache.
func TestTargetGetsByServiceWithoutFromCacheReadsDB(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Target{}, &models.TargetBusiGroup{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&models.Target{Ident: "host-db"}).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}

	tc := &memsto.TargetCacheType{}
	tc.Set(map[string]*models.Target{"host-cache": {Ident: "host-cache"}}, 1, 1)
	rt := &Router{TargetCache: tc, Ctx: &ctx.Context{DB: db, IsCenter: true}}

	for _, tt := range []struct{ url, want string }{
		{"/v1/n9e/targets", "host-db"},
		{"/v1/n9e/targets?from_cache=1", "host-cache"},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, tt.url, nil)

		rt.targetGetsByService(c)

		var resp struct {
			Dat []*models.Target `json:"dat"`
			Err string           `json:"err"`
		}
		decodeServiceResp(t, w, &resp)
		if resp.Err != "" || len(resp.Dat) != 1 || resp.Dat[0].Ident != tt.want {
			t.Fatalf("%s: err = %q, targets = %+v, want %s", tt.url, resp.Err, resp.Dat, tt.want)
		}
	}
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

// TestTargetServiceEndpointsOfEngine covers engine_name on /v1/n9e/targets and
// /v1/n9e/statistic?name=target, sent by edges that sync only their own
// targets: both answer for that engine's targets only, from the cache with
// from_cache=1 and from the DB otherwise, and agree with each other.
func TestTargetServiceEndpointsOfEngine(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.Target{}, &models.TargetBusiGroup{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, target := range []*models.Target{
		{Ident: "db-a", EngineName: "edge-a", UpdateAt: 10},
		{Ident: "db-b", EngineName: "edge-b", UpdateAt: 20},
	} {
		if err := db.Create(target).Error; err != nil {
			t.Fatalf("create target: %v", err)
		}
	}
	if err := db.Create(&models.TargetBusiGroup{TargetIdent: "db-a", GroupId: 7}).Error; err != nil {
		t.Fatalf("create target busi group: %v", err)
	}

	tc := &memsto.TargetCacheType{}
	tc.Set(map[string]*models.Target{
		"cache-a": {Ident: "cache-a", EngineName: "edge-a", UpdateAt: 100},
		"cache-b": {Ident: "cache-b", EngineName: "edge-b", UpdateAt: 200},
	}, 2, 200)
	dbCtx := &ctx.Context{DB: db, IsCenter: true}

	for _, tt := range []struct {
		name      string
		rt        *Router
		targetURL string
		want      string
		wantStat  models.Statistics
	}{
		{"cache", &Router{TargetCache: tc}, "/v1/n9e/targets?from_cache=1&engine_name=edge-a", "cache-a", models.Statistics{Total: 1, LastUpdated: 100}},
		{"db", &Router{Ctx: dbCtx}, "/v1/n9e/targets?engine_name=edge-a", "db-a", models.Statistics{Total: 1, LastUpdated: 10}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, tt.targetURL, nil)
			tt.rt.targetGetsByService(c)

			var lst struct {
				Dat []*models.Target `json:"dat"`
				Err string           `json:"err"`
			}
			decodeServiceResp(t, w, &lst)
			if lst.Err != "" || len(lst.Dat) != 1 || lst.Dat[0].Ident != tt.want {
				t.Fatalf("targets: err = %q, targets = %+v, want %s", lst.Err, lst.Dat, tt.want)
			}
			if tt.want == "db-a" && (len(lst.Dat[0].GroupIds) != 1 || lst.Dat[0].GroupIds[0] != 7) {
				t.Fatalf("db-a group ids = %v, want [7]", lst.Dat[0].GroupIds)
			}

			w = httptest.NewRecorder()
			c, _ = gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/v1/n9e/statistic?name=target&engine_name=edge-a", nil)
			tt.rt.statistic(c)

			var stat struct {
				Dat *models.Statistics `json:"dat"`
				Err string             `json:"err"`
			}
			decodeServiceResp(t, w, &stat)
			if stat.Err != "" || stat.Dat == nil || *stat.Dat != tt.wantStat {
				t.Fatalf("statistic: err = %q, stat = %+v, want %+v", stat.Err, stat.Dat, tt.wantStat)
			}
		})
	}

	// An engine without targets: max(update_at) is NULL in the DB.
	stat, err := models.TargetStatisticsOfEngine(dbCtx, "none")
	if err != nil || stat.Total != 0 || stat.LastUpdated != 0 {
		t.Fatalf("empty engine stat = %+v, err %v, want {0 0}", stat, err)
	}
}
