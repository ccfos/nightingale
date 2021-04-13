package manager

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/prober/config"
	"github.com/didi/nightingale/v4/src/modules/server/collector"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs/prometheus"
)

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

func TestManager(t *testing.T) {
	collector.CollectorRegister(&fakeCollector{BaseCollector: collector.NewBaseCollector(
		"fake",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &fakeRule{} },
	)})

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprintf(w, sampleTextFormat) }))
	defer s.Close()

	b, err := json.Marshal(fakeRule{URLs: []string{s.URL}})
	if err != nil {
		t.Fatal(err)
	}

	rule, err := newCollectRule(&models.CollectRule{
		Id:          1,
		Nid:         2,
		Step:        3,
		Timeout:     4,
		CollectType: "fake",
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

type fakeCollector struct {
	*collector.BaseCollector
}

type fakeRule struct {
	URLs            []string `label:"URLs" json:"urls,required" description:"An array of urls to scrape metrics from" example:"http://my-service-exporter:8080/metrics"`
	ResponseTimeout int      `label:"RESP Timeout" json:"response_timeout" default:"3" description:"Specify timeout duration for slower prometheus clients"`
}

func (p *fakeRule) Validate() error {
	if len(p.URLs) == 0 || p.URLs[0] == "" {
		return fmt.Errorf(" prometheus.rule unable to get urls")
	}
	return nil
}

func (p *fakeRule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	input := &prometheus.Prometheus{
		URLs:          p.URLs,
		URLTag:        "target",
		MetricVersion: 2,
	}

	if err := setValue(&input.ResponseTimeout.Duration,
		time.Second*time.Duration(p.ResponseTimeout)); err != nil {
		return nil, err
	}
	return input, nil
}

func setValue(in interface{}, value interface{}) error {
	rv := reflect.Indirect(reflect.ValueOf(in))

	if !rv.IsValid() || !rv.CanSet() {
		return fmt.Errorf("invalid argument IsValid %v CanSet %v", rv.IsValid(), rv.CanSet())
	}
	rv.Set(reflect.Indirect(reflect.ValueOf(value)))
	return nil
}
