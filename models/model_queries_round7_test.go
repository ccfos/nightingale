package models

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetAlertRuleIdsByTaskId_FindsTemplatesAndLegacyCallbacks(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	require.NoError(t, c.DB.Create(&[]AlertRule{
		{Id: 1, RuleConfig: `{"task_tpls":[{"tpl_id":7}]}`},
		{Id: 2, Callbacks: "{ibex}/7"},
		{Id: 3, RuleConfig: `{"task_tpls":[{"tpl_id":9}]}`, Callbacks: "{ibex}/9"},
	}).Error)

	ids, err := GetAlertRuleIdsByTaskId(c, 7)

	require.NoError(t, err)
	require.ElementsMatch(t, []int64{1, 2}, ids)
}

func TestGetAlertRuleIdsByTaskId_ReturnsEmptySelectionForUnusedTemplate(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	require.NoError(t, c.DB.Create(&AlertRule{Id: 1, RuleConfig: `{"task_tpls":[{"tpl_id":7}]}`}).Error)

	ids, err := GetAlertRuleIdsByTaskId(c, 9)

	require.NoError(t, err)
	require.Empty(t, ids)
}

func TestGetAlertRuleIdsByTaskId_PropagatesDatabaseFailure(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	failure := errors.New("rule query rejected")
	const callback = "round7:reject_task_rule_query"
	require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		tx.AddError(failure)
	}))
	t.Cleanup(func() { c.DB.Callback().Query().Remove(callback) })

	ids, err := GetAlertRuleIdsByTaskId(c, 7)

	require.ErrorIs(t, err, failure)
	require.Empty(t, ids)
}

func TestUserDelByIds_RemovesSelectedUsersAndMemberships(t *testing.T) {
	c := pipelineContext(t, &User{}, &UserGroupMember{})
	require.NoError(t, c.DB.Create(&[]User{{Id: 1, Username: "alice"}, {Id: 2, Username: "bob"}}).Error)
	require.NoError(t, c.DB.Create(&[]UserGroupMember{{UserId: 1, GroupId: 7}, {UserId: 2, GroupId: 7}}).Error)

	err := UserDelByIds(c, []int64{1})

	require.NoError(t, err)
	var users []User
	require.NoError(t, c.DB.Find(&users).Error)
	require.Len(t, users, 1)
	require.Equal(t, "bob", users[0].Username)
	var members []UserGroupMember
	require.NoError(t, c.DB.Find(&members).Error)
	require.Equal(t, []UserGroupMember{{UserId: 2, GroupId: 7}}, members)
}

func TestUserDelByIds_UserDeleteFailureRollsBackMembershipRemoval(t *testing.T) {
	c := pipelineContext(t, &User{}, &UserGroupMember{})
	require.NoError(t, c.DB.Create(&User{Id: 1, Username: "alice"}).Error)
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 1, GroupId: 7}).Error)
	failure := errors.New("user deletion rejected")
	const callback = "round7:reject_user_delete"
	require.NoError(t, c.DB.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { c.DB.Callback().Delete().Remove(callback) })

	err := UserDelByIds(c, []int64{1})

	require.ErrorIs(t, err, failure)
	groups, err := MyGroupIds(c, 1)
	require.NoError(t, err)
	require.Equal(t, []int64{7}, groups)
	var count int64
	require.NoError(t, c.DB.Model(&User{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestDataRespString_RendersSeriesAndSamples(t *testing.T) {
	series := DataResp{
		Ref: "A", Metric: model.Metric{"__name__": "cpu", "host": "node"}, Labels: "host=node",
		Values: [][]float64{{100, 2}, {200, 3}}, Query: "cpu",
	}

	rendered := series.String()

	require.Equal(t, `Ref: A Metric: cpu{host="node"} Labels: host=node Values:   [100, 2]   [200, 3] Query: cpu `, rendered)
}

func TestDataRespMetricName_ReturnsOnlyMetricNameLabel(t *testing.T) {
	series := DataResp{Metric: model.Metric{"__name__": "cpu", "host": "node"}}

	name := series.MetricName()

	require.Equal(t, "cpu", name)
	require.Empty(t, (&DataResp{Metric: model.Metric{"host": "node"}}).MetricName())
}

func TestDataRespLabelsString_UsesCanonicalMetricOrdering(t *testing.T) {
	series := DataResp{Metric: model.Metric{"zone": "west", "host": "node", "__name__": "cpu"}}

	labels := series.LabelsString()

	require.Equal(t, `cpu{host="node", zone="west"}`, labels)
}

func TestGetNotifyTpl_LoadsOnlyTemplateFiles(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "n9e-tpl-round7-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	require.NoError(t, os.WriteFile(filepath.Join(dir, "custom.tpl"), []byte("{{.RuleName}}"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0600))

	templates := getNotifyTpl(dir)

	require.Equal(t, map[string]string{"custom": "{{.RuleName}}"}, templates)
}
