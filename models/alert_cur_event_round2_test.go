package models

import (
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
)

func seedCurrentEventsRound2(t *testing.T, c *ctx.Context) {
	t.Helper()
	require.NoError(t, c.DB.Create(&[]AlertCurEvent{
		{
			Id: 1, RuleId: 11, RuleName: "CPU high", RuleProd: "metric", GroupId: 7, Severity: 1,
			DatasourceId: 3, Cate: PROMETHEUS, TriggerTime: 100, Hash: "cpu-a", Tags: "host=a,,env=prod",
			Annotations: "{}", RuleConfig: "{}",
		},
		{
			Id: 2, RuleId: 11, RuleName: "CPU high", RuleProd: "metric", GroupId: 7, Severity: 2,
			DatasourceId: 4, Cate: PROMETHEUS, TriggerTime: 200, Hash: "cpu-b", Tags: "host=b,,env=prod",
			Annotations: "{}", RuleConfig: "{}",
		},
		{
			Id: 3, RuleId: 12, RuleName: "Disk full", RuleProd: "host", GroupId: 8, Severity: 1,
			DatasourceId: 3, Cate: HOST, TriggerTime: 300, Hash: "disk-c", Tags: "host=c,,env=test",
			Annotations: "{}", RuleConfig: "{}",
		},
	}).Error)
}

func TestAlertCurEventQueries_ApplyEachFilterToBothCountAndList(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{})
	seedCurrentEventsRound2(t, c)
	cases := []struct {
		name     string
		prods    []string
		groups   []int64
		start    int64
		end      int64
		severity []int64
		ds       []int64
		cates    []string
		rule     int64
		query    string
		ids      []int64
		want     []int64
	}{
		{name: "all", want: []int64{3, 2, 1}},
		{name: "product", prods: []string{"metric"}, want: []int64{2, 1}},
		{name: "group", groups: []int64{8}, want: []int64{3}},
		{name: "inclusive time", start: 100, end: 200, want: []int64{2, 1}},
		{name: "severity", severity: []int64{1}, want: []int64{3, 1}},
		{name: "datasource", ds: []int64{4}, want: []int64{2}},
		{name: "category", cates: []string{HOST}, want: []int64{3}},
		{name: "rule", rule: 11, want: []int64{2, 1}},
		{name: "ids", ids: []int64{1, 3}, want: []int64{3, 1}},
		{name: "name and tag terms", query: "CPU host=b", want: []int64{2}},
		{name: "intersecting filters", groups: []int64{7}, severity: []int64{1}, ds: []int64{4}, want: []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			total, err := AlertCurEventTotal(c, tc.prods, tc.groups, tc.start, tc.end,
				tc.severity, tc.ds, tc.cates, tc.rule, tc.query, tc.ids)
			require.NoError(t, err)
			events, err := AlertCurEventsGet(c, tc.prods, tc.groups, tc.start, tc.end,
				tc.severity, tc.ds, tc.cates, tc.rule, tc.query, 10, 0, tc.ids)

			require.NoError(t, err)
			require.Equal(t, int64(len(tc.want)), total)
			ids := make([]int64, 0, len(events))
			for _, event := range events {
				ids = append(ids, event.Id)
				require.Contains(t, event.TagsJSON, "host="+map[int64]string{1: "a", 2: "b", 3: "c"}[event.Id])
			}
			require.Equal(t, tc.want, ids)
		})
	}
}

func TestAlertCurEventsGet_PaginatesAfterOrdering(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{})
	seedCurrentEventsRound2(t, c)

	events, err := AlertCurEventsGet(c, nil, nil, 0, 0, nil, nil, nil, 0, "", 1, 1, nil)

	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, int64(2), events[0].Id)
}

func TestAlertCurEventGet_ResolvesNotifyGroupsAndMissingRows(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{}, &UserGroup{})
	event := AlertCurEvent{Id: 1, NotifyGroups: "7 invalid 99", Tags: "env=prod"}
	require.NoError(t, event.Add(c))
	require.NoError(t, c.DB.Create(&UserGroup{Id: 7, Name: "Operations"}).Error)

	stored, err := AlertCurEventGetById(c, 1)

	require.NoError(t, err)
	require.Equal(t, []string{"env=prod"}, stored.TagsJSON)
	require.Len(t, stored.NotifyGroupsObj, 1)
	require.Equal(t, "Operations", stored.NotifyGroupsObj[0].Name)
	missing, err := AlertCurEventGetById(c, 99)
	require.NoError(t, err)
	require.Nil(t, missing)
	exists, err := AlertCurEventExists(c, "id = ?", 1)
	require.NoError(t, err)
	require.True(t, exists)
}

func TestAlertCurEventGetByIds_OrdersAndDecodesOnlyRequestedEvents(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{})
	seedCurrentEventsRound2(t, c)

	events, err := AlertCurEventGetByIds(c, []int64{1, 3, 99})

	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, int64(3), events[0].Id)
	require.Equal(t, int64(1), events[1].Id)
	require.Equal(t, []string{"host=a", "env=prod"}, events[1].TagsJSON)
	events, err = AlertCurEventGetByIds(c, nil)
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestAlertCurEventGetByRuleIdAndDsId_RequiresBothIdentifiers(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{})
	seedCurrentEventsRound2(t, c)

	events, err := AlertCurEventGetByRuleIdAndDsId(c, 11, 4)

	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, int64(2), events[0].Id)
	require.Equal(t, []string{"host=b", "env=prod"}, events[0].TagsJSON)
}

func TestAlertCurEventGetMap_GroupsHashesByRuleAndFiltersDatasource(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{})
	seedCurrentEventsRound2(t, c)

	hashes, err := AlertCurEventGetMap(c, "3")

	require.NoError(t, err)
	require.Equal(t, map[int64]map[string]struct{}{11: {"cpu-a": {}}, 12: {"disk-c": {}}}, hashes)
	hashes, err = AlertCurEventGetMap(c, "")
	require.NoError(t, err)
	require.Len(t, hashes[11], 2)
}

func TestAlertNumbers_CountsOnlyRequestedBusinessGroups(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{})
	seedCurrentEventsRound2(t, c)

	counts, err := AlertNumbers(c, []int64{7, 99})

	require.NoError(t, err)
	require.Equal(t, map[int64]int64{7: 2}, counts)
	counts, err = AlertNumbers(c, nil)
	require.NoError(t, err)
	require.Empty(t, counts)
}

func TestAlertCurEventGetsFromAlertMute_FiltersGroupSeverityAndDatasource(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{})
	seedCurrentEventsRound2(t, c)
	mute := AlertMute{GroupId: 7, SeveritiesJson: []int{1}, DatasourceIdsJson: []int64{3}}

	events, err := AlertCurEventGetsFromAlertMute(c, &mute)

	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, int64(1), events[0].Id)
	mute.SeveritiesJson = nil
	mute.DatasourceIdsJson = []int64{0}
	events, err = AlertCurEventGetsFromAlertMute(c, &mute)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, int64(2), events[0].Id)
	require.Equal(t, int64(1), events[1].Id)
}

func TestAlertCurEventDel_DeletesOnlyRequestedIDs(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{})
	seedCurrentEventsRound2(t, c)

	require.NoError(t, AlertCurEventDel(c, nil))
	require.NoError(t, AlertCurEventDel(c, []int64{3, 99}))

	ids, err := AlertCurEventIds(c)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{1, 2}, ids)
}

func TestAlertCurEventDelByHash_DeletesOnlyMatchingHash(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{})
	seedCurrentEventsRound2(t, c)

	require.NoError(t, AlertCurEventDelByHash(c, "cpu-a"))

	ids, err := AlertCurEventIds(c)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{2, 3}, ids)
}

func TestAlertCurEventStatistics_UsesExplicitDayAndWeekBoundaries(t *testing.T) {
	c := pipelineContext(t, &AlertCurEvent{}, &AlertHisEvent{})
	// Wednesday makes the Monday boundary distinct from both today and the last 24 hours.
	now := time.Date(2026, time.November, 11, 12, 0, 0, 0, time.UTC)
	midnight := time.Date(2026, time.November, 11, 0, 0, 0, 0, time.UTC)
	monday := time.Date(2026, time.November, 9, 0, 0, 0, 0, time.UTC)
	require.NoError(t, c.DB.Create(&[]AlertCurEvent{
		{Id: 1, TriggerTime: now.Add(-24*time.Hour - time.Second).Unix()},
		{Id: 2, TriggerTime: now.Add(-24 * time.Hour).Unix()},
	}).Error)
	require.NoError(t, c.DB.Create(&[]AlertHisEvent{
		{Id: 1, TriggerTime: midnight.Unix()},
		{Id: 2, TriggerTime: midnight.Unix(), IsRecovered: 1},
		{Id: 3, TriggerTime: monday.Unix()},
		{Id: 4, TriggerTime: monday.Add(-time.Second).Unix()},
	}).Error)

	stats := AlertCurEventStatistics(c, now)

	require.Equal(t, map[string]interface{}{
		"total": int64(2), "total_24_ago": int64(1), "total_today": int64(1), "total_week": int64(2),
	}, stats)
}

func TestAlertCurEventDeepCopy_IsolatesMutableNotificationAndTagData(t *testing.T) {
	original := AlertCurEvent{
		NotifyGroupsObj: []*UserGroup{{Name: "ops"}, nil}, NotifyUsersObj: []*User{{Username: "alice"}, nil},
		Target: &Target{Ident: "node-a"}, CallbacksJSON: []string{"callback"}, NotifyChannelsJSON: []string{"email"},
		NotifyGroupsJSON: []string{"7"}, TagsJSON: []string{"env=prod"}, TagsMap: map[string]string{"env": "prod"},
		OriginalTagsJSON: []string{"env=original"}, AnnotationsJSON: map[string]string{"summary": "CPU high"},
		ExtraInfo: []string{"detail"}, ShotImageBase64: map[string]string{"panel": "image"},
		ExtraInfoMap: []map[string]string{{"source": "original"}, nil}, NotifyRuleIds: []int64{3},
	}
	before := original.DeepCopy()

	clone := original.DeepCopy()
	clone.NotifyGroupsObj[0].Name = "changed"
	clone.NotifyUsersObj[0].Username = "changed"
	clone.Target.Ident = "changed"
	clone.CallbacksJSON[0] = "changed"
	clone.NotifyChannelsJSON[0] = "changed"
	clone.NotifyGroupsJSON[0] = "changed"
	clone.TagsJSON[0] = "changed"
	clone.TagsMap["env"] = "changed"
	clone.OriginalTagsJSON[0] = "changed"
	clone.AnnotationsJSON["summary"] = "changed"
	clone.ExtraInfo[0] = "changed"
	clone.ShotImageBase64["panel"] = "changed"
	clone.ExtraInfoMap[0]["source"] = "changed"
	clone.NotifyRuleIds[0] = 99

	require.Equal(t, *before, original)
	require.Nil(t, clone.NotifyGroupsObj[1])
	require.Nil(t, clone.NotifyUsersObj[1])
	require.Nil(t, clone.ExtraInfoMap[1])
}
