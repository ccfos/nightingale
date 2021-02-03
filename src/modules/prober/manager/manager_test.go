package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/monapi/plugins/prometheus"
	"github.com/didi/nightingale/src/modules/prober/config"
)

func TestManager(t *testing.T) {
	{
		http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, sampleTextFormat) })
		server := &http.Server{Addr: ":18080"}
		go func() {
			server.ListenAndServe()
		}()
		defer server.Shutdown(context.Background())

		time.Sleep(time.Millisecond * 100)
	}

	promRule := prometheus.PrometheusRule{
		URLs: []string{"http://localhost:18080/metrics"},
	}

	b, err := json.Marshal(promRule)
	if err != nil {
		t.Fatal(err)
	}

	rule, err := newCollectRule(&models.CollectRule{
		Id:          1,
		Nid:         2,
		Step:        3,
		Timeout:     4,
		CollectType: "prometheus",
		Name:        "prom-test",
		Region:      "default",
		Data:        json.RawMessage(b),
		Tags:        "a=1,b=2",
	})
	if err != nil {
		t.Fatal(err)
	}

	rule.reset()
	err = rule.input.Gather(rule.acc)
	if err != nil {
		t.Fatalf("gather %s", err)
	}

	metrics, err := rule.prepareMetrics(&config.PluginConfig{Mode: config.PluginModeAll})
	if err != nil {
		t.Fatalf("prepareMetrics %s", err)
	}

	for k, v := range metrics {
		t.Logf("%d %s %s %f", k, v.CounterType, v.PK(), v.Value)
	}
}

const sampleTextFormat = `
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 15 1490802350000
# HELP test_guage guage
# TYPE test_guage gauge
test_guauge{label="1"} 1.1
test_guauge{label="2"} 1.2
test_guauge{label="3"} 1.3
`
