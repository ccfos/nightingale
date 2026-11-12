package process

import (
	"testing"

	"github.com/ccfos/nightingale/v6/alert/queue"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func stateProcessor(t *testing.T, rule *models.AlertRule) *Processor {
	t.Helper()
	old := queue.EventQueue.PopBackBy(queue.EventQueue.Len())
	t.Cleanup(func() {
		queue.EventQueue.RemoveAll()
		for _, e := range old {
			queue.EventQueue.PushFront(e)
		}
	})
	return &Processor{
		rule:                   rule,
		fires:                  NewAlertCurEventMap(nil),
		pendings:               NewAlertCurEventMap(nil),
		pendingsUseByRecover:   NewAlertCurEventMap(nil),
		alertMuteCache:         &memsto.AlertMuteCacheType{},
		HandleFireEventHook:    func(*models.AlertCurEvent) {},
		HandleRecoverEventHook: func(*models.AlertCurEvent) {},
	}
}

func TestFireEvent_RepeatAndMutedSnapshotDecisions(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		step, max, number, muted int
		now                      int64
		shadow                   int
		shadowTime               int64
		wantQueue                bool
		wantNumber, wantShadow   int
	}{
		{name: "repeat disabled", number: 1, now: 200},
		{name: "interval pending", step: 1, number: 1, now: 159},
		{name: "interval inclusive", step: 1, number: 1, now: 160, wantQueue: true, wantNumber: 2},
		{name: "max reached", step: 1, max: 2, number: 2, now: 200},
		{name: "below max", step: 1, max: 3, number: 2, now: 200, wantQueue: true, wantNumber: 3},
		{
			name:       "first after unmute even without repeat",
			number:     0,
			now:        200,
			wantQueue:  true,
			wantNumber: 1,
		},
		{name: "muted repeat disabled", number: 1, muted: 1, now: 200},
		{name: "muted interval pending", step: 1, number: 1, muted: 1, now: 159},
		{
			name:       "muted snapshot",
			step:       1,
			number:     1,
			muted:      1,
			now:        160,
			wantQueue:  true,
			wantNumber: 1,
			wantShadow: 2,
		},
		{
			name:       "muted snapshot max",
			step:       1,
			max:        2,
			number:     1,
			muted:      1,
			shadow:     2,
			shadowTime: 120,
			now:        200,
		},
		{
			name:       "muted shadow interval pending",
			step:       1,
			number:     1,
			muted:      1,
			shadow:     2,
			shadowTime: 120,
			now:        179,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := stateProcessor(t, &models.AlertRule{Id: 1, NotifyRepeatStep: tc.step, NotifyMaxNumber: tc.max})
			previous := &models.AlertCurEvent{
				Hash:                 "h",
				FirstTriggerTime:     50,
				FirstEvalTime:        40,
				LastSentTime:         100,
				NotifyCurNumber:      tc.number,
				MutedPersistNumber:   tc.shadow,
				LastMutedPersistTime: tc.shadowTime,
			}
			p.fires.Set("h", previous)
			calls := 0
			p.HandleFireEventHook = func(*models.AlertCurEvent) { calls++ }
			next := &models.AlertCurEvent{Hash: "h", LastEvalTime: tc.now, NotifyMuted: tc.muted}

			p.fireEvent(next)

			require.Equal(t, 1, calls)
			require.Equal(t, int64(50), next.FirstTriggerTime)

			items := queue.EventQueue.PopBackBy(10)
			if tc.wantQueue {
				require.Len(t, items, 1)

				got := items[0].(*models.AlertCurEvent)

				require.Equal(t, tc.wantNumber, got.NotifyCurNumber)
				require.Equal(t, tc.wantShadow, got.MutedPersistNumber)

				// Muting advances snapshot counters while preserving real notification state.
				if tc.muted == 1 {
					require.Equal(t, int64(100), got.LastSentTime)
					require.Equal(t, tc.now, got.LastMutedPersistTime)
				} else {
					require.Equal(t, tc.now, got.LastSentTime)
				}
			} else {
				require.Empty(t, items)

				got, _ := p.fires.Get("h")

				require.Equal(t, tc.now, got.LastEvalTime)
			}
		})
	}
}

func TestFireEvent_FirstOccurrenceQueuesSnapshot(t *testing.T) {
	for _, muted := range []int{0, 1} {
		t.Run(map[int]string{0: "normal", 1: "notify muted"}[muted], func(t *testing.T) {
			p := stateProcessor(t, &models.AlertRule{Id: 1})
			e := &models.AlertCurEvent{Hash: "first", TriggerTime: 80, LastEvalTime: 100, NotifyMuted: muted}

			p.fireEvent(e)
			items := queue.EventQueue.PopBackBy(10)

			require.Len(t, items, 1)

			got := items[0].(*models.AlertCurEvent)

			require.Equal(t, int64(80), got.FirstTriggerTime)
			require.Equal(t, 1-muted, got.NotifyCurNumber)
			require.Equal(t, muted, got.MutedPersistNumber)
			require.NotSame(t, e, got)
		})
	}
}

func TestHandleEvent_PendingDurationAndSeverityInhibition(t *testing.T) {
	p := stateProcessor(t, &models.AlertRule{Id: 1, PromForDuration: 30})
	p.PromEvalInterval = 10
	// A twenty-second elapsed span plus the ten-second evaluation interval satisfies the duration.
	p.inhibit = true

	p.handleEvent([]*models.AlertCurEvent{
		nil,
		{Hash: "critical", Severity: 1, LastEvalTime: 100},
		{Hash: "warning", Severity: 2, LastEvalTime: 100},
	})

	require.Zero(t, queue.EventQueue.Len())

	pending, ok := p.pendings.Get("critical")

	require.True(t, ok)
	require.Equal(t, int64(100), pending.FirstEvalTime)

	// At the duration boundary, the critical event inhibits the warning.
	p.handleEvent([]*models.AlertCurEvent{
		{Hash: "critical", Severity: 1, LastEvalTime: 120},
		{Hash: "warning", Severity: 2, LastEvalTime: 120},
	})
	items := queue.EventQueue.PopBackBy(10)

	require.Len(t, items, 1)
	require.Equal(t, "critical", items[0].(*models.AlertCurEvent).Hash)
	require.ElementsMatch(t, []string{"critical", "warning"}, p.pendingsUseByRecover.Keys())
}

func TestHandleRecover_RecoversOnlyMissingEvents(t *testing.T) {
	p := stateProcessor(t, &models.AlertRule{Id: 1})
	for _, hash := range []string{"active", "gone"} {
		e := &models.AlertCurEvent{
			Hash:          hash,
			LastEvalTime:  100,
			RecoverConfig: models.RecoverConfig{JudgeType: models.Origin},
		}
		p.fires.Set(hash, e)
		p.pendings.Set(hash, e)
		p.pendingsUseByRecover.Set(hash, e)
	}

	p.HandleRecover(map[string]struct{}{"active": {}}, 200, true)

	require.Equal(t, []string{"active"}, p.fires.Keys())
	require.Equal(t, []string{"active"}, p.pendings.Keys())
	require.Equal(t, []string{"active"}, p.pendingsUseByRecover.Keys())

	items := queue.EventQueue.PopBackBy(10)

	require.Len(t, items, 1)
	require.True(t, items[0].(*models.AlertCurEvent).IsRecovered)
}

func TestDeleteProcessEvent_RemovesAllProcessingState(t *testing.T) {
	p := stateProcessor(t, &models.AlertRule{Id: 1})
	event := &models.AlertCurEvent{Hash: "active"}
	p.fires.Set("active", event)
	p.pendings.Set("active", event)
	p.pendingsUseByRecover.Set("active", event)

	p.DeleteProcessEvent("active")

	require.Empty(t, p.fires.GetAll())
	require.Empty(t, p.pendings.GetAll())
	require.Empty(t, p.pendingsUseByRecover.GetAll())
}

func TestAlertCurEventMap_ResetMutedShadowAndUpdateEvalTime(t *testing.T) {
	m := NewAlertCurEventMap(map[string]*models.AlertCurEvent{"h": {
		LastSentTime:         10,
		NotifyCurNumber:      2,
		MutedPersistNumber:   3,
		LastMutedPersistTime: 40,
	}})
	m.ResetMutedShadow("missing")
	m.UpdateLastEvalTime("missing", 99)
	m.ResetMutedShadow("h")
	m.UpdateLastEvalTime("h", 50)
	e, ok := m.Get("h")

	require.True(t, ok)
	require.Equal(t, int64(50), e.LastEvalTime)
	require.Zero(t, e.MutedPersistNumber)
	require.Zero(t, e.LastMutedPersistTime)
	require.Equal(t, 2, e.NotifyCurNumber)
	require.Equal(t, int64(10), e.LastSentTime)
}

func TestAlertCurEventMap_SetAllReplacesEvents(t *testing.T) {
	m := NewAlertCurEventMap(map[string]*models.AlertCurEvent{"old": {Hash: "old"}})

	m.SetAll(map[string]*models.AlertCurEvent{"new": {Hash: "new"}})

	require.Equal(t, []string{"new"}, m.Keys())
}

func TestBuildEvent_EnrichesTagsAndResolvesRelabeledTarget(t *testing.T) {
	for _, from := range []string{"inner", "external"} {
		t.Run(from, func(t *testing.T) {
			p := stateProcessor(t, &models.AlertRule{
				Id:                 1,
				Name:               "CPU",
				GroupId:            7,
				AppendTagsJSON:     []string{"team={{$labels.env}}"},
				Annotations:        `{"summary":"hot"}`,
				EventRelabelConfig: []*pconf.RelabelConfig{{Action: "replace", TargetLabel: "ident", Replacement: "host-a"}},
			})
			p.ctx = &ctx.Context{}
			p.datasourceId = 2
			p.datasourceCache = &memsto.DatasourceCacheType{}
			p.datasourceCache.Set(map[int64]*models.Datasource{2: {Id: 2, Name: "prom"}}, 1, 1)
			p.BusiGroupCache = &memsto.BusiGroupCacheType{}
			p.BusiGroupCache.Set(map[int64]*models.BusiGroup{7: {Id: 7, Name: "ops"}}, 1, 1)
			p.TargetCache = &memsto.TargetCacheType{}
			target := &models.Target{Ident: "host-a", Note: "node", GroupIds: []int64{7}}
			p.TargetCache.Set(map[string]*models.Target{"host-a": target}, 1, 1)
			point := models.AnomalyPoint{
				Labels:    model.Metric{"env": "prod"},
				Timestamp: 80,
				Severity:  2,
				Value:     90,
				Query:     "cpu > 80",
			}

			e := p.BuildEvent(point, from, 100, "rule-hash")

			require.Equal(t, "prom", e.Cluster)
			require.Equal(t, "ops", e.GroupName)
			require.Equal(t, "prod", e.TagsMap["team"])
			require.Equal(t, "CPU", e.TagsMap["rulename"])
			require.Equal(t, "host-a", e.TargetIdent)
			require.Equal(t, "node", e.TargetNote)
			require.Equal(t, []string{"ops"}, e.Target.GroupNames)
			// Enrichment belongs to the event copy; the cached target remains unchanged.
			require.Empty(t, target.GroupNames)
			require.NotSame(t, target, e.Target)
			require.NotContains(t, e.OriginalTags, "ident=")
			require.Equal(t, "hot", e.AnnotationsJSON["summary"])
			require.Equal(t, "rule-hash", e.RuleHash)
			require.Equal(t, Hash(1, 2, point), e.Hash)

			if from == "inner" {
				require.Equal(t, int64(100), e.LastEvalTime)
			} else {
				require.Equal(t, int64(80), e.LastEvalTime)
			}

		})
	}
}

func TestLabelMapToArr_SortsLabels(t *testing.T) {
	require.Equal(t, []string{"a=1", "b=2"}, labelMapToArr(map[string]string{"b": "2", "a": "1"}))
}

func TestHash_SeverityDistinguishesEventsButNotTagGroups(t *testing.T) {
	point := models.AnomalyPoint{Labels: model.Metric{"env": "prod"}, Severity: 2}

	other := point
	other.Severity = 1

	// Severity distinguishes events, but shared tags keep them in the same inhibition group.
	require.NotEqual(t, Hash(1, 2, point), Hash(1, 2, other))
	require.Equal(t, TagHash(point), TagHash(other))
}
