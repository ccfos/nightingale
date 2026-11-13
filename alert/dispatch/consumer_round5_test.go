package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/alert/astats"
	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	promsdk "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/glebarez/sqlite"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type recoveryPromAPI struct {
	promsdk.API
	value   model.Value
	err     error
	queries []string
}

func (a *recoveryPromAPI) Query(_ context.Context, query string, _ time.Time) (model.Value, promsdk.Warnings, error) {
	a.queries = append(a.queries, query)
	return a.value, nil, a.err
}

func consumerStats() *astats.Stats {
	return &astats.Stats{
		CounterAlertsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_consumed_events"},
			[]string{"cluster", "type", "group"}),
		CounterRuleEvalErrorTotal: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_persist_errors"},
			[]string{"datasource", "stage", "group", "rule"}),
	}
}

func TestQueryRecoveryVal_StoresLatestValueInAnnotations(t *testing.T) {
	api := &recoveryPromAPI{value: model.Vector{{Value: 42}}}
	consumer := &Consumer{
		ctx:         ctx.NewContext(context.Background(), nil, true),
		promClients: &prom.PromClientMap{ReaderClients: map[int64]promsdk.API{7: api}},
	}
	event := &models.AlertCurEvent{
		IsRecovered: true, DatasourceId: 7,
		AnnotationsJSON: map[string]string{"recovery_promql": "  cpu_usage  ", "owner": "ops"},
	}

	consumer.queryRecoveryVal(event)

	require.Equal(t, []string{"cpu_usage"}, api.queries)
	require.Equal(t, "42", event.AnnotationsJSON["recovery_value"])
	require.JSONEq(t, `{"recovery_promql":"  cpu_usage  ","owner":"ops","recovery_value":"42"}`, event.Annotations)
}

func TestQueryRecoveryVal_RecordsQueryFailureOrEmptyResult(t *testing.T) {
	for _, tc := range []struct {
		name    string
		failure error
		want    string
	}{
		{name: "query error", failure: errors.New("unavailable"), want: "promql:up error:unavailable"},
		{name: "empty vector", want: "promql:up error:result is empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &recoveryPromAPI{value: model.Vector{}, err: tc.failure}
			consumer := &Consumer{
				ctx:         ctx.NewContext(context.Background(), nil, true),
				promClients: &prom.PromClientMap{ReaderClients: map[int64]promsdk.API{7: api}},
			}
			event := &models.AlertCurEvent{
				IsRecovered: true, DatasourceId: 7, AnnotationsJSON: map[string]string{"recovery_promql": "up"},
			}

			consumer.queryRecoveryVal(event)

			require.Equal(t, []string{"up"}, api.queries)
			require.Equal(t, tc.want, event.AnnotationsJSON["recovery_promql_error"])
			var annotations map[string]string
			require.NoError(t, json.Unmarshal([]byte(event.Annotations), &annotations))
			require.Equal(t, event.AnnotationsJSON, annotations)
			require.NotContains(t, annotations, "recovery_value")
		})
	}
}

func TestQueryRecoveryVal_SkipsInapplicableQueries(t *testing.T) {
	for _, tc := range []struct {
		name        string
		recovered   bool
		annotations map[string]string
		datasource  int64
	}{
		{name: "firing", annotations: map[string]string{"recovery_promql": "up"}, datasource: 7},
		{name: "no query", recovered: true, datasource: 7},
		{name: "blank query", recovered: true, annotations: map[string]string{"recovery_promql": " "}, datasource: 7},
		{name: "missing datasource", recovered: true, annotations: map[string]string{"recovery_promql": "up"}, datasource: 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &recoveryPromAPI{value: model.Vector{{Value: 42}}}
			consumer := &Consumer{promClients: &prom.PromClientMap{ReaderClients: map[int64]promsdk.API{7: api}}}
			event := &models.AlertCurEvent{
				IsRecovered: tc.recovered, DatasourceId: tc.datasource, AnnotationsJSON: tc.annotations, Annotations: "unchanged",
			}

			consumer.queryRecoveryVal(event)

			require.Empty(t, api.queries)
			require.Equal(t, "unchanged", event.Annotations)
			require.Equal(t, tc.annotations, event.AnnotationsJSON)
		})
	}
}

func TestConsumerPersist_EdgeUsesCenterAssignedEventID(t *testing.T) {
	var received models.AlertCurEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/n9e/event-persist" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"dat":123,"err":""}`))
	}))
	t.Cleanup(server.Close)
	c := ctx.NewContext(context.Background(), nil, false)
	c.CenterApi = conf.CenterApi{Addrs: []string{server.URL}, Timeout: 9000}
	consumer := NewConsumer(aconf.Alerting{}, c, &Dispatch{Astats: consumerStats()}, nil, nil)
	event := &models.AlertCurEvent{Hash: "event-edge", Tags: "env=prod", RuleId: 7}

	consumer.persist(event)

	require.Equal(t, int64(123), event.Id)
	require.Equal(t, "event-edge", received.Hash)
	require.Equal(t, []string{"env=prod"}, received.TagsJSON)
	require.EqualValues(t, 9000, c.CenterApi.Timeout)
	require.EqualValues(t, persistTimeoutMs, consumer.persistCtx.CenterApi.Timeout)
}

func TestConsumerPersist_CenterRecordsFiringAndRecovery(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&models.AlertCurEvent{}, &models.AlertHisEvent{}))
	consumer := &Consumer{ctx: ctx.NewContext(context.Background(), db, true), dispatch: &Dispatch{Astats: consumerStats()}}
	event := &models.AlertCurEvent{Hash: "cpu-1", RuleId: 7, TriggerValue: "91"}

	consumer.persist(event)

	require.Positive(t, event.Id)
	var current models.AlertCurEvent
	require.NoError(t, db.First(&current, "hash = ?", event.Hash).Error)
	require.Equal(t, event.Id, current.Id)
	require.Equal(t, "91", current.TriggerValue)
	event.IsRecovered = true

	consumer.persist(event)

	var count int64
	require.NoError(t, db.Model(&models.AlertCurEvent{}).Count(&count).Error)
	require.Zero(t, count)
	var history []models.AlertHisEvent
	require.NoError(t, db.Order("id").Find(&history).Error)
	require.Len(t, history, 2)
	require.Zero(t, history[0].IsRecovered)
	require.Equal(t, 1, history[1].IsRecovered)
	require.Equal(t, history[1].Id, event.Id)
}

func TestConsumerPersist_CountsDatabaseFailure(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	stats := consumerStats()
	consumer := &Consumer{ctx: ctx.NewContext(context.Background(), db, true), dispatch: &Dispatch{Astats: stats}}
	event := &models.AlertCurEvent{RuleId: 7, DatasourceId: 8, GroupName: "ops"}

	consumer.persist(event)

	require.Zero(t, event.Id)
	require.Equal(t, 1.0, testutil.ToFloat64(stats.CounterRuleEvalErrorTotal.WithLabelValues("8", "persist_event", "ops", "7")))
}

func TestConsumeOne_ParsesMutedEventAndRunsCleanup(t *testing.T) {
	d := round2Dispatch(t)
	d.Astats = consumerStats()
	d.alertSubscribeCache = &memsto.AlertSubscribeCacheType{}
	consumer := &Consumer{ctx: d.ctx, dispatch: d, alertMuteCache: &memsto.AlertMuteCacheType{}}
	old := NotifyMutedEventHook
	t.Cleanup(func() { NotifyMutedEventHook = old })
	var cleaned *models.AlertCurEvent
	NotifyMutedEventHook = func(event *models.AlertCurEvent) { cleaned = event }
	event := &models.AlertCurEvent{
		Id: 12, Status: 1, NotifyMuted: 1, MuteId: 7, Cluster: "prom", GroupName: "ops",
		RuleName: "CPU on {{$labels.host}}", RuleNote: "value {{$value}} / {{$annotations.owner}}",
		Annotations: `{"owner":"ops"}`, TagsMap: map[string]string{"host": "node-1"}, TriggerValue: "91",
	}

	consumer.consumeOne(event)

	require.Same(t, event, cleaned)
	require.Equal(t, "CPU on node-1", event.RuleName)
	require.Equal(t, "value 91 / ops", event.RuleNote)
	require.Equal(t, 1.0, testutil.ToFloat64(d.Astats.CounterAlertsTotal.WithLabelValues("prom", "alert", "ops")))
	require.Equal(t, 1, sender.NotifyRecordQueue.Len())
	record := sender.NotifyRecordQueue.PopBack().(*models.NotificationRecord)
	require.Equal(t, int64(12), record.EventId)
	require.Equal(t, models.NotiStatusMuted, record.Status)
	require.Contains(t, record.Details, "id=7")
}
