package memsto

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/prometheus/client_golang/prometheus"
)

// TestSyncTargetsEdgeStoresListStat covers an edge whose stat and list
// requests are answered by two centers at different cache states: the stat
// comes from a center that has already reloaded, the list from one that has
// not. The edge must store the stat of the list it received, so that the next
// poll sees a change and reloads instead of latching the stale list.
func TestSyncTargetsEdgeStoresListStat(t *testing.T) {
	stale := []*models.Target{{Ident: "host-1", UpdateAt: 100}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var dat interface{}
		switch r.URL.Path {
		case "/v1/n9e/statistic":
			dat = &models.Statistics{Total: 2, LastUpdated: 300}
		case "/v1/n9e/targets":
			if r.URL.Query().Get("from_cache") != "1" {
				t.Errorf("targets request without from_cache=1: %s", r.URL.RequestURI())
			}
			dat = stale
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"dat": dat, "err": ""})
	}))
	defer srv.Close()

	tc := &TargetCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             &ctx.Context{IsCenter: false, CenterApi: conf.CenterApi{Addrs: []string{srv.URL}}},
		stats: &Stats{
			GaugeCronDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_cron_duration"}, []string{"name"}),
			GaugeSyncNumber:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_sync_number"}, []string{"name"}),
		},
		targets:      make(map[string]*models.Target),
		targetsIndex: make(map[string][]string),
	}

	if err := tc.syncTargets(); err != nil {
		t.Fatalf("syncTargets: %v", err)
	}
	if total, last := tc.Stat(); total != 1 || last != 100 {
		t.Fatalf("stored stat = {%d %d}, want {1 100} derived from the received list", total, last)
	}
	if !tc.StatChanged(2, 300) {
		t.Fatal("StatChanged(2, 300) = false: the stale list would never be reloaded")
	}
}

func TestTargetsStat(t *testing.T) {
	if total, last := targetsStat(nil); total != 0 || last != 0 {
		t.Fatalf("empty = {%d %d}, want {0 0}", total, last)
	}
	total, last := targetsStat([]*models.Target{{UpdateAt: 5}, {UpdateAt: 9}, {UpdateAt: 7}})
	if total != 3 || last != 9 {
		t.Fatalf("stat = {%d %d}, want {3 9}", total, last)
	}
}

// TestUpdateTargetConcurrentWithSnapshot runs the locked writer against
// Snapshot; with -race this fails if UpdateTarget stops taking the write lock.
func TestUpdateTargetConcurrentWithSnapshot(t *testing.T) {
	tc := &TargetCacheType{}
	tc.Set(map[string]*models.Target{"host-1": {Ident: "host-1"}}, 1, 1)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 1000; i++ {
			tc.UpdateTarget("host-1", func(t *models.Target) {
				t.OS = "linux"
				t.HostTags = []string{"a=b"}
			})
		}
	}()
	for i := 0; i < 1000; i++ {
		lst, _, _ := tc.Snapshot()
		_ = lst[0].HostTags
	}
	<-done

	if cached, _ := tc.Get("host-1"); cached.OS != "linux" {
		t.Fatalf("OS = %q, want linux", cached.OS)
	}
	if tc.UpdateTarget("missing", func(*models.Target) { t.Fatal("fn called for missing target") }) {
		t.Fatal("UpdateTarget(missing) = true")
	}
}

// TestSyncTargetsOfEngine covers an edge with SyncOwnTargetsOnly: both the
// stat and the list requests carry engine_name, and the stat derived from the
// list matches the center's stat of the same engine, so an unchanged engine
// does not reload every poll. A center that ignores engine_name returns all
// targets, which the edge then stores with their own stat.
func TestSyncTargetsOfEngine(t *testing.T) {
	all := []*models.Target{
		{Ident: "host-a", EngineName: "edge-a", UpdateAt: 100},
		{Ident: "host-b", EngineName: "edge-b", UpdateAt: 300},
	}
	for _, tt := range []struct {
		name        string
		honorEngine bool
		wantIdents  []string
		wantStat    [2]int64
	}{
		{"new center", true, []string{"host-a"}, [2]int64{1, 100}},
		{"old center", false, []string{"host-a", "host-b"}, [2]int64{2, 300}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var listCalls int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				engine := r.URL.Query().Get("engine_name")
				if engine != "edge-a" {
					t.Errorf("request without engine_name=edge-a: %s", r.URL.RequestURI())
				}
				lst := all
				if tt.honorEngine {
					lst = nil
					for _, target := range all {
						if target.EngineName == engine {
							lst = append(lst, target)
						}
					}
				}
				var dat interface{}
				switch r.URL.Path {
				case "/v1/n9e/statistic":
					total, last := targetsStat(lst)
					dat = &models.Statistics{Total: total, LastUpdated: last}
				case "/v1/n9e/targets":
					listCalls++
					dat = lst
				default:
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]interface{}{"dat": dat, "err": ""})
			}))
			defer srv.Close()

			tc := &TargetCacheType{
				statTotal:       -1,
				statLastUpdated: -1,
				engineName:      "edge-a",
				ctx:             &ctx.Context{IsCenter: false, CenterApi: conf.CenterApi{Addrs: []string{srv.URL}}},
				stats: &Stats{
					GaugeCronDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_cron_duration"}, []string{"name"}),
					GaugeSyncNumber:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_sync_number"}, []string{"name"}),
				},
				targets:      make(map[string]*models.Target),
				targetsIndex: make(map[string][]string),
			}

			for i := 0; i < 2; i++ {
				if err := tc.syncTargets(); err != nil {
					t.Fatalf("syncTargets: %v", err)
				}
			}
			if listCalls != 1 {
				t.Fatalf("list requested %d times, want 1: the unchanged stat must not reload", listCalls)
			}
			if total, last := tc.Stat(); total != tt.wantStat[0] || last != tt.wantStat[1] {
				t.Fatalf("stat = {%d %d}, want %v", total, last, tt.wantStat)
			}
			for _, ident := range tt.wantIdents {
				if _, has := tc.Get(ident); !has {
					t.Fatalf("%s not cached", ident)
				}
			}
			if got := len(tc.GetAll()); got != len(tt.wantIdents) {
				t.Fatalf("cached %d targets, want %d", got, len(tt.wantIdents))
			}
		})
	}
}

// TestTargetCacheOfEngine covers the center side: SnapshotOfEngine and
// StatOfEngine pick the same targets, and their stat is what targetsStat
// derives from the snapshot, so an edge storing that stat stays in sync. A
// target moving to another engine changes the stat of both engines.
func TestTargetCacheOfEngine(t *testing.T) {
	tc := &TargetCacheType{}
	tc.Set(map[string]*models.Target{
		"host-1": {Ident: "host-1", EngineName: "edge-a", UpdateAt: 100},
		"host-2": {Ident: "host-2", EngineName: "edge-a", UpdateAt: 200},
		"host-3": {Ident: "host-3", EngineName: "edge-b", UpdateAt: 300},
	}, 3, 300)

	lst, total, last := tc.SnapshotOfEngine("edge-a")
	if len(lst) != 2 || total != 2 || last != 200 {
		t.Fatalf("snapshot = %d targets, stat {%d %d}, want 2 targets {2 200}", len(lst), total, last)
	}
	if st, sl := tc.StatOfEngine("edge-a"); st != total || sl != last {
		t.Fatalf("StatOfEngine = {%d %d}, want {%d %d}", st, sl, total, last)
	}
	if lst, total, _ := tc.SnapshotOfEngine("none"); lst == nil || len(lst) != 0 || total != 0 {
		t.Fatalf("unknown engine snapshot = %v, total %d, want empty non-nil list", lst, total)
	}

	tc.UpdateTarget("host-2", func(t *models.Target) {
		t.EngineName = "edge-b"
		t.UpdateAt = 400
	})
	if st, sl := tc.StatOfEngine("edge-a"); st != 1 || sl != 100 {
		t.Fatalf("edge-a after move = {%d %d}, want {1 100}", st, sl)
	}
	if st, sl := tc.StatOfEngine("edge-b"); st != 2 || sl != 400 {
		t.Fatalf("edge-b after move = {%d %d}, want {2 400}", st, sl)
	}
}

// TestEngineStatsFollowUpdateTarget checks that the per-engine stat kept by
// UpdateTarget always equals a scan of the cached targets, including moves of
// the target holding an engine's max update_at and an engine left empty.
func TestEngineStatsFollowUpdateTarget(t *testing.T) {
	tc := &TargetCacheType{}
	tc.Set(map[string]*models.Target{
		"host-1": {Ident: "host-1", EngineName: "edge-a", UpdateAt: 100},
		"host-2": {Ident: "host-2", EngineName: "edge-a", UpdateAt: 200},
		"host-3": {Ident: "host-3", EngineName: "edge-b", UpdateAt: 300},
		"host-4": {Ident: "host-4", EngineName: "", UpdateAt: 50},
	}, 4, 300)

	steps := []struct {
		ident    string
		engine   string
		updateAt int64
	}{
		{"host-1", "edge-a", 250}, // raises edge-a's max
		{"host-1", "edge-b", 260}, // edge-a's max holder leaves
		{"host-2", "edge-a", 150}, // update_at goes back
		{"host-2", "edge-a", 150}, // no change
		{"host-2", "edge-c", 500}, // edge-a left empty
		{"host-3", "edge-b", 300}, // no change
		{"host-4", "edge-a", 50},  // "" left empty, edge-a refilled
		{"host-3", "edge-c", 100}, // non-max holder leaves edge-b, joins below max
	}
	engines := []string{"", "edge-a", "edge-b", "edge-c"}
	for i, step := range steps {
		tc.UpdateTarget(step.ident, func(t *models.Target) {
			t.EngineName = step.engine
			t.UpdateAt = step.updateAt
		})
		for _, engine := range engines {
			_, wantTotal, wantLast := tc.SnapshotOfEngine(engine)
			if total, last := tc.StatOfEngine(engine); total != wantTotal || last != wantLast {
				t.Fatalf("step %d: StatOfEngine(%q) = {%d %d}, want {%d %d}", i, engine, total, last, wantTotal, wantLast)
			}
		}
	}
}
