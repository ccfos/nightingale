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
