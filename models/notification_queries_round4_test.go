package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMessageTemplateGets_CombinesExactIdentityFilters(t *testing.T) {
	c := pipelineContext(t, &MessageTemplate{})
	require.NoError(t, c.DB.Create(&[]MessageTemplate{
		{ID: 1, Name: "CPU", Ident: "cpu", Content: map[string]string{"text": "CPU high"}},
		{ID: 2, Name: "CPU", Ident: "cpu_other"},
		{ID: 3, Name: "Memory", Ident: "memory"},
	}).Error)
	cases := []struct {
		name  string
		id    int64
		title string
		ident string
		want  []int64
	}{
		{name: "all", want: []int64{1, 2, 3}},
		{name: "name", title: "CPU", want: []int64{1, 2}},
		{name: "ident", ident: "cpu", want: []int64{1}},
		{name: "combined", id: 1, title: "CPU", ident: "cpu", want: []int64{1}},
		{name: "mismatch", id: 1, title: "Memory", want: []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := MessageTemplateGets(c, tc.id, tc.title, tc.ident)

			require.NoError(t, err)
			ids := make([]int64, 0, len(rows))
			for _, row := range rows {
				ids = append(ids, row.ID)
			}
			require.ElementsMatch(t, tc.want, ids)
		})
	}
}

func TestMessageTemplatesGetBy_FiltersChannelsAndOrdersByWeightThenID(t *testing.T) {
	c := pipelineContext(t, &MessageTemplate{})
	require.NoError(t, c.DB.Create(&[]MessageTemplate{
		{ID: 1, Ident: "mail_zh", NotifyChannelIdent: "mail", Weight: 10},
		{ID: 2, Ident: "mail_en", NotifyChannelIdent: "mail", Weight: 10},
		{ID: 3, Ident: "chat", NotifyChannelIdent: "chat", Weight: 1},
		{ID: 4, Ident: "mail_custom", NotifyChannelIdent: "mail", Weight: 0, UserGroupIds: []int64{7}},
	}).Error)

	rows, err := MessageTemplatesGetBy(c, []string{"mail"})

	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Equal(t, []int64{4, 1, 2}, []int64{rows[0].ID, rows[1].ID, rows[2].ID})
	require.Equal(t, []int64{7}, rows[0].UserGroupIds)
	require.Equal(t, []int64{}, rows[1].UserGroupIds)
	all, err := MessageTemplatesGetBy(c, nil)
	require.NoError(t, err)
	require.Len(t, all, 4)
	require.Equal(t, int64(4), all[0].ID)
}

func TestNotifyChannelGets_CombinesIdentityAndEnableFilters(t *testing.T) {
	c := pipelineContext(t, &NotifyChannelConfig{})
	require.NoError(t, c.DB.Create(&[]NotifyChannelConfig{
		{ID: 1, Name: "mail", Ident: "smtp", Enable: true},
		{ID: 2, Name: "mail", Ident: "smtp_disabled", Enable: false},
		{ID: 3, Name: "chat", Ident: "chat", Enable: true},
	}).Error)
	cases := []struct {
		name    string
		id      int64
		title   string
		ident   string
		enabled int
		want    []int64
	}{
		{name: "all", enabled: -1, want: []int64{1, 2, 3}},
		{name: "enabled", enabled: 1, want: []int64{1, 3}},
		{name: "disabled", enabled: 0, want: []int64{2}},
		{name: "combined", id: 1, title: "mail", ident: "smtp", enabled: 1, want: []int64{1}},
		{name: "mismatch", title: "chat", enabled: 0, want: []int64{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := NotifyChannelGets(c, tc.id, tc.title, tc.ident, tc.enabled)

			require.NoError(t, err)
			ids := make([]int64, 0, len(rows))
			for _, row := range rows {
				ids = append(ids, row.ID)
			}
			require.ElementsMatch(t, tc.want, ids)
		})
	}
}

func TestNotifyRulesGet_SortsNamesAndNormalizesNullableArrays(t *testing.T) {
	c := pipelineContext(t, &NotifyRule{})
	require.NoError(t, c.DB.Create(&[]NotifyRule{
		{ID: 1, Name: "zeta", Enable: true},
		{ID: 2, Name: "alpha", Enable: false},
		{ID: 3, Name: "beta", Enable: true, UserGroupIds: []int64{7}, NotifyConfigs: []NotifyConfig{{ChannelID: 8}}},
	}).Error)

	rows, err := NotifyRulesGet(c, "enable = ?", true)

	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, int64(3), rows[0].ID)
	require.Equal(t, []int64{7}, rows[0].UserGroupIds)
	require.Equal(t, int64(8), rows[0].NotifyConfigs[0].ChannelID)
	require.Equal(t, []int64{}, rows[1].UserGroupIds)
	require.Equal(t, []NotifyConfig{}, rows[1].NotifyConfigs)
	all, err := NotifyRulesGet(c, "")
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.Equal(t, "alpha", all[0].Name)
	missing, err := NotifyRuleGet(c, "id = ?", 99)
	require.NoError(t, err)
	require.Nil(t, missing)
}

type round4ChannelUsage struct{ channelID int64 }

func (checker round4ChannelUsage) IfUsed(rule *NotifyRule) bool {
	for _, config := range rule.NotifyConfigs {
		if config.ChannelID == checker.channelID {
			return true
		}
	}
	return false
}

func TestUsedByNotifyRule_ReturnsOnlyRulesMatchingChecker(t *testing.T) {
	c := pipelineContext(t, &NotifyRule{})
	require.NoError(t, c.DB.Create(&[]NotifyRule{
		{ID: 1, Name: "used", NotifyConfigs: []NotifyConfig{{ChannelID: 7}, {ChannelID: 8}}},
		{ID: 2, Name: "unused", NotifyConfigs: []NotifyConfig{{ChannelID: 8}}},
		{ID: 3, Name: "empty"},
	}).Error)

	ids, err := UsedByNotifyRule(c, round4ChannelUsage{channelID: 7})

	require.NoError(t, err)
	require.Equal(t, []int64{1}, ids)
}
