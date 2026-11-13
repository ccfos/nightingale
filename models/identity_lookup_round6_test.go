package models

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInitRoot_HashesLegacyPasswordOnce(t *testing.T) {
	c := permissionContext(t, &User{}, &Configs{})
	require.NoError(t, c.DB.Create(&User{Id: 1, Username: "root", Password: "legacy", Roles: AdminRole}).Error)

	changed := InitRoot(c)

	require.True(t, changed)
	stored, err := UserGetByUsername(c, "root")
	require.NoError(t, err)
	salt, err := ConfigsGet(c, SALT)
	require.NoError(t, err)
	require.NotEmpty(t, salt)
	expected, err := CryptoPass(c, "legacy")
	require.NoError(t, err)
	require.Equal(t, expected, stored.Password)
	require.False(t, InitRoot(c))
	unchanged, err := UserGetByUsername(c, "root")
	require.NoError(t, err)
	require.Equal(t, stored.Password, unchanged.Password)
}

func TestInitRoot_MissingRootLeavesUsersUnchanged(t *testing.T) {
	c := permissionContext(t, &User{})
	require.NoError(t, c.DB.Create(&User{Id: 1, Username: "alice", Password: "original"}).Error)

	changed := InitRoot(c)

	require.False(t, changed)
	stored, err := UserGetByUsername(c, "alice")
	require.NoError(t, err)
	require.Equal(t, "original", stored.Password)
}

func TestUser_BusiGroupsAdminCanFindGroupsByHostIdent(t *testing.T) {
	c := permissionContext(t, &BusiGroup{}, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]BusiGroup{{Id: 7, Name: "ops"}, {Id: 8, Name: "database"}}).Error)
	require.NoError(t, c.DB.Create(&Target{Ident: "node-1"}).Error)
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{Id: 1, TargetIdent: "node-1", GroupId: 7},
		{Id: 2, TargetIdent: "node-1", GroupId: 8},
	}).Error)
	admin := User{RolesLst: []string{AdminRole}}

	groups, err := admin.BusiGroups(c, 10, "node-1")

	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Equal(t, "database", groups[0].Name)
	require.Equal(t, "ops", groups[1].Name)
}

func TestUser_BusiGroupsUnknownHostReturnsEmpty(t *testing.T) {
	c := permissionContext(t, &BusiGroup{}, &Target{})
	admin := User{RolesLst: []string{AdminRole}}

	groups, err := admin.BusiGroups(c, 10, "missing-host")

	require.NoError(t, err)
	require.Empty(t, groups)
}

func TestUser_UserGroupsAdminCanFindMembershipByUsername(t *testing.T) {
	c := permissionContext(t, &User{}, &UserGroup{}, &UserGroupMember{})
	require.NoError(t, c.DB.Create(&User{Id: 1, Username: "alice"}).Error)
	require.NoError(t, c.DB.Create(&[]UserGroup{{Id: 7, Name: "ops"}, {Id: 8, Name: "unrelated"}}).Error)
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 1, GroupId: 7}).Error)
	admin := User{RolesLst: []string{AdminRole}}

	groups, err := admin.UserGroups(c, 10, "alice")

	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, int64(7), groups[0].Id)
}

func TestUser_UserGroupsUnknownUsernameReturnsEmpty(t *testing.T) {
	c := permissionContext(t, &User{}, &UserGroup{})
	admin := User{RolesLst: []string{AdminRole}}

	groups, err := admin.UserGroups(c, 10, "missing-user")

	require.NoError(t, err)
	require.Empty(t, groups)
}

func TestCleanupExpiredTokens_DeletesOnlyExpiredExpiringTokens(t *testing.T) {
	c := permissionContext(t, &SourceToken{})
	require.NoError(t, c.DB.Create(&[]SourceToken{
		{Id: 1, Token: "expired", ExpireAt: 1},
		{Id: 2, Token: "permanent", ExpireAt: 0},
		{Id: 3, Token: "future", ExpireAt: math.MaxInt64},
	}).Error)

	deleted, err := CleanupExpiredTokens(c)

	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	rows, err := SourceTokenGets(c, "", "")
	require.NoError(t, err)
	tokens := make([]string, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, row.Token)
		require.False(t, row.IsExpired())
	}
	require.ElementsMatch(t, []string{"permanent", "future"}, tokens)
	require.True(t, (&SourceToken{ExpireAt: 1}).IsExpired())
}

func TestBuiltinCateDelete_RestrictsDeletionToOwnerAndName(t *testing.T) {
	c := permissionContext(t, &BuiltinCate{})
	for _, category := range []BuiltinCate{
		{Id: 1, Name: "database", UserId: 7},
		{Id: 2, Name: "database", UserId: 8},
		{Id: 3, Name: "network", UserId: 7},
	} {
		require.NoError(t, category.Create(c))
	}

	err := BuiltinCateDelete(c, "database", 7)

	require.NoError(t, err)
	owned, err := BuiltinCateGetByUserId(c, 7)
	require.NoError(t, err)
	require.Len(t, owned, 1)
	require.Equal(t, int64(3), owned["network"].Id)
	other, err := BuiltinCateGetByUserId(c, 8)
	require.NoError(t, err)
	require.Equal(t, int64(2), other["database"].Id)
}

func TestUserGetsBySso_MapsOnlyUsersFromSelectedIdentityProvider(t *testing.T) {
	c := permissionContext(t, &User{})
	require.NoError(t, c.DB.Create(&[]User{
		{Id: 1, Username: "alice", Belong: "oidc", Nickname: "Alice"},
		{Id: 2, Username: "bob", Belong: "oidc", Nickname: "Bob"},
		{Id: 3, Username: "charlie", Belong: "ldap"},
	}).Error)

	users, err := UserGetsBySso(c, "oidc")

	require.NoError(t, err)
	require.Len(t, users, 2)
	require.Equal(t, "Alice", users["alice"].Nickname)
	require.Equal(t, "Bob", users["bob"].Nickname)
	require.NotSame(t, users["alice"], users["bob"])
	require.NotContains(t, users, "charlie")
}
