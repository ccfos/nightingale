package writer

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/tlsx"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
)

func run(writers *WritersType, wg *sync.WaitGroup) {
	defer wg.Done()

	for i := 0; i < 10; i += 1 {
		writers.PushSample("test", "test")
	}
}

func TestNewWriters(t *testing.T) {
	pushgwConfig := pconf.Pushgw{
		BusiGroupLabelKey:   "group_label",
		IdentMetrics:        []string{"metric1", "metric2"},
		IdentStatsThreshold: 10,
		IdentDropThreshold:  5,
		WriteConcurrency:    4,
		LabelRewrite:        true,
		ForceUseServerTS:    false,
		DebugSample: map[string]string{
			"debug_key1": "value1",
			"debug_key2": "value2",
		},
		DropSample: []map[string]string{
			{"drop_key1": "value1"},
			{"drop_key2": "value2"},
		},
		WriterOpt: pconf.WriterGlobalOpt{
			QueueMaxSize: 100,
			QueuePopSize: 100,
		},
		Writers: []pconf.WriterOptions{
			{
				Url:                   "https://example.com",
				BasicAuthUser:         "your_username",
				BasicAuthPass:         "your_password",
				Timeout:               5000 * int64(time.Second),
				DialTimeout:           3000 * int64(time.Second),
				TLSHandshakeTimeout:   2000 * int64(time.Second),
				ExpectContinueTimeout: 1000 * int64(time.Second),
				IdleConnTimeout:       6000 * int64(time.Second),
				KeepAlive:             3000 * int64(time.Second),
				MaxConnsPerHost:       10,
				MaxIdleConns:          5,
				MaxIdleConnsPerHost:   2,
				Headers:               []string{"Content-Type: application/json"},
				WriteRelabels:         nil,
				ClientConfig:          tlsx.ClientConfig{},
			},
		},
		MetricsMaxCount:     -1,
		MetricRateFreshTime: 200,
	}

	writers := NewWriters(pushgwConfig)
	fmt.Printf("writers: %v\n", writers)

	wg := &sync.WaitGroup{}
	for i := 0; i < 10; i += 1 {
		wg.Add(1)
		go run(writers, wg)
		time.Sleep(1 * time.Second)
	}
	wg.Wait()
}
