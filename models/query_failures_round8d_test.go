package models

import (
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// A query failure must be returned, rather than being mistaken for an empty successful sync.
func TestR8dCenterQueryFailures(t *testing.T) {
	cases := []struct {
		name string
		call func(*ctx.Context) error
	}{
		{"UserStatistics", func(c *ctx.Context) error { _, err := UserStatistics(c); return err }},
		{"DatasourceStatistics", func(c *ctx.Context) error { _, err := DatasourceStatistics(c); return err }},
		{"BusiGroupStatistics", func(c *ctx.Context) error { _, err := BusiGroupStatistics(c); return err }},
		{"AlertRuleStatistics", func(c *ctx.Context) error { _, err := AlertRuleStatistics(c); return err }},
		{"AlertMuteStatistics", func(c *ctx.Context) error { _, err := AlertMuteStatistics(c); return err }},
		{"AlertSubscribeStatistics", func(c *ctx.Context) error { _, err := AlertSubscribeStatistics(c); return err }},
		{"RecordingRuleStatistics", func(c *ctx.Context) error { _, err := RecordingRuleStatistics(c); return err }},
		{"NotifyChannelStatistics", func(c *ctx.Context) error { _, err := NotifyChannelStatistics(c); return err }},
		{"TaskTplStatistics", func(c *ctx.Context) error { _, err := TaskTplStatistics(c); return err }},
		{"ConfigsUserVariableStatistics", func(c *ctx.Context) error { _, err := ConfigsUserVariableStatistics(c); return err }},
		{"ConfigCvalStatistics", func(c *ctx.Context) error { _, err := ConfigCvalStatistics(c); return err }},
		{"UserGetAll", func(c *ctx.Context) error { _, err := UserGetAll(c); return err }},
		{"UserGetsBySso", func(c *ctx.Context) error { _, err := UserGetsBySso(c, "sso"); return err }},
		{"UserGet", func(c *ctx.Context) error { _, err := UserGet(c, "id=?", 1); return err }},
		{"UsersGet", func(c *ctx.Context) error { _, err := UsersGet(c, "id=?", 1); return err }},
		{"UserTotal", func(c *ctx.Context) error { _, err := UserTotal(c, "alice", 1, 100, nil); return err }},
		{"UserGets", func(c *ctx.Context) error {
			_, err := UserGets(c, "", 10, 0, 0, 0, "username", false, nil, nil, nil, nil)
			return err
		}},
		{"UserGetsByIds", func(c *ctx.Context) error { _, err := UserGetsByIds(c, []int64{1}); return err }},
		{"UsersGetByGroupIds", func(c *ctx.Context) error { _, err := UsersGetByGroupIds(c, []int64{1}); return err }},
		{"GetDatasources", func(c *ctx.Context) error { _, err := GetDatasources(c); return err }},
		{"DatasourceGetMap", func(c *ctx.Context) error { _, err := DatasourceGetMap(c); return err }},
		{"DatasourceGet", func(c *ctx.Context) error { _, err := DatasourceGet(c, 1); return err }},
		{"GetDatasourceInfosByIds", func(c *ctx.Context) error { _, err := GetDatasourceInfosByIds(c, []int64{1}); return err }},
		{"GetDatasourceIdsByEngineName", func(c *ctx.Context) error { _, err := GetDatasourceIdsByEngineName(c, "default"); return err }},
		{"GetDatasourcesCountBy", func(c *ctx.Context) error {
			_, err := GetDatasourcesCountBy(c, "prometheus", "metric", "test")
			return err
		}},
		{"GetDatasourcesGetsBy", func(c *ctx.Context) error {
			_, err := GetDatasourcesGetsBy(c, "prometheus", "metric", "test", "enabled")
			return err
		}},
		{"ConfigsGet", func(c *ctx.Context) error { _, err := ConfigsGet(c, "key"); return err }},
		{"ConfigsGetAll", func(c *ctx.Context) error { _, err := ConfigsGetAll(c); return err }},
		{"ConfigsSelectByCkey", func(c *ctx.Context) error { _, err := ConfigsSelectByCkey(c, "key"); return err }},
		{"ConfigGet", func(c *ctx.Context) error { _, err := ConfigGet(c, 1); return err }},
		{"ConfigsGetUserVariable", func(c *ctx.Context) error { _, err := ConfigsGetUserVariable(c); return err }},
		{"BusiGroupGetMap", func(c *ctx.Context) error { _, err := BusiGroupGetMap(c); return err }},
		{"BusiGroupGet", func(c *ctx.Context) error { _, err := BusiGroupGet(c, "id=?", 1); return err }},
		{"BusiGroupGetAll", func(c *ctx.Context) error { _, err := BusiGroupGetAll(c); return err }},
		{"AlertRuleGet", func(c *ctx.Context) error { _, err := AlertRuleGet(c, "id=?", 1); return err }},
		{"AlertRuleGetsAll", func(c *ctx.Context) error { _, err := AlertRuleGetsAll(c); return err }},
		{"AlertRuleExists", func(c *ctx.Context) error { _, err := AlertRuleExists(c, 1, 2, "name"); return err }},
		{"AlertMuteGet", func(c *ctx.Context) error { _, err := AlertMuteGet(c, "id=?", 1); return err }},
		{"AlertMuteGetsAll", func(c *ctx.Context) error { _, err := AlertMuteGetsAll(c); return err }},
		{"AlertSubscribeGet", func(c *ctx.Context) error { _, err := AlertSubscribeGet(c, "id=?", 1); return err }},
		{"AlertSubscribeGetsByService", func(c *ctx.Context) error { _, err := AlertSubscribeGetsByService(c); return err }},
		{"AlertSubscribeGetsAll", func(c *ctx.Context) error { _, err := AlertSubscribeGetsAll(c); return err }},
		{"RecordingRuleGet", func(c *ctx.Context) error { _, err := RecordingRuleGet(c, "id=?", 1); return err }},
		{"RecordingRuleEnabledGets", func(c *ctx.Context) error { _, err := RecordingRuleEnabledGets(c); return err }},
		{"RecordingRuleGetsByCluster", func(c *ctx.Context) error { _, err := RecordingRuleGetsByCluster(c); return err }},
		{"TaskTplGet", func(c *ctx.Context) error { _, err := TaskTplGet(c, "id=?", 1); return err }},
		{"TaskTplGetAll", func(c *ctx.Context) error { _, err := TaskTplGetAll(c); return err }},
		{"NotifyChannelGetsAll", func(c *ctx.Context) error { _, err := NotifyChannelGetsAll(c); return err }},
		{"NotifyChannelIdentsGet", func(c *ctx.Context) error { _, err := NotifyChannelIdentsGet(c, []int64{1}); return err }},
		{"NotifyChannelsGet", func(c *ctx.Context) error { _, err := NotifyChannelsGet(c, "id=?", 1); return err }},
		{"AlertCurEventGet", func(c *ctx.Context) error { _, err := AlertCurEventGet(c, "id=?", 1); return err }},
		{"AlertCurEventGetByRuleIdAndDsId", func(c *ctx.Context) error { _, err := AlertCurEventGetByRuleIdAndDsId(c, 1, 2); return err }},
		{"AlertNumbers", func(c *ctx.Context) error { _, err := AlertNumbers(c, []int64{1}); return err }},
		{"AlertCurEventGetMap", func(c *ctx.Context) error { _, err := AlertCurEventGetMap(c, "default"); return err }},
		{"TargetBusiGroupsGetAll", func(c *ctx.Context) error { _, err := TargetBusiGroupsGetAll(c); return err }},
		{"TargetGroupIdsGetByIdent", func(c *ctx.Context) error { _, err := TargetGroupIdsGetByIdent(c, "host"); return err }},
		{"TargetGroupIdsGetByIdents", func(c *ctx.Context) error { _, err := TargetGroupIdsGetByIdents(c, []string{"host"}); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &User{})
			failure := errors.New("r8d database query rejected")
			const callback = "r8d:reject_query"
			require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) { tx.AddError(failure) }))
			t.Cleanup(func() { c.DB.Callback().Query().Remove(callback) })
			require.ErrorIs(t, tc.call(c), failure)
		})
	}
}
