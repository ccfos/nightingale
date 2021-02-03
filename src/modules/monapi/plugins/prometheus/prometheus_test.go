package prometheus

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/prober/manager"
	"github.com/influxdata/telegraf"
)

const sampleTextFormat = `# HELP test_metric An untyped metric with a timestamp
# TYPE test_metric untyped
test_metric{label="value"} 1.0 1490802350000
# HELP helo_stats_test_timer helo_stats_test_timer summary
# TYPE helo_stats_test_timer summary
helo_stats_test_timer{region="bj",zone="test_1",quantile="0.5"} 0.501462767
helo_stats_test_timer{region="bj",zone="test_1",quantile="0.75"} 0.751876572
helo_stats_test_timer{region="bj",zone="test_1",quantile="0.95"} 0.978413628
helo_stats_test_timer{region="bj",zone="test_1",quantile="0.99"} 0.989530661
helo_stats_test_timer{region="bj",zone="test_1",quantile="0.999"} 0.989530661
helo_stats_test_timer_sum{region="bj",zone="test_1"} 39.169514066999994
helo_stats_test_timer_count{region="bj",zone="test_1"} 74
# HELP helo_stats_test_histogram helo_stats_test_histogram histogram
# TYPE helo_stats_test_histogram histogram
helo_stats_test_histogram_bucket{region="bj",zone="test_1",le="0"} 0
helo_stats_test_histogram_bucket{region="bj",zone="test_1",le="0.05"} 0
helo_stats_test_histogram_bucket{region="bj",zone="test_1",le="0.1"} 2
helo_stats_test_histogram_bucket{region="bj",zone="test_1",le="0.25"} 13
helo_stats_test_histogram_bucket{region="bj",zone="test_1",le="0.5"} 24
helo_stats_test_histogram_bucket{region="bj",zone="test_1",le="1"} 56
helo_stats_test_histogram_bucket{region="bj",zone="test_1",le="3"} 56
helo_stats_test_histogram_bucket{region="bj",zone="test_1",le="6"} 56
helo_stats_test_histogram_bucket{region="bj",zone="test_1",le="+Inf"} 56
helo_stats_test_histogram_sum{region="bj",zone="test_1"} 40.45
helo_stats_test_histogram_count{region="bj",zone="test_1"} 56
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 15 1490802350000
# HELP test_guage guage
# TYPE test_guage gauge
test_guauge{label="1"} 1.1
test_guauge{label="2"} 1.2
test_guauge{label="3"} 1.3
`

func TestCollect(t *testing.T) {
	http.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, sampleTextFormat) })
	server := &http.Server{Addr: ":18080"}
	go func() {
		server.ListenAndServe()
	}()
	defer server.Shutdown(context.Background())

	time.Sleep(time.Millisecond * 100)

	PluginTest(t, &PrometheusRule{
		URLs: []string{"http://localhost:18080/metrics"},
	})
}

type telegrafPlugin interface {
	TelegrafInput() (telegraf.Input, error)
}

func PluginTest(t *testing.T, plugin telegrafPlugin) telegraf.Input {
	input, err := plugin.TelegrafInput()
	if err != nil {
		t.Error(err)
	}

	PluginInputTest(t, input)

	return input
}

func PluginInputTest(t *testing.T, input telegraf.Input) {
	metrics := []*dataobj.MetricValue{}

	acc, err := manager.NewAccumulator(manager.AccumulatorOptions{Name: "plugin-test", Metrics: &metrics})
	if err != nil {
		t.Error(err)
	}

	if err = input.Gather(acc); err != nil {
		t.Error(err)
	}

	for k, v := range metrics {
		t.Logf("%d %s %s %f", k, v.CounterType, v.PK(), v.Value)
	}
}
