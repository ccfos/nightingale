package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlertSubscribe_AddPersistsSerializedNotificationSelection(t *testing.T) {
	c := permissionContext(t, &AlertSubscribe{})
	sub := AlertSubscribe{
		Name: "CPU subscribers", GroupId: 7, SeveritiesJson: []int{1, 2}, DatasourceIdsJson: []int64{9},
		NotifyVersion: 1, NotifyRuleIds: []int64{11}, CreateBy: "alice",
		ExtraConfigJson: map[string]interface{}{"ticket": true},
	}

	err := sub.Add(c)

	require.NoError(t, err)
	stored, err := AlertSubscribeGet(c, "id = ?", sub.Id)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.JSONEq(t, "[9]", stored.DatasourceIds)
	require.JSONEq(t, "[1,2]", stored.Severities)
	require.JSONEq(t, `{"ticket":true}`, stored.ExtraConfig)
	require.Equal(t, []int64{11}, stored.NotifyRuleIds)
	require.Equal(t, sub.CreateAt, stored.UpdateAt)
	require.Positive(t, stored.CreateAt)
}

func TestAlertSubscribe_AddRejectsMissingSeverityWithoutInserting(t *testing.T) {
	c := permissionContext(t, &AlertSubscribe{})
	sub := AlertSubscribe{Name: "invalid"}

	err := sub.Add(c)

	require.EqualError(t, err, "severities is required")
	rows, err := AlertSubscribeGets(c, 0)
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestAlertSubscribe_UpdateSerializesSelectedFieldsOnly(t *testing.T) {
	c := permissionContext(t, &AlertSubscribe{})
	sub := AlertSubscribe{Id: 1, Name: "original", GroupId: 7, Severities: "[1]", CreateBy: "alice"}
	require.NoError(t, c.DB.Create(&sub).Error)
	sub.Name, sub.CreateBy = "updated", "ignored"
	sub.SeveritiesJson = []int{2, 3}

	err := sub.Update(c, "name", "severities")

	require.NoError(t, err)
	stored, err := AlertSubscribeGet(c, "id = ?", sub.Id)
	require.NoError(t, err)
	require.Equal(t, "updated", stored.Name)
	require.JSONEq(t, "[2,3]", stored.Severities)
	require.Equal(t, "alice", stored.CreateBy)
}

func TestAlertSubscribe_UpdateRejectsInvalidNotificationRules(t *testing.T) {
	c := permissionContext(t, &AlertSubscribe{})
	sub := AlertSubscribe{Id: 1, Name: "original", Severities: "[1]"}
	require.NoError(t, c.DB.Create(&sub).Error)
	sub.Name, sub.NotifyVersion, sub.SeveritiesJson = "invalid", 1, []int{1}

	err := sub.Update(c, "name")

	require.EqualError(t, err, "no notify rules selected")
	stored, err := AlertSubscribeGet(c, "id = ?", sub.Id)
	require.NoError(t, err)
	require.Equal(t, "original", stored.Name)
}

func TestAlertSubscribeGets_FiltersBusinessGroupAndOrdersNewestFirst(t *testing.T) {
	c := permissionContext(t, &AlertSubscribe{})
	require.NoError(t, c.DB.Create(&[]AlertSubscribe{
		{Id: 1, Name: "old", GroupId: 7},
		{Id: 2, Name: "other", GroupId: 8},
		{Id: 3, Name: "new", GroupId: 7},
	}).Error)

	rows, err := AlertSubscribeGets(c, 7)

	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, []int64{3, 1}, []int64{rows[0].Id, rows[1].Id})
	rows, err = AlertSubscribeGetsByBGIds(c, []int64{8})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(2), rows[0].Id)
	rows, err = AlertSubscribeGetsByBGIds(c, nil)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	require.Equal(t, int64(3), rows[0].Id)
}

func TestAlertSubscribeGetsByService_DecodesStoredSelections(t *testing.T) {
	c := permissionContext(t, &AlertSubscribe{})
	require.NoError(t, c.DB.Create(&AlertSubscribe{
		Id: 1, DatasourceIds: "[9]", Severities: "[2]", Webhooks: `["http://127.0.0.1/hook"]`, ExtraConfig: "{}",
	}).Error)

	rows, err := AlertSubscribeGetsByService(c)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, []int64{9}, rows[0].DatasourceIdsJson)
	require.Equal(t, []int{2}, rows[0].SeveritiesJson)
	require.Equal(t, []string{"http://127.0.0.1/hook"}, rows[0].WebhooksJson)
}

func TestAlertSubscribeDel_PreservesUnselectedSubscriptions(t *testing.T) {
	c := permissionContext(t, &AlertSubscribe{})
	require.NoError(t, c.DB.Create(&[]AlertSubscribe{{Id: 1}, {Id: 2}}).Error)

	require.NoError(t, AlertSubscribeDel(c, nil))
	require.NoError(t, AlertSubscribeDel(c, []int64{2}))

	rows, err := AlertSubscribeGetsByBGIds(c, nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(1), rows[0].Id)
}
