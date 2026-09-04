package dscache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/models"
)

type failingDatasource struct {
	identity string
	started  chan struct{}
	release  chan struct{}
	calls    *atomic.Int64
	once     sync.Once
}

func (d *failingDatasource) Init(map[string]interface{}) (datasource.Datasource, error) {
	return d, nil
}
func (d *failingDatasource) InitClient() error {
	d.calls.Add(1)
	if d.started != nil {
		d.once.Do(func() { close(d.started) })
	}
	if d.release != nil {
		<-d.release
	}
	return errors.New("fixture unavailable")
}
func (d *failingDatasource) Validate(context.Context) error { return nil }
func (d *failingDatasource) Equal(other datasource.Datasource) bool {
	value, ok := other.(*failingDatasource)
	return ok && value.identity == d.identity
}
func (d *failingDatasource) MakeLogQuery(context.Context, interface{}, []string, int64, int64) (interface{}, error) {
	return nil, nil
}
func (d *failingDatasource) MakeTSQuery(context.Context, interface{}, []string, int64, int64) (interface{}, error) {
	return nil, nil
}
func (d *failingDatasource) QueryData(context.Context, interface{}) ([]models.DataResp, error) {
	return nil, nil
}
func (d *failingDatasource) QueryLog(context.Context, interface{}) ([]interface{}, int64, error) {
	return nil, 0, nil
}
func (d *failingDatasource) QueryMapData(context.Context, interface{}) ([]map[string]string, error) {
	return nil, nil
}

func newTestCache() *Cache {
	return &Cache{
		datas:        make(map[string]map[int64]datasource.Datasource),
		initAttempts: make(map[string]map[int64]initAttempt),
		mutex:        new(sync.RWMutex),
	}
}

func TestPutCoalescesInflightAndBacksOffUnchangedFailure(t *testing.T) {
	cache := newTestCache()
	var calls atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	first := &failingDatasource{
		identity: "same",
		started:  started,
		release:  release,
		calls:    &calls,
	}

	done := make(chan struct{})
	go func() {
		cache.Put("fixture", 1, first)
		close(done)
	}()
	<-started
	cache.Put("fixture", 1, &failingDatasource{identity: "same", calls: &calls})
	close(release)
	<-done
	cache.Put("fixture", 1, &failingDatasource{identity: "same", calls: &calls})

	if got := calls.Load(); got != 1 {
		t.Fatalf("unchanged in-flight/backoff datasource should initialize once, got %d", got)
	}
}

func TestPutRetriesChangedConfigurationImmediately(t *testing.T) {
	cache := newTestCache()
	var calls atomic.Int64
	oldInterval := failedInitRetryInterval
	failedInitRetryInterval = time.Hour
	defer func() { failedInitRetryInterval = oldInterval }()

	cache.Put("fixture", 1, &failingDatasource{identity: "old", calls: &calls})
	cache.Put("fixture", 1, &failingDatasource{identity: "new", calls: &calls})

	if got := calls.Load(); got != 2 {
		t.Fatalf("changed datasource configuration should retry immediately, got %d calls", got)
	}
}

func TestFailedAttemptRemainsVisibleForDeleteReconciliation(t *testing.T) {
	cache := newTestCache()
	var calls atomic.Int64
	cache.Put("fixture", 7, &failingDatasource{identity: "removed", calls: &calls})

	ids := cache.GetAllIds()["fixture"]
	if len(ids) != 1 || ids[0] != 7 {
		t.Fatalf("failed attempt should remain visible to deletion reconciliation, got %v", ids)
	}

	cache.Delete("fixture", 7)
	if ids := cache.GetAllIds()["fixture"]; len(ids) != 0 {
		t.Fatalf("deleted failed attempt should not remain tracked, got %v", ids)
	}
}
