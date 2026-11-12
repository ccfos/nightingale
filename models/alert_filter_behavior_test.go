package models

import (
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/stretchr/testify/require"
)

func TestTagFilter_VerifyAcceptsSupportedOperatorsAndLegacyOp(t *testing.T) {
	for _, op := range []string{"==", "!=", "=~", "!~", "in", "not in"} {
		t.Run(op, func(t *testing.T) {
			filter := TagFilter{Key: "env", Op: op, Value: "prod"}

			require.NoError(t, filter.Verify())

			require.Equal(t, op, filter.Func)
		})
	}
}

func TestTagFilter_VerifyRejectsMissingKeyAndUnknownOperator(t *testing.T) {
	cases := []struct {
		name   string
		filter TagFilter
		want   string
	}{
		{name: "missing key", filter: TagFilter{Func: "=="}, want: "tag key cannot be empty"},
		{name: "unknown operator", filter: TagFilter{Key: "env", Func: "contains"}, want: "invalid operation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.EqualError(t, tc.filter.Verify(), tc.want)
		})
	}
}

func TestParseTagFilter_CompilesRegexAndMembershipSets(t *testing.T) {
	cases := []struct {
		name  string
		value interface{}
		want  []string
	}{
		{name: "space separated", value: "prod staging prod", want: []string{"prod", "staging"}},
		{name: "integer slice", value: []int{1, 2}, want: []string{"1", "2"}},
		{name: "string slice", value: []string{"prod", "staging"}, want: []string{"prod", "staging"}},
		{name: "json slice", value: []interface{}{"prod", int(2), float64(3)}, want: []string{"prod", "2", "3"}},
		{name: "scalar", value: int(12), want: []string{"12"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parsed, err := ParseTagFilter([]TagFilter{{Key: "env", Func: "in", Value: tc.value}})

			require.NoError(t, err)
			require.Len(t, parsed[0].Vset, len(tc.want))
			for _, value := range tc.want {
				require.Contains(t, parsed[0].Vset, value)
			}
		})
	}
	for _, value := range []interface{}{"^prod-", int(123)} {
		t.Run("regex", func(t *testing.T) {
			parsed, err := ParseTagFilter([]TagFilter{{Key: "env", Func: "=~", Value: value}})

			require.NoError(t, err)
			if value == "^prod-" {
				require.True(t, parsed[0].Regexp.MatchString("prod-west"))
			} else {
				require.True(t, parsed[0].Regexp.MatchString("123"))
			}
		})
	}
}

func TestGetTagFilters_DecodesJSONMembershipAndRegex(t *testing.T) {
	filters := ormx.JSONArr(`[{"key":"env","func":"=~","value":"^prod-"},` +
		`{"key":"group","func":"not in","value":[1,"2",3]},` +
		`{"key":"region","func":"in","value":"west east"}]`)

	parsed, err := GetTagFilters(filters)

	require.NoError(t, err)
	require.Len(t, parsed, 3)
	require.True(t, parsed[0].Regexp.MatchString("prod-west"))
	require.False(t, parsed[0].Regexp.MatchString("staging-west"))
	require.Equal(t, map[string]struct{}{"1": {}, "2": {}, "3": {}}, parsed[1].Vset)
	require.Equal(t, map[string]struct{}{"west": {}, "east": {}}, parsed[2].Vset)
}

func TestTagFilterParsing_RejectsMalformedRegexAndUnsupportedRegexValues(t *testing.T) {
	for _, value := range []interface{}{"[", true} {
		t.Run("invalid regex", func(t *testing.T) {
			parsed, err := ParseTagFilter([]TagFilter{{Key: "env", Func: "=~", Value: value}})

			require.Error(t, err)
			require.Nil(t, parsed)
		})
	}
	for _, raw := range []string{"{", `[{"key":"env","func":"=~","value":"["}]`,
		`[{"key":"env","func":"=~","value":true}]`} {
		t.Run("invalid json filter", func(t *testing.T) {
			parsed, err := GetTagFilters(ormx.JSONArr(raw))

			require.Error(t, err)
			require.Nil(t, parsed)
		})
	}
}

func TestAlertMute_VerifyChecksGroupTimeRangeAndFilters(t *testing.T) {
	cases := []struct {
		name   string
		change func(*AlertMute)
		want   string
	}{
		{name: "negative group", change: func(m *AlertMute) { m.GroupId = -1 }, want: "group_id invalid"},
		{name: "equal bounds", change: func(m *AlertMute) { m.Etime = m.Btime }, want: "<= btime"},
		{name: "reversed bounds", change: func(m *AlertMute) { m.Etime = m.Btime - 1 }, want: "<= btime"},
		{name: "invalid tags", change: func(m *AlertMute) { m.Tags = ormx.JSONArr("{") }, want: "unexpected end"},
		{name: "valid", change: func(m *AlertMute) {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mute := AlertMute{Btime: 100, Etime: 200, DatasourceIdsJson: []int64{3, 0, 4}}
			tc.change(&mute)

			err := mute.Verify()

			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
			} else {
				require.NoError(t, err)
				require.Equal(t, []int64{0}, mute.DatasourceIdsJson)
			}
		})
	}
}

func TestAlertMute_IsWithinTimeRangeIncludesBothBounds(t *testing.T) {
	mute := AlertMute{Btime: 100, Etime: 200}
	cases := []struct {
		name      string
		timestamp int64
		want      bool
	}{
		{name: "before", timestamp: 99},
		{name: "start", timestamp: 100, want: true},
		{name: "inside", timestamp: 150, want: true},
		{name: "end", timestamp: 200, want: true},
		{name: "after", timestamp: 201},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, mute.IsWithinTimeRange(tc.timestamp))
		})
	}
}

func TestAlertMute_IsWithinPeriodicMuteHonorsWeekdayAndOvernightWindows(t *testing.T) {
	cases := []struct {
		name   string
		start  string
		end    string
		days   string
		hour   int
		minute int
		want   bool
	}{
		{name: "start inclusive", start: "09:00", end: "17:00", days: "1", hour: 9, want: true},
		{name: "end exclusive", start: "09:00", end: "17:00", days: "1", hour: 17},
		{name: "wrong weekday", start: "09:00", end: "17:00", days: "2", hour: 10},
		{name: "overnight evening", start: "22:00", end: "06:00", days: "1", hour: 23, want: true},
		{name: "overnight morning", start: "22:00", end: "06:00", days: "1", hour: 5, want: true},
		{name: "overnight outside", start: "22:00", end: "06:00", days: "1", hour: 12},
		{name: "equal bounds all day", start: "09:00", end: "09:00", days: "1", hour: 12, want: true},
		{name: "full day", start: "00:00", end: "23:59", days: "1", hour: 23, minute: 59, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Build the Monday in the same timezone used by the production formatter.
			timestamp := time.Date(2024, time.January, 1, tc.hour, tc.minute, 0, 0, time.Local).Unix()
			mute := AlertMute{PeriodicMutesJson: []PeriodicMute{{
				EnableStime: tc.start, EnableEtime: tc.end, EnableDaysOfWeek: tc.days,
			}}}

			require.Equal(t, tc.want, mute.IsWithinPeriodicMute(timestamp))
		})
	}
}

func TestAlertSubscribe_VerifyValidatesLegacyNotificationSettings(t *testing.T) {
	cases := []struct {
		name   string
		change func(*AlertSubscribe)
		want   string
	}{
		{
			name:   "missing severity",
			change: func(s *AlertSubscribe) { s.SeveritiesJson = nil },
			want:   "severities is required",
		},
		{name: "missing channel", change: func(s *AlertSubscribe) { s.NewChannels = "" }, want: "new_channels is required"},
		{
			name:   "invalid group id",
			change: func(s *AlertSubscribe) { s.UserGroupIds = "abc" },
			want:   "user_group_ids invalid",
		},
		{name: "invalid tags", change: func(s *AlertSubscribe) { s.Tags = ormx.JSONArr("{") }, want: "unexpected end"},
		{name: "valid", change: func(s *AlertSubscribe) {}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub := AlertSubscribe{
				SeveritiesJson: []int{1, 2}, UserGroupIds: "7 8", NewChannels: "email", NotifyRuleIds: []int64{9},
			}
			tc.change(&sub)

			err := sub.Verify()

			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
			} else {
				require.NoError(t, err)
				require.Empty(t, sub.NotifyRuleIds)
			}
		})
	}
}

func TestAlertSubscribe_VerifyModernNotificationsClearsLegacyFields(t *testing.T) {
	sub := AlertSubscribe{
		NotifyVersion: 1, NotifyRuleIds: []int64{9}, SeveritiesJson: []int{1}, DatasourceIdsJson: []int64{3, 0},
		UserGroupIds: "7", RedefineChannels: 1, NewChannels: "email", RedefineWebhooks: 1, Webhooks: "/callback",
		RedefineSeverity: 1, NewSeverity: 2,
	}

	require.NoError(t, sub.Verify())

	require.Equal(t, []int64{0}, sub.DatasourceIdsJson)
	require.Equal(t, []int64{9}, sub.NotifyRuleIds)
	require.Empty(t, sub.UserGroupIds)
	require.Empty(t, sub.NewChannels)
	require.Empty(t, sub.Webhooks)
	require.Zero(t, sub.RedefineChannels)
	require.Zero(t, sub.RedefineWebhooks)
	require.Zero(t, sub.RedefineSeverity)
	require.Zero(t, sub.NewSeverity)
	sub.NotifyRuleIds = nil
	require.EqualError(t, sub.Verify(), "no notify rules selected")
}

func TestAlertSubscribe_FE2DBRoundTripsJSONFields(t *testing.T) {
	sub := AlertSubscribe{
		DatasourceIdsJson: []int64{3, 4}, WebhooksJson: []string{"/callback"}, SeveritiesJson: []int{1, 2},
		ExtraConfigJson: map[string]interface{}{"repeat": float64(60)},
	}

	require.NoError(t, sub.FE2DB())
	decoded := AlertSubscribe{DatasourceIds: sub.DatasourceIds, Webhooks: sub.Webhooks,
		Severities: sub.Severities, ExtraConfig: sub.ExtraConfig}
	require.NoError(t, decoded.DB2FE())

	require.JSONEq(t, `[3,4]`, sub.DatasourceIds)
	require.Equal(t, sub.DatasourceIdsJson, decoded.DatasourceIdsJson)
	require.Equal(t, sub.WebhooksJson, decoded.WebhooksJson)
	require.Equal(t, sub.SeveritiesJson, decoded.SeveritiesJson)
	require.Equal(t, sub.ExtraConfigJson, decoded.ExtraConfigJson)
}

func TestAlertSubscribe_DB2FERejectsMalformedJSONFields(t *testing.T) {
	cases := []struct {
		name string
		sub  AlertSubscribe
	}{
		{name: "datasources", sub: AlertSubscribe{DatasourceIds: "{"}},
		{name: "webhooks", sub: AlertSubscribe{Webhooks: "{"}},
		{name: "extra config", sub: AlertSubscribe{ExtraConfig: "{"}},
		{name: "severities", sub: AlertSubscribe{Severities: "{"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.sub.DB2FE())
		})
	}
}

func TestAlertSubscribe_MatchClusterSupportsWildcardAndExactIDs(t *testing.T) {
	cases := []struct {
		name    string
		ids     []int64
		eventID int64
		want    bool
	}{
		{name: "unrestricted", eventID: 3, want: true},
		{name: "event without datasource", ids: []int64{3}, want: true},
		{name: "wildcard", ids: []int64{0}, eventID: 3, want: true},
		{name: "exact", ids: []int64{3, 4}, eventID: 4, want: true},
		{name: "mismatch", ids: []int64{3}, eventID: 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sub := AlertSubscribe{DatasourceIdsJson: tc.ids}

			require.Equal(t, tc.want, sub.MatchCluster(tc.eventID))
		})
	}
}

func TestAlertSubscribe_MatchProdSupportsUnrestrictedAndExactProduct(t *testing.T) {
	require.True(t, (&AlertSubscribe{}).MatchProd(METRIC))
	require.True(t, (&AlertSubscribe{Prod: METRIC}).MatchProd(METRIC))
	require.False(t, (&AlertSubscribe{Prod: HOST}).MatchProd(METRIC))
}

func TestAlertSubscribe_ModifyEventAppliesExplicitOverrides(t *testing.T) {
	sub := AlertSubscribe{
		RedefineSeverity: 1, NewSeverity: 3, RedefineChannels: 1, NewChannels: "email wecom", UserGroupIds: "7 8",
		RedefineWebhooks: 1, Webhooks: `["/callback"]`, WebhooksJson: []string{"/callback"}, NotifyRuleIds: []int64{9},
	}
	event := AlertCurEvent{Severity: 1, NotifyChannels: "old", NotifyRuleIds: []int64{1}}

	sub.ModifyEvent(&event)

	require.Equal(t, 3, event.Severity)
	require.Equal(t, "email wecom", event.NotifyChannels)
	require.Equal(t, []string{"email", "wecom"}, event.NotifyChannelsJSON)
	require.Equal(t, sub.Webhooks, event.Callbacks)
	require.Equal(t, sub.WebhooksJson, event.CallbacksJSON)
	require.Equal(t, "7 8", event.NotifyGroups)
	require.Equal(t, []string{"7", "8"}, event.NotifyGroupsJSON)
	require.Equal(t, []int64{9}, event.NotifyRuleIds)
}

func TestAlertSubscribe_ModifyEventClearsCallbacksUnlessExplicitlyRedefined(t *testing.T) {
	event := AlertCurEvent{Severity: 1, NotifyChannels: "email", Callbacks: "/old", CallbacksJSON: []string{"/old"},
		NotifyRuleIds: []int64{9}}

	(&AlertSubscribe{}).ModifyEvent(&event)

	require.Equal(t, 1, event.Severity)
	require.Equal(t, "email", event.NotifyChannels)
	require.Empty(t, event.Callbacks)
	require.Empty(t, event.CallbacksJSON)
	require.Empty(t, event.NotifyRuleIds)
	require.Empty(t, event.NotifyGroupsJSON)
}

func TestAlertSubscribe_CompatibleWithOldRuleIdKeepsModernIDs(t *testing.T) {
	sub := AlertSubscribe{RuleId: 42}

	sub.CompatibleWithOldRuleId()

	require.Equal(t, []int64{42}, sub.RuleIds)
	sub.RuleIds = []int64{7, 8}
	sub.CompatibleWithOldRuleId()
	require.Equal(t, []int64{7, 8}, sub.RuleIds)
}
