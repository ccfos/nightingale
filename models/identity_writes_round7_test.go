package models

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUserUpdate_PersistsOnlySelectedFields(t *testing.T) {
	c := pipelineContext(t, &User{})
	require.NoError(t, c.DB.Create(&User{Id: 1, Username: "alice", Nickname: "Original", Email: "old@example.test"}).Error)
	user := User{Id: 1, Username: "alice", Nickname: "Changed", Email: "new@example.test"}

	err := user.Update(c, "nickname")

	require.NoError(t, err)
	var stored User
	require.NoError(t, c.DB.First(&stored, 1).Error)
	require.Equal(t, "Changed", stored.Nickname)
	require.Equal(t, "old@example.test", stored.Email)
}

func TestUserUpdate_InvalidLocalUsernameLeavesProfileUnchanged(t *testing.T) {
	c := pipelineContext(t, &User{})
	require.NoError(t, c.DB.Create(&User{Id: 1, Username: "alice", Nickname: "Original"}).Error)
	user := User{Id: 1, Nickname: "Changed"}

	err := user.Update(c, "nickname")

	require.Error(t, err)
	var stored User
	require.NoError(t, c.DB.First(&stored, 1).Error)
	require.Equal(t, "Original", stored.Nickname)
}

func TestUserUpdate_ExternalIdentityCanUpdateSelectedProfileFields(t *testing.T) {
	c := pipelineContext(t, &User{})
	require.NoError(t, c.DB.Create(&User{Id: 1, Username: "external", Belong: "ldap"}).Error)
	user := User{Id: 1, Belong: "ldap", Nickname: "Directory name"}

	err := user.Update(c, "nickname")

	require.NoError(t, err)
	var stored User
	require.NoError(t, c.DB.First(&stored, 1).Error)
	require.Equal(t, "Directory name", stored.Nickname)
	require.Equal(t, "external", stored.Username)
}

func TestUserAddUserAndGroups_CreatesUserWithDeduplicatedMemberships(t *testing.T) {
	c := pipelineContext(t, &User{}, &UserGroupMember{})
	user := User{Username: "alice", TeamsLst: []int64{7, 8, 7}}

	err := user.AddUserAndGroups(c, true)

	require.NoError(t, err)
	require.Positive(t, user.Id)
	var stored User
	require.NoError(t, c.DB.First(&stored, user.Id).Error)
	require.Equal(t, "alice", stored.Username)
	groups, err := MyGroupIds(c, user.Id)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{7, 8}, groups)
}

func TestUserAddUserAndGroups_DuplicateUsernameDoesNotChangeMemberships(t *testing.T) {
	c := pipelineContext(t, &User{}, &UserGroupMember{})
	require.NoError(t, c.DB.Create(&User{Id: 1, Username: "alice"}).Error)
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 1, GroupId: 7}).Error)
	duplicate := User{Username: "alice", TeamsLst: []int64{8}}

	err := duplicate.AddUserAndGroups(c, true)

	require.ErrorContains(t, err, "failed to add user: Username already exists")
	groups, err := MyGroupIds(c, 1)
	require.NoError(t, err)
	require.Equal(t, []int64{7}, groups)
	var count int64
	require.NoError(t, c.DB.Model(&User{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestUserFindSameContact_ReturnsFirstNonemptyMatchingContact(t *testing.T) {
	for _, tc := range []struct {
		name  string
		user  User
		email string
		phone string
		want  string
	}{
		{
			name: "email wins", user: User{Email: "a@example.test", Phone: "123"},
			email: "a@example.test", phone: "123", want: "email",
		},
		{
			name: "phone", user: User{Email: "a@example.test", Phone: "123"},
			email: "other@example.test", phone: "123", want: "phone",
		},
		{name: "empty contacts", user: User{}},
		{name: "no match", user: User{Email: "a@example.test", Phone: "123"}, email: "other@example.test", phone: "456"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.user.FindSameContact(tc.email, tc.phone)

			require.Equal(t, tc.want, got)
		})
	}
}

func TestMyBusiGroupIds_ResolvesGroupsThroughMemberships(t *testing.T) {
	c := pipelineContext(t, &UserGroupMember{}, &BusiGroupMember{})
	require.NoError(t, c.DB.Create(&[]UserGroupMember{{UserId: 1, GroupId: 7}, {UserId: 2, GroupId: 8}}).Error)
	require.NoError(t, c.DB.Create(&[]BusiGroupMember{
		{BusiGroupId: 10, UserGroupId: 7},
		{BusiGroupId: 20, UserGroupId: 8},
	}).Error)

	ids, err := MyBusiGroupIds(c, 1)

	require.NoError(t, err)
	require.Equal(t, []int64{10}, ids)
}

func TestRoleOperationBind_InsertFailureRestoresPreviousPermissions(t *testing.T) {
	c := pipelineContext(t, &RoleOperation{})
	require.NoError(t, c.DB.Create(&RoleOperation{RoleName: "viewer", Operation: "read"}).Error)
	failure := errors.New("permission write rejected")
	const callback = "round7:reject_role_write"
	require.NoError(t, c.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "role_operation" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { c.DB.Callback().Create().Remove(callback) })

	err := RoleOperationBind(c, "viewer", []string{"write"})

	require.ErrorIs(t, err, failure)
	operations, err := OperationsOfRole(c, []string{"viewer"})
	require.NoError(t, err)
	require.Equal(t, []string{"read"}, operations)
}

func TestUserDelByIds_MembershipDeleteFailurePreservesUsers(t *testing.T) {
	c := pipelineContext(t, &User{}, &UserGroupMember{})
	require.NoError(t, c.DB.Create(&User{Id: 1, Username: "alice"}).Error)
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 1, GroupId: 7}).Error)
	failure := errors.New("membership deletion rejected")
	const callback = "round7:reject_membership_delete"
	require.NoError(t, c.DB.Callback().Delete().Before("gorm:delete").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_group_member" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { c.DB.Callback().Delete().Remove(callback) })

	err := UserDelByIds(c, []int64{1})

	require.ErrorIs(t, err, failure)
	groups, err := MyGroupIds(c, 1)
	require.NoError(t, err)
	require.Equal(t, []int64{7}, groups)
	var user User
	require.NoError(t, c.DB.First(&user, 1).Error)
	require.Equal(t, "alice", user.Username)
}

func TestUserBusiGroups_MembershipReadFailureReturnsError(t *testing.T) {
	c := pipelineContext(t, &UserGroupMember{}, &BusiGroupMember{}, &BusiGroup{})
	failure := errors.New("membership read rejected")
	const callback = "round7:reject_membership_read"
	require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "user_group_member" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { c.DB.Callback().Query().Remove(callback) })

	groups, err := (&User{Id: 1}).BusiGroups(c, 10, "")

	require.ErrorIs(t, err, failure)
	require.ErrorContains(t, err, "failed to get MyGroupIds")
	require.Empty(t, groups)
}

func TestUserBusiGroups_PermissionReadFailureReturnsError(t *testing.T) {
	c := pipelineContext(t, &UserGroupMember{}, &BusiGroupMember{}, &BusiGroup{})
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 1, GroupId: 7}).Error)
	failure := errors.New("permission read rejected")
	const callback = "round7:reject_permission_read"
	require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "busi_group_member" {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { c.DB.Callback().Query().Remove(callback) })

	groups, err := (&User{Id: 1}).BusiGroups(c, 10, "")

	require.ErrorIs(t, err, failure)
	require.ErrorContains(t, err, "failed to get BusiGroupIds")
	require.Empty(t, groups)
}
