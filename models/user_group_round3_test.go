package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserGroupVerify_RejectsUnsafeNameAndNote(t *testing.T) {
	cases := []struct {
		name    string
		group   UserGroup
		message string
	}{
		{name: "name", group: UserGroup{Name: "<script>"}, message: "Name has invalid characters"},
		{name: "note", group: UserGroup{Name: "ops", Note: "<script>"}, message: "Note has invalid characters"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.group.Verify()

			require.EqualError(t, err, tc.message)
		})
	}
}

func TestUserGroupAdd_RejectsDuplicateName(t *testing.T) {
	c := pipelineContext(t, &UserGroup{})
	group := UserGroup{Name: "ops", CreateBy: "alice"}
	require.NoError(t, group.Add(c))
	duplicate := UserGroup{Name: "ops"}

	err := duplicate.Add(c)

	require.EqualError(t, err, "UserGroup already exists")
	rows, err := UserGroupGetAll(c)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "alice", rows[0].CreateBy)
	require.Equal(t, rows[0].CreateAt, rows[0].UpdateAt)
}

func TestUserGroupUpdate_WritesOnlySelectedFields(t *testing.T) {
	c := pipelineContext(t, &UserGroup{})
	group := UserGroup{Id: 1, Name: "ops", Note: "old note", CreateBy: "alice"}
	require.NoError(t, c.DB.Create(&group).Error)
	group.Name = "renamed"
	group.Note = "new note"
	group.CreateBy = "eve"

	require.NoError(t, group.Update(c, "name", "note"))

	got, err := UserGroupGetById(c, 1)
	require.NoError(t, err)
	require.Equal(t, "renamed", got.Name)
	require.Equal(t, "new note", got.Note)
	require.Equal(t, "alice", got.CreateBy)
}

func TestUserGroupAddMembers_SkipsMissingUsersAndDuplicateMemberships(t *testing.T) {
	c := pipelineContext(t, &User{}, &UserGroup{}, &UserGroupMember{})
	require.NoError(t, c.DB.Create(&[]User{{Id: 1, Username: "alice"}, {Id: 2, Username: "bob"}}).Error)
	group := UserGroup{Id: 7, Name: "ops"}
	require.NoError(t, c.DB.Create(&group).Error)
	require.NoError(t, c.DB.Create(&UserGroupMember{GroupId: 8, UserId: 1}).Error)

	require.NoError(t, group.AddMembers(c, []int64{1, 1, 2, 99}))

	ids, err := MemberIds(c, 7)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{1, 2}, ids)
	require.NoError(t, group.DelMembers(c, []int64{1}))
	ids, err = MemberIds(c, 7)
	require.NoError(t, err)
	require.Equal(t, []int64{2}, ids)
	other, err := MemberIds(c, 8)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, other)
	require.NoError(t, group.DelMembers(c, nil))
}

func TestUserGroupDel_RemovesOnlyItsMemberships(t *testing.T) {
	c := pipelineContext(t, &UserGroup{}, &UserGroupMember{})
	groups := []UserGroup{{Id: 7, Name: "ops"}, {Id: 8, Name: "dev"}}
	require.NoError(t, c.DB.Create(&groups).Error)
	require.NoError(t, c.DB.Create(&[]UserGroupMember{{GroupId: 7, UserId: 1}, {GroupId: 8, UserId: 1}}).Error)

	require.NoError(t, groups[0].Del(c))

	got, err := UserGroupGetById(c, 7)
	require.NoError(t, err)
	require.Nil(t, got)
	rows, err := UserGroupMemberGetAll(c)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(8), rows[0].GroupId)
	groupsLeft, err := UserGroupGetAll(c)
	require.NoError(t, err)
	require.Len(t, groupsLeft, 1)
	require.Equal(t, int64(8), groupsLeft[0].Id)
}

func TestUserGroupMemberSync_MergesOrReplacesOnlySelectedUsersTeams(t *testing.T) {
	cases := []struct {
		name     string
		incoming []int64
		cover    bool
		want     []int64
	}{
		{name: "merge", incoming: []int64{2, 3, 3}, want: []int64{1, 2, 3}},
		{name: "replace", incoming: []int64{2, 3, 3}, cover: true, want: []int64{2, 3}},
		{name: "empty merge", want: []int64{1, 2}},
		{name: "empty replacement", cover: true},
		{name: "unchanged", incoming: []int64{1, 2}, cover: true, want: []int64{1, 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &UserGroupMember{})
			require.NoError(t, c.DB.Create(&[]UserGroupMember{
				{GroupId: 1, UserId: 7}, {GroupId: 2, UserId: 7}, {GroupId: 1, UserId: 8},
			}).Error)
			user := User{Id: 7, TeamsLst: tc.incoming}

			require.NoError(t, UserGroupMemberSyncByUser(c, &user, tc.cover))

			got, err := MyGroupIds(c, 7)
			require.NoError(t, err)
			require.ElementsMatch(t, tc.want, got)
			other, err := MyGroupIds(c, 8)
			require.NoError(t, err)
			require.Equal(t, []int64{1}, other)
		})
	}
}
