package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBusiGroupAdd_ValidatesMembersBeforeCreatingGroup(t *testing.T) {
	c := pipelineContext(t, &BusiGroup{}, &BusiGroupMember{}, &UserGroup{})
	members := []BusiGroupMember{{UserGroupId: 99, PermFlag: "rw"}}

	err := BusiGroupAdd(c, "Operations", 0, "ignored", members, "author")

	require.EqualError(t, err, "Some UserGroup id not exists")
	groups, err := BusiGroupGetAll(c)
	require.NoError(t, err)
	require.Empty(t, groups)
}

func TestBusiGroupAdd_StoresPermissionsAndClearsDisabledLabel(t *testing.T) {
	c := pipelineContext(t, &BusiGroup{}, &BusiGroupMember{}, &UserGroup{})
	require.NoError(t, c.DB.Create(&UserGroup{Id: 7, Name: "Operators"}).Error)

	err := BusiGroupAdd(c, "Operations", 0, "ignored", []BusiGroupMember{{UserGroupId: 7, PermFlag: "rw"}}, "author")

	require.NoError(t, err)
	group, err := BusiGroupGet(c, "name = ?", "Operations")
	require.NoError(t, err)
	require.NotNil(t, group)
	require.Empty(t, group.LabelValue)
	require.Equal(t, "author", group.CreateBy)
	require.NoError(t, group.FillUserGroups(c))
	require.Len(t, group.UserGroups, 1)
	require.Equal(t, "Operators", group.UserGroups[0].UserGroup.Name)
	require.Equal(t, "rw", group.UserGroups[0].PermFlag)
	byID, err := BusiGroupGetMap(c)
	require.NoError(t, err)
	require.Equal(t, "Operations", byID[group.Id].Name)
	selected, err := BusiGroupGetByIds(c, []int64{group.Id, 99})
	require.NoError(t, err)
	require.Len(t, selected, 1)
	missing, err := BusiGroupGetById(c, 99)
	require.NoError(t, err)
	require.Nil(t, missing)
}

func TestBusiGroupAdd_RejectsDuplicateNameAndEnabledLabel(t *testing.T) {
	c := pipelineContext(t, &BusiGroup{}, &BusiGroupMember{}, &UserGroup{})
	require.NoError(t, BusiGroupAdd(c, "Operations", 1, "ops", nil, "author"))

	err := BusiGroupAdd(c, "Operations", 0, "", nil, "other")

	require.EqualError(t, err, "BusiGroup already exists")
	require.EqualError(t, BusiGroupAdd(c, "Another", 1, "ops", nil, "other"), "BusiGroup already exists")
	groups, err := BusiGroupGetAll(c)
	require.NoError(t, err)
	require.Len(t, groups, 1)
}

func TestBusiGroupUpdate_RejectsConflictsAndClearsDisabledLabel(t *testing.T) {
	c := pipelineContext(t, &BusiGroup{})
	group := BusiGroup{Id: 1, Name: "Operations", LabelEnable: 1, LabelValue: "ops"}
	require.NoError(t, c.DB.Create(&group).Error)
	require.NoError(t, c.DB.Create(&BusiGroup{Id: 2, Name: "Platform", LabelEnable: 1, LabelValue: "platform"}).Error)

	require.NoError(t, group.Update(c, "Operations", 1, "ops", "ignored"))
	require.EqualError(t, group.Update(c, "Platform", 1, "ops", "editor"), "BusiGroup already exists")
	require.EqualError(t, group.Update(c, "Renamed", 1, "platform", "editor"), "BusiGroup already exists")
	require.NoError(t, group.Update(c, "Renamed", 0, "ignored", "editor"))

	stored, err := BusiGroupGetById(c, 1)
	require.NoError(t, err)
	require.Equal(t, "Renamed", stored.Name)
	require.Zero(t, stored.LabelEnable)
	require.Empty(t, stored.LabelValue)
	require.Equal(t, "editor", stored.UpdateBy)
}

func TestBusiGroupMemberAdd_UpsertsPermissionWithoutDuplicatingMembership(t *testing.T) {
	c := pipelineContext(t, &BusiGroupMember{})
	member := BusiGroupMember{BusiGroupId: 1, UserGroupId: 7, PermFlag: "ro"}
	require.NoError(t, BusiGroupMemberAdd(c, member))
	require.NoError(t, BusiGroupMemberAdd(c, member))

	member.PermFlag = "rw"
	require.NoError(t, BusiGroupMemberAdd(c, member))

	members, err := BusiGroupMemberGetsByBusiGroupId(c, 1)
	require.NoError(t, err)
	require.Equal(t, []BusiGroupMember{member}, members)
	ids, err := UserGroupIdsOfBusiGroup(c, 1, "rw")
	require.NoError(t, err)
	require.Equal(t, []int64{7}, ids)
	ids, err = BusiGroupIds(c, []int64{7}, "ro")
	require.NoError(t, err)
	require.Empty(t, ids)
}

func TestBusiGroupDelMembers_RetainsAtLeastOneTeam(t *testing.T) {
	c := pipelineContext(t, &BusiGroup{}, &BusiGroupMember{})
	group := BusiGroup{Id: 1, Name: "Operations"}
	require.NoError(t, c.DB.Create(&group).Error)
	first := BusiGroupMember{BusiGroupId: 1, UserGroupId: 7, PermFlag: "rw"}
	second := BusiGroupMember{BusiGroupId: 1, UserGroupId: 8, PermFlag: "ro"}
	require.NoError(t, group.AddMembers(c, []BusiGroupMember{first, second}, "author"))

	require.NoError(t, group.DelMembers(c, []BusiGroupMember{second}, "editor"))
	err := group.DelMembers(c, []BusiGroupMember{first}, "editor")

	require.EqualError(t, err, "the business group must retain at least one team")
	members, err := BusiGroupMemberGetsByBusiGroupId(c, 1)
	require.NoError(t, err)
	require.Equal(t, []BusiGroupMember{first}, members)
	stored, err := BusiGroupGetById(c, 1)
	require.NoError(t, err)
	require.Equal(t, "editor", stored.UpdateBy)
}

func TestBusiGroupDel_RejectsGroupsWithAlertRules(t *testing.T) {
	c := pipelineContext(t, &BusiGroup{}, &AlertRule{})
	group := BusiGroup{Id: 1, Name: "Operations"}
	require.NoError(t, c.DB.Create(&group).Error)
	require.NoError(t, c.DB.Create(&AlertRule{Id: 1, GroupId: 1}).Error)

	err := group.Del(c)

	require.EqualError(t, err, "Some alert rules still in the BusiGroup")
	stored, err := BusiGroupGetById(c, 1)
	require.NoError(t, err)
	require.NotNil(t, stored)
}

func TestBusiGroupDel_RemovesMembershipsAndOrphanedCurrentEvents(t *testing.T) {
	c := pipelineContext(t, &BusiGroup{}, &BusiGroupMember{}, &AlertCurEvent{}, &AlertRule{}, &AlertMute{},
		&AlertSubscribe{}, &Board{}, &Target{}, &RecordingRule{}, &TaskTpl{}, &TargetBusiGroup{})
	group := BusiGroup{Id: 1, Name: "Operations"}
	require.NoError(t, c.DB.Create(&group).Error)
	require.NoError(t, c.DB.Create(&[]BusiGroupMember{
		{BusiGroupId: 1, UserGroupId: 7},
		{BusiGroupId: 2, UserGroupId: 8},
	}).Error)
	require.NoError(t, c.DB.Create(&[]AlertCurEvent{{Id: 1, GroupId: 1}, {Id: 2, GroupId: 2}}).Error)

	require.NoError(t, group.Del(c))

	stored, err := BusiGroupGetById(c, 1)
	require.NoError(t, err)
	require.Nil(t, stored)
	members, err := BusiGroupMemberGets(c, "1 = 1")
	require.NoError(t, err)
	require.Equal(t, []BusiGroupMember{{BusiGroupId: 2, UserGroupId: 8}}, members)
	ids, err := AlertCurEventIds(c)
	require.NoError(t, err)
	require.Equal(t, []int64{2}, ids)
}
