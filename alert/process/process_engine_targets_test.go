package process

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/prometheus/client_golang/prometheus"
)

// TestRecoverAlertCurEventFromDbOwnTargetsOnly covers host rule events on an
// edge whose TargetCache holds only its own engine's targets: an event of an
// ident missing from that cache belongs to another engine and must not be
// loaded into this edge's fires, while an edge with the full cache keeps
// loading events of unknown idents as before. The fake center returns the same
// target list either way, standing in for the engine-filtered list.
func TestRecoverAlertCurEventFromDbOwnTargetsOnly(t *testing.T) {
	events := []*models.AlertCurEvent{
		{Hash: "own", RuleId: 1, Cate: models.HOST, TargetIdent: "host-own"},
		{Hash: "foreign", RuleId: 1, Cate: models.HOST, TargetIdent: "host-foreign"},
	}
	cached := []*models.Target{{Ident: "host-own", EngineName: "edge-a", UpdateAt: 1}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var dat interface{}
		switch r.URL.Path {
		case "/v1/n9e/statistic":
			dat = &models.Statistics{Total: 1, LastUpdated: 1}
		case "/v1/n9e/targets":
			dat = cached
		case "/v1/n9e/alert-cur-events-get-by-rid":
			dat = events
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"dat": dat, "err": ""})
	}))
	defer srv.Close()

	edgeCtx := &ctx.Context{CenterApi: conf.CenterApi{Addrs: []string{srv.URL}}}
	stats := &memsto.Stats{
		GaugeCronDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_cron_duration"}, []string{"name"}),
		GaugeSyncNumber:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "test_sync_number"}, []string{"name"}),
	}
	ruleCache := &memsto.AlertRuleCacheType{}
	ruleCache.Set(map[int64]*models.AlertRule{1: {Id: 1}}, 1, 1)

	for _, tt := range []struct {
		name       string
		engineName string
		want       []string
	}{
		{"own targets only", "edge-a", []string{"own"}},
		{"all targets", "", []string{"own", "foreign"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tc := memsto.NewTargetCacheOfEngine(edgeCtx, stats, nil, tt.engineName)
			p := &Processor{
				rule:           &models.AlertRule{Id: 1},
				EngineName:     "edge-a",
				TargetCache:    tc,
				BusiGroupCache: &memsto.BusiGroupCacheType{},
				alertRuleCache: ruleCache,
				ctx:            edgeCtx,
			}

			p.RecoverAlertCurEventFromDb()

			fires := p.fires.GetAll()
			if len(fires) != len(tt.want) {
				t.Fatalf("fires = %d events, want %v", len(fires), tt.want)
			}
			for _, hash := range tt.want {
				if _, has := p.fires.Get(hash); !has {
					t.Fatalf("event %s not loaded", hash)
				}
			}
		})
	}
}
