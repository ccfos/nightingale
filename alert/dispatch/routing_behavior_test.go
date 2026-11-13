package dispatch

import (
	"encoding/json"
	"html/template"
	"net/http"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestNotifyRuleMatchCheck_TimeRanges(t *testing.T) {
	// Ordinary ranges exclude the end; 23:59 is special and includes the last minute.
	for _, tc := range []struct {
		name, clock, start, end string
		week                    []int
		want                    string
	}{
		{name: "unrestricted"},
		{
			name:  "start inclusive",
			clock: "10:00",
			start: "10:00",
			end:   "12:00",
			week:  []int{1},
		},
		{
			name:  "end exclusive",
			clock: "12:00",
			start: "10:00",
			end:   "12:00",
			week:  []int{1},
			want:  "time",
		},
		{
			name:  "last minute inclusive",
			clock: "23:59",
			start: "10:00",
			end:   "23:59",
			week:  []int{1},
		},
		{
			name:  "overnight late",
			clock: "23:00",
			start: "21:00",
			end:   "09:00",
			week:  []int{1},
		},
		{
			name:  "overnight early",
			clock: "08:00",
			start: "21:00",
			end:   "09:00",
			week:  []int{1},
		},
		{
			name:  "overnight daytime",
			clock: "12:00",
			start: "21:00",
			end:   "09:00",
			week:  []int{1},
			want:  "time",
		},
		{
			name:  "wrong weekday",
			clock: "11:00",
			start: "10:00",
			end:   "12:00",
			week:  []int{2},
			want:  "time",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clock := tc.clock
			if clock == "" {
				clock = "11:00"
			}
			tm, err := time.ParseInLocation("2006-01-02 15:04", "2026-11-09 "+clock, time.Local)

			require.NoError(t, err)
			cfg := models.NotifyConfig{Severities: []int{2}}
			if tc.start != "" {
				cfg.TimeRanges = []models.TimeRanges{{Start: tc.start, End: tc.end, Week: tc.week}}
			}
			before := append([]models.TagFilter(nil), cfg.LabelKeys...)

			err = NotifyRuleMatchCheck(&cfg, &models.AlertCurEvent{
				TriggerTime: tm.Unix(),
				Severity:    2,
				RuleName:    "CPU",
				TagsMap:     map[string]string{"env": "prod"},
			})
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}

			require.Equal(t, before, cfg.LabelKeys, "cached filters must remain unchanged")
		})
	}
}

func TestNotifyRuleMatchCheck_SeverityAndTagFilters(t *testing.T) {
	for _, tc := range []struct {
		name          string
		severity      int
		labels, attrs []models.TagFilter
		want          string
	}{
		{name: "wrong severity", severity: 1, want: "severity"},
		{
			name:     "label match with op",
			severity: 2,
			labels:   []models.TagFilter{{Key: "env", Op: "==", Value: "prod"}},
		},
		{
			name:     "label mismatch",
			severity: 2,
			labels:   []models.TagFilter{{Key: "env", Func: "==", Value: "dev"}},
			want:     "tag not match",
		},
		{
			name:     "invalid label",
			severity: 2,
			labels:   []models.TagFilter{{Key: "env", Func: "=~", Value: "["}},
			want:     "parse tag",
		},
		{
			name:     "attribute match",
			severity: 2,
			attrs:    []models.TagFilter{{Key: "rule_name", Func: "==", Value: "CPU"}},
		},
		{
			name:     "attribute mismatch",
			severity: 2,
			attrs:    []models.TagFilter{{Key: "rule_name", Func: "==", Value: "Memory"}},
			want:     "attributes",
		},
		{
			name:     "invalid attribute",
			severity: 2,
			attrs:    []models.TagFilter{{Key: "rule_name", Func: "=~", Value: "["}},
			want:     "parse tag",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tm, err := time.ParseInLocation("2006-01-02 15:04", "2026-11-09 11:00", time.Local)

			require.NoError(t, err)
			cfg := models.NotifyConfig{Severities: []int{tc.severity}, LabelKeys: tc.labels, Attributes: tc.attrs}
			before := append([]models.TagFilter(nil), cfg.LabelKeys...)

			err = NotifyRuleMatchCheck(&cfg, &models.AlertCurEvent{
				TriggerTime: tm.Unix(),
				Severity:    2,
				RuleName:    "CPU",
				TagsMap:     map[string]string{"env": "prod"},
			})
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}

			require.Equal(t, before, cfg.LabelKeys, "cached filters must remain unchanged")
		})
	}
}

func TestPipelineApplicable_Filters(t *testing.T) {
	event := &models.AlertCurEvent{RuleName: "CPU", TagsMap: map[string]string{"env": "prod"}}
	for _, tc := range []struct {
		name string
		p    *models.EventPipeline
		want bool
	}{
		{name: "nil", p: nil, want: true},
		{name: "disabled filtering", p: &models.EventPipeline{}, want: true},
		{
			name: "both match",
			p: &models.EventPipeline{
				FilterEnable: true,
				LabelFilters: []models.TagFilter{{Key: "env", Op: "==", Value: "prod"}},
				AttrFilters:  []models.TagFilter{{Key: "rule_name", Func: "==", Value: "CPU"}},
			},
			want: true,
		},
		{
			name: "label mismatch",
			p: &models.EventPipeline{FilterEnable: true, LabelFilters: []models.TagFilter{{
				Key:   "env",
				Op:    "==",
				Value: "dev",
			}}},
			want: false,
		},
		{
			name: "invalid regexp",
			p: &models.EventPipeline{FilterEnable: true, LabelFilters: []models.TagFilter{{
				Key:   "env",
				Func:  "=~",
				Value: "[",
			}}},
			want: false,
		},
		{
			name: "attribute mismatch",
			p: &models.EventPipeline{
				FilterEnable: true,
				AttrFilters:  []models.TagFilter{{Key: "rule_name", Func: "==", Value: "Memory"}},
			},
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, PipelineApplicable(tc.p, event))

			if tc.p != nil && len(tc.p.LabelFilters) > 0 && tc.p.LabelFilters[0].Op != "" {
				require.Empty(t, tc.p.LabelFilters[0].Func)
			}
		})
	}
}

func TestShouldSkipNotify(t *testing.T) {
	for _, tc := range []struct {
		event *models.AlertCurEvent
		skip  bool
	}{
		{event: nil, skip: true},
		{event: &models.AlertCurEvent{}, skip: false},
		{event: &models.AlertCurEvent{IsRecovered: true}, skip: true},
		{event: &models.AlertCurEvent{IsRecovered: true, NotifyRecovered: 1}, skip: false},
	} {
		require.Equal(t, tc.skip, shouldSkipNotify(nil, tc.event, 1))
	}
}

func recipientCaches() (*memsto.UserCacheType, *memsto.UserGroupCacheType) {
	uc := &memsto.UserCacheType{}
	ugc := &memsto.UserGroupCacheType{}
	u1 := &models.User{Id: 1, Username: "alice", Email: "a@example.test", Phone: "123"}
	u2 := &models.User{Id: 2, Username: "bob", Email: "b@example.test"}
	uc.Set(map[int64]*models.User{1: u1, 2: u2, 3: {Id: 3}}, 3, 1, 0, 0)
	ugc.Set(map[int64]*models.UserGroup{7: {Id: 7, UserIds: []int64{1, 2, 3, 99}}}, 1, 1)
	return uc, ugc
}

func recipientConfig() *models.NotifyConfig {
	cfg := &models.NotifyConfig{Params: map[string]interface{}{
		"user_ids":                   []int64{1, 1, 99},
		"user_group_ids":             []int64{7},
		"ids":                        []int64{9},
		"pagerduty_integration_keys": []string{"routing"},
		"pagerduty_integration_ids":  []int64{4},
		"im_group_ids":               []string{"chat"},
		"attempts":                   3,
		"label":                      "ops",
	}}
	return cfg
}

func TestGetNotifyConfigParams_ResolvesRecipientsAndCustomParameters(t *testing.T) {
	uc, ugc := recipientCaches()
	cfg := recipientConfig()

	contacts, ids, keys, params, groups := GetNotifyConfigParams(cfg, "email", uc, ugc)

	require.ElementsMatch(t, []string{"a@example.test", "b@example.test"}, contacts)
	require.Equal(t, []int64{9}, ids)
	require.Equal(t, []string{"routing"}, keys)
	require.Equal(t, map[string]string{"attempts": "3", "label": "ops"}, params)
	require.Equal(t, []string{"chat"}, groups)

	// A user may belong to several groups; empty phone contacts are omitted.
	phones, _, _, _, _ := GetNotifyConfigParams(cfg, "phone", uc, ugc)

	require.Equal(t, []string{"123"}, phones)
}

func TestGetNotifyConfigParams_IgnoresMalformedUserIDs(t *testing.T) {
	uc, ugc := recipientCaches()
	cfg := &models.NotifyConfig{Params: map[string]interface{}{"user_ids": "invalid"}}

	empty, _, _, _, _ := GetNotifyConfigParams(cfg, "email", uc, ugc)

	require.Empty(t, empty)
}

func TestBuildNotifyContext_ResolvesProviderAndBuildsRequest(t *testing.T) {
	uc, ugc := recipientCaches()
	cfg := recipientConfig()

	events := []*models.AlertCurEvent{{Hash: "h"}}
	client := &http.Client{}
	tpl := map[string]interface{}{"body": "alert"}
	channel := &models.NotifyChannelConfig{
		Ident:       "callback",
		RequestType: "http",
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{
			URL:    "https://callback.test",
			Method: "POST",
		}},
	}
	nc, err := BuildNotifyContext(nil, uc, ugc, events, 42, cfg, channel, tpl, client, "https://n9e.test")

	require.NoError(t, err)
	require.NotNil(t, nc.Provider)
	require.Same(t, channel, nc.Request.Config)
	require.Equal(t, int64(42), nc.Request.NotifyRuleId)
	require.Equal(t, events, nc.Request.Events)
	require.Equal(t, tpl, nc.Request.TplContent)
	require.Same(t, client, nc.Request.HttpClient)
	require.Equal(t, "https://n9e.test", nc.Request.SiteUrl)
	require.Equal(t, map[string]string{"attempts": "3", "label": "ops"}, nc.Request.CustomParams)
}

func TestBuildNotifyContext_RejectsUnknownChannel(t *testing.T) {
	uc, ugc := recipientCaches()
	cfg := recipientConfig()
	events := []*models.AlertCurEvent{{Hash: "h"}}
	channel := &models.NotifyChannelConfig{Ident: "unknown", RequestType: "unknown"}

	_, err := BuildNotifyContext(nil, uc, ugc, events, 42, cfg, channel, nil, nil, "")

	require.ErrorContains(t, err, "unknown channel")
}

func TestFillUsers_DeduplicatesGroupMembers(t *testing.T) {
	uc, ugc := recipientCaches()
	event := &models.AlertCurEvent{NotifyGroupsJSON: []string{
		"bad",
		"7",
		"7",
		"99",
	}, NotifyChannelsJSON: []string{"email"}}

	fillUsers(event, uc, ugc)

	require.ElementsMatch(t, []*models.User{uc.GetByUserId(1), uc.GetByUserId(2), uc.GetByUserId(3)}, event.NotifyUsersObj)
	require.Len(t, event.NotifyGroupsObj, 1)
}

func TestNotifyGroupDispatch_RoutesAllGroupMembers(t *testing.T) {
	_, ugc := recipientCaches()
	event := &models.AlertCurEvent{NotifyGroupsJSON: []string{
		"bad",
		"7",
		"7",
		"99",
	}, NotifyChannelsJSON: []string{"email"}}

	target := NotifyGroupDispatch(nil, event, nil, &Dispatch{userGroupCache: ugc})

	require.ElementsMatch(t, []int64{1, 2, 3, 99}, target.ToChannelUserMap()["email"])
}

func TestNotifyTarget_OrMergeCombinesRecipients(t *testing.T) {
	a := NewNotifyTarget()
	a.userMap[1] = NotifyChannels{"email": true, "sms": false}
	b := NewNotifyTarget()
	b.userMap[1] = NotifyChannels{"email": false, "sms": true}
	b.userMap[2] = NewNotifyChannels([]string{"email"})
	b.callbacks["https://callback.test"] = struct{}{}
	b.webhooks["hook"] = &models.Webhook{Url: "hook"}

	a.OrMerge(nil)
	a.OrMerge(b)

	require.ElementsMatch(t, []int64{1, 2}, a.ToChannelUserMap()["email"])
	require.Equal(t, []int64{1}, a.ToChannelUserMap()["sms"])
}

func TestNotifyTarget_AndMergeDisablesSelectedChannels(t *testing.T) {
	a := NewNotifyTarget()
	a.userMap[1] = NotifyChannels{"email": true, "sms": false}
	b := NewNotifyTarget()
	b.userMap[1] = NotifyChannels{"email": false, "sms": true}
	b.userMap[2] = NewNotifyChannels([]string{"email"})
	b.callbacks["https://callback.test"] = struct{}{}
	b.webhooks["hook"] = &models.Webhook{Url: "hook"}

	a.OrMerge(b)
	mask := NewNotifyTarget()
	mask.userMap[1] = NotifyChannels{"email": false}

	a.AndMerge(mask)

	require.Equal(t, []int64{2}, a.ToChannelUserMap()["email"])
	require.ElementsMatch(t, []int64{1, 2}, a.ToUidList())
	require.Equal(t, b.webhooks, a.ToWebhookMap())
}

func TestEventCallbacksDispatch_IgnoresEmptyAndDuplicateCallbacks(t *testing.T) {
	a := NewNotifyTarget()
	a.callbacks["https://callback.test"] = struct{}{}

	event := &models.AlertCurEvent{CallbacksJSON: []string{"", "https://callback.test", "https://other.test"}}

	result := EventCallbacksDispatch(nil, event, a, nil)

	require.Nil(t, result)
	require.ElementsMatch(t, []string{"https://callback.test", "https://other.test"}, a.ToCallbackList())
}

func TestGenNoticeBytes_RendersTemplatesAndIncludesRenderErrors(t *testing.T) {
	d := &Dispatch{tpls: map[string]*template.Template{
		"ok":  template.Must(template.New("ok").Parse("{{.RuleName}}")),
		"bad": template.Must(template.New("bad").Parse("{{.Missing}}")),
	}}
	var notice Notice

	require.NoError(t, json.Unmarshal(d.genNoticeBytes(&models.AlertCurEvent{RuleName: "CPU"}), &notice))
	require.Equal(t, "CPU", notice.Event.RuleName)
	require.Equal(t, "CPU", notice.Tpls["ok"])
	require.Contains(t, notice.Tpls["bad"], "Missing")
}

func TestSubMatches_FiltersSubscriptions(t *testing.T) {
	event := &models.AlertCurEvent{
		RuleId:           10,
		DatasourceId:     3,
		RuleProd:         "metric",
		Cate:             "prometheus",
		Severity:         2,
		TriggerTime:      200,
		FirstTriggerTime: 100,
		NotifyRuleIds:    []int64{1},
	}
	base := models.AlertSubscribe{
		Id:                2,
		DatasourceIdsJson: []int64{3},
		Prod:              "metric",
		Cate:              "prometheus",
		ForDuration:       100,
		SeveritiesJson:    []int{2},
	}
	for _, tc := range []struct {
		name   string
		change func(*models.AlertSubscribe)
		want   bool
	}{
		{name: "match", change: func(*models.AlertSubscribe) {}, want: true},
		{name: "disabled", change: func(s *models.AlertSubscribe) { s.Disabled = 1 }, want: false}, {
			name:   "datasource",
			change: func(s *models.AlertSubscribe) { s.DatasourceIdsJson = []int64{4} },
			want:   false,
		},
		{name: "product", change: func(s *models.AlertSubscribe) { s.Prod = "host" }, want: false},
		{name: "category", change: func(s *models.AlertSubscribe) { s.Cate = "host" }, want: false}, {
			name:   "duration",
			change: func(s *models.AlertSubscribe) { s.ForDuration = 101 },
			want:   false,
		}, {
			name:   "severity",
			change: func(s *models.AlertSubscribe) { s.SeveritiesJson = []int{1} },
			want:   false,
		}, {
			name:   "wildcard severity",
			change: func(s *models.AlertSubscribe) { s.SeveritiesJson = []int{0} },
			want:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := base
			tc.change(&s)

			require.Equal(t, tc.want, (&Dispatch{}).subMatches(&s, event))
		})
	}
}

func TestCollectSubscribes_IncludesRuleSpecificAndGlobalRules(t *testing.T) {
	event := &models.AlertCurEvent{
		RuleId:           10,
		DatasourceId:     3,
		RuleProd:         "metric",
		Cate:             "prometheus",
		Severity:         2,
		TriggerTime:      200,
		FirstTriggerTime: 100,
		NotifyRuleIds:    []int64{1},
	}
	base := models.AlertSubscribe{
		Id:                2,
		DatasourceIdsJson: []int64{3},
		Prod:              "metric",
		Cate:              "prometheus",
		ForDuration:       100,
		SeveritiesJson:    []int{2},
	}
	sub := base
	sub.NotifyVersion = 1
	sub.NotifyRuleIds = []int64{9}
	cache := &memsto.AlertSubscribeCacheType{}
	cache.Set(map[int64][]*models.AlertSubscribe{10: {&sub}, 0: {{Disabled: 1}}}, 2, 1)
	d := &Dispatch{alertSubscribeCache: cache}

	require.Len(t, d.collectSubscribes(event), 2)
}

func TestCleanupNotifyMuted_RecoveredEventAlsoClearsSubscriptionRules(t *testing.T) {
	event := &models.AlertCurEvent{
		RuleId:           10,
		DatasourceId:     3,
		RuleProd:         "metric",
		Cate:             "prometheus",
		Severity:         2,
		TriggerTime:      200,
		FirstTriggerTime: 100,
		NotifyRuleIds:    []int64{1},
	}
	base := models.AlertSubscribe{
		Id:                2,
		DatasourceIdsJson: []int64{3},
		Prod:              "metric",
		Cate:              "prometheus",
		ForDuration:       100,
		SeveritiesJson:    []int{2},
	}
	sub := base
	sub.NotifyVersion = 1
	sub.NotifyRuleIds = []int64{9}
	cache := &memsto.AlertSubscribeCacheType{}
	cache.Set(map[int64][]*models.AlertSubscribe{10: {&sub}, 0: {{Disabled: 1}}}, 2, 1)
	d := &Dispatch{alertSubscribeCache: cache}

	old := NotifyMutedEventHook
	t.Cleanup(func() { NotifyMutedEventHook = old })
	var seen [][]int64
	NotifyMutedEventHook = func(e *models.AlertCurEvent) { seen = append(seen, append([]int64(nil), e.NotifyRuleIds...)) }

	d.CleanupNotifyMuted(event)

	require.Equal(t, [][]int64{{1}}, seen)

	// Recovery also clears the matching subscription rule without changing the original event.
	seen = nil
	event.IsRecovered = true

	d.CleanupNotifyMuted(event)

	require.Equal(t, [][]int64{{1}, {9}}, seen)
	require.Equal(t, []int64{1}, event.NotifyRuleIds)
}
