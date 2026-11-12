package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecordingRuleVerify_RejectsInvalidModelFields(t *testing.T) {
	cases := []struct {
		name string
		rule RecordingRule
		want string
	}{
		{name: "negative group", rule: RecordingRule{GroupId: -1}, want: "GroupId(-1) invalid"},
		{name: "invalid metric", rule: RecordingRule{Name: "bad-name", PromQl: "up"}, want: "Name has invalid chreacters"},
		{
			name: "invalid query metric",
			rule: RecordingRule{QueryConfigsJson: []QueryConfig{{NewMetric: "bad-name"}}},
			want: "Metric Name has invalid chreacters",
		},
		{name: "missing tag value", rule: RecordingRule{AppendTags: "env"}, want: "AppendTags(env) invalid"},
		{
			name: "invalid tag name", rule: RecordingRule{AppendTags: "bad-name=prod"},
			want: "AppendTags(bad-name=prod) invalid",
		},
		{
			name: "oversized query config",
			rule: RecordingRule{QueryConfigs: strings.Repeat("x", 65536)},
			want: "query_configs length (65536 bytes) exceeds TEXT type limit (65535 bytes), please reduce the configuration size",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.Verify()

			require.EqualError(t, err, tc.want)
		})
	}
}

func TestRecordingRuleConversion_UpgradesLegacyDatasourceSelections(t *testing.T) {
	rule := RecordingRule{
		Name: "job:up", PromQl: "up", DatasourceIdsJson: []int64{3, 4}, PromEvalInterval: 30,
		AppendTagsJSON: []string{"env=prod", "team=ops"},
		QueryConfigsJson: []QueryConfig{{
			NewMetric: "job:up", Queries: []Query{{Cate: PROMETHEUS, DatasourceIds: []int64{5, 6}}},
		}},
	}

	rule.FE2DB()
	loaded := RecordingRule{
		DatasourceIds: rule.DatasourceIds, QueryConfigs: rule.QueryConfigs,
		AppendTags: rule.AppendTags, PromEvalInterval: rule.PromEvalInterval,
	}
	require.NoError(t, loaded.DB2FE())

	require.Equal(t, rule.DatasourceIdsJson, loaded.DatasourceIdsJson)
	require.Equal(t, rule.AppendTagsJSON, loaded.AppendTagsJSON)
	require.Equal(t, "@every 30s", loaded.CronPattern)
	require.Equal(t, []DatasourceQuery{{MatchType: 0, Op: "in", Values: []interface{}{int64(3), int64(4)}}},
		loaded.DatasourceQueries)
	queries := loaded.QueryConfigsJson[0].Queries
	require.Equal(t, []DatasourceQuery{{MatchType: 0, Op: "in", Values: []interface{}{int64(5), int64(6)}}},
		queries[0].DatasourceQueries)
}

func TestRecordingRuleFillDatasourceQueries_PreservesExplicitSelectionAndSupportsAll(t *testing.T) {
	explicit := []DatasourceQuery{{MatchType: 1, Op: "=", Values: []interface{}{"prod"}}}
	cases := []struct {
		name string
		rule RecordingRule
		want []DatasourceQuery
	}{
		{
			name: "all", rule: RecordingRule{DatasourceIds: "[0,3]"},
			want: []DatasourceQuery{{MatchType: 2, Op: "in", Values: []interface{}{}}},
		},
		{name: "explicit", rule: RecordingRule{DatasourceIds: "[0]", DatasourceQueries: explicit}, want: explicit},
		{name: "absent", rule: RecordingRule{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.rule.FillDatasourceQueries()

			require.NoError(t, err)
			require.Equal(t, tc.want, tc.rule.DatasourceQueries)
		})
	}
}

func TestRecordingRuleUpdate_PreservesCreationMetadataAndWritesZeroValues(t *testing.T) {
	c := pipelineContext(t, &RecordingRule{})
	original := RecordingRule{Id: 42, Name: "old", GroupId: 7, Disabled: 1, CreateAt: 100, CreateBy: "creator"}
	require.NoError(t, c.DB.Create(&original).Error)
	replacement := RecordingRule{
		Id: 99, Name: "new", GroupId: 7, CreateAt: 999, CreateBy: "untrusted", UpdateBy: "operator",
		PromQl: "up", DatasourceIdsJson: []int64{3}, AppendTagsJSON: []string{"env=prod"},
	}

	require.NoError(t, original.Update(c, replacement))
	stored, err := RecordingRuleGetById(c, 42)

	require.NoError(t, err)
	require.Equal(t, int64(42), stored.Id)
	require.Equal(t, int64(100), stored.CreateAt)
	require.Equal(t, "creator", stored.CreateBy)
	require.Equal(t, "operator", stored.UpdateBy)
	require.Equal(t, "new", stored.Name)
	require.Zero(t, stored.Disabled)
	require.Equal(t, []int64{3}, stored.DatasourceIdsJson)
	require.Equal(t, []string{"env=prod"}, stored.AppendTagsJSON)
	require.Equal(t, 60, stored.PromEvalInterval)
}

func TestRecordingRuleQueries_FilterGroupsAndDisabledRows(t *testing.T) {
	c := pipelineContext(t, &RecordingRule{})
	require.NoError(t, c.DB.Create(&[]RecordingRule{
		{Id: 1, Name: "z_metric", GroupId: 7, UpdateAt: 100, DatasourceIds: "[3]"},
		{Id: 2, Name: "a_metric", GroupId: 7, Disabled: 1, UpdateAt: 900},
		{Id: 3, Name: "other_metric", GroupId: 8, UpdateAt: 200},
	}).Error)

	group, err := RecordingRuleGets(c, 7)

	require.NoError(t, err)
	require.Len(t, group, 2)
	require.Equal(t, "a_metric", group[0].Name)
	require.Equal(t, []int64{3}, group[1].DatasourceIdsJson)
	groups, err := RecordingRuleGetsByBGIds(c, []int64{8})
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, int64(3), groups[0].Id)
	all, err := RecordingRuleGetsByBGIds(c, nil)
	require.NoError(t, err)
	require.Len(t, all, 3)
	enabled, err := RecordingRuleEnabledGets(c)
	require.NoError(t, err)
	require.Len(t, enabled, 2)
	for _, rule := range enabled {
		require.Zero(t, rule.Disabled)
	}
	clusterRules, err := RecordingRuleGetsByCluster(c)
	require.NoError(t, err)
	require.Equal(t, enabled, clusterRules)
	stats, err := RecordingRuleStatistics(c)
	require.NoError(t, err)
	// The recording-rule watermark includes disabled rows, unlike the runtime rule list.
	require.Equal(t, int64(3), stats.Total)
	require.Equal(t, int64(900), stats.LastUpdated)
	missing, err := RecordingRuleGetById(c, 99)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestRecordingRuleDels_RestrictsDeletionToBusinessGroup(t *testing.T) {
	c := pipelineContext(t, &RecordingRule{})
	first := RecordingRule{Name: "metric", PromQl: "up", GroupId: 7}
	second := RecordingRule{Name: "metric", PromQl: "up", GroupId: 8}
	require.NoError(t, first.Add(c))
	require.NoError(t, second.Add(c))

	require.NoError(t, RecordingRuleDels(c, []int64{first.Id, second.Id}, 7))

	deleted, err := RecordingRuleGetById(c, first.Id)
	require.NoError(t, err)
	require.Nil(t, deleted)
	retained, err := RecordingRuleGetById(c, second.Id)
	require.NoError(t, err)
	require.Equal(t, int64(8), retained.GroupId)
}

func TestRecordingRuleUpgradeToV6_MapsClustersAndAllDatasourceMarker(t *testing.T) {
	c := pipelineContext(t, &RecordingRule{})
	require.NoError(t, c.DB.Create(&[]RecordingRule{
		{Id: 1, Cluster: "$all"},
		{Id: 2, Cluster: "prod missing staging"},
	}).Error)

	require.NoError(t, RecordingRuleUpgradeToV6(c, map[string]Datasource{"prod": {Id: 3}, "staging": {Id: 4}}))

	all, err := RecordingRuleGetById(c, 1)
	require.NoError(t, err)
	require.Equal(t, []int64{0}, all.DatasourceIdsJson)
	selected, err := RecordingRuleGetById(c, 2)
	require.NoError(t, err)
	require.Equal(t, []int64{3, 4}, selected.DatasourceIdsJson)
}
