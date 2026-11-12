package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Searching by host ident regressed to exposing host groups outside the user's memberships.
func TestUserBusiGroupsHostIdentPermissionsRegression(t *testing.T) {
	c := permissionContext(t, &BusiGroup{}, &BusiGroupMember{}, &UserGroupMember{}, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&[]BusiGroup{{Id: 7, Name: "allowed"}, {Id: 8, Name: "restricted"}}).Error)
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 1, GroupId: 20}).Error)
	require.NoError(t, c.DB.Create(&BusiGroupMember{BusiGroupId: 7, UserGroupId: 20}).Error)
	require.NoError(t, c.DB.Create(&Target{Ident: "host-permissions"}).Error)
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{TargetIdent: "host-permissions", GroupId: 7},
		{TargetIdent: "host-permissions", GroupId: 8},
	}).Error)

	groups, err := (&User{Id: 1}).BusiGroups(c, 10, "host-permissions")

	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, int64(7), groups[0].Id)
}
