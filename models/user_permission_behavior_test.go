package models

import (
	"context"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func permissionContext(t *testing.T, tables ...interface{}) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(tables...))
	return ctx.NewContext(context.Background(), db, true)
}

func TestUser_CheckPermUsesRoleOperationBindings(t *testing.T) {
	c := permissionContext(t, &RoleOperation{})
	require.NoError(t, c.DB.Create(&[]RoleOperation{
		{RoleName: "viewer", Operation: "rule:read"}, {RoleName: "editor", Operation: "rule:write"},
	}).Error)
	cases := []struct {
		name      string
		roles     []string
		operation string
		want      bool
	}{
		{name: "allowed", roles: []string{"viewer"}, operation: "rule:read", want: true},
		{name: "denied", roles: []string{"viewer"}, operation: "rule:write"},
		{name: "any assigned role", roles: []string{"viewer", "editor"}, operation: "rule:write", want: true},
		{name: "no role", operation: "rule:read"},
		{name: "admin", roles: []string{AdminRole}, operation: "unknown:operation", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := User{RolesLst: tc.roles}

			allowed, err := user.CheckPerm(c, tc.operation)

			require.NoError(t, err)
			require.Equal(t, tc.want, allowed)
		})
	}
}

func TestRoleOperationBind_ReplacesOnlySelectedRoleOperations(t *testing.T) {
	c := permissionContext(t, &RoleOperation{})
	require.NoError(t, c.DB.Create(&[]RoleOperation{
		{RoleName: "viewer", Operation: "read"}, {RoleName: "editor", Operation: "write"},
	}).Error)

	require.NoError(t, RoleOperationBind(c, "viewer", []string{"list", "inspect"}))
	operations, err := OperationsOfRole(c, []string{"viewer"})

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"list", "inspect"}, operations)
	editorAllowed, err := RoleHasOperation(c, []string{"editor"}, "write")
	require.NoError(t, err)
	require.True(t, editorAllowed)
	require.NoError(t, RoleOperationBind(c, "viewer", nil))
	operations, err = OperationsOfRole(c, []string{"viewer"})
	require.NoError(t, err)
	require.Empty(t, operations)
}

func TestOperationsOfRole_AdminGetsDistinctOperationsAcrossRoles(t *testing.T) {
	c := permissionContext(t, &RoleOperation{})
	require.NoError(t, c.DB.Create(&[]RoleOperation{
		{RoleName: "viewer", Operation: "read"}, {RoleName: "editor", Operation: "read"},
		{RoleName: "editor", Operation: "write"},
	}).Error)

	operations, err := OperationsOfRole(c, []string{AdminRole})

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"read", "write"}, operations)
}

func TestUser_CanModifyUserGroupAllowsAdminCreatorOrMember(t *testing.T) {
	c := permissionContext(t, &UserGroupMember{})
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 3, GroupId: 7}).Error)
	group := &UserGroup{Id: 7, CreateBy: "owner"}
	cases := []struct {
		name string
		user User
		want bool
	}{
		{name: "admin", user: User{RolesLst: []string{AdminRole}}, want: true},
		{name: "creator", user: User{Username: "owner"}, want: true},
		{name: "member", user: User{Id: 3, Username: "member"}, want: true},
		{name: "outsider", user: User{Id: 4, Username: "outsider"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, err := tc.user.CanModifyUserGroup(c, group)

			require.NoError(t, err)
			require.Equal(t, tc.want, allowed)
		})
	}
}

func TestUser_CanDoBusiGroupHonorsReadWriteMembership(t *testing.T) {
	c := permissionContext(t, &UserGroupMember{}, &BusiGroupMember{})
	require.NoError(t, c.DB.Create(&[]UserGroupMember{{UserId: 3, GroupId: 7}, {UserId: 4, GroupId: 8}}).Error)
	require.NoError(t, c.DB.Create(&[]BusiGroupMember{
		{BusiGroupId: 10, UserGroupId: 7, PermFlag: "ro"},
		{BusiGroupId: 10, UserGroupId: 8, PermFlag: "rw"},
	}).Error)
	cases := []struct {
		name        string
		user        User
		groupID     int64
		permissions []string
		want        bool
	}{
		{name: "read member", user: User{Id: 3}, groupID: 10, want: true},
		{name: "reader cannot write", user: User{Id: 3}, groupID: 10, permissions: []string{"rw"}},
		{name: "writer", user: User{Id: 4}, groupID: 10, permissions: []string{"rw"}, want: true},
		{name: "unrelated group", user: User{Id: 4}, groupID: 11},
		{name: "outsider", user: User{Id: 5}, groupID: 10},
		{name: "admin", user: User{RolesLst: []string{AdminRole}}, groupID: 11, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, err := tc.user.CanDoBusiGroup(c, &BusiGroup{Id: tc.groupID}, tc.permissions...)

			require.NoError(t, err)
			require.Equal(t, tc.want, allowed)
		})
	}
}

func TestUser_CheckGroupPermissionRequiresAtLeastOneSharedGroup(t *testing.T) {
	c := permissionContext(t, &UserGroupMember{})
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 3, GroupId: 7}).Error)
	user := User{Id: 3}

	require.NoError(t, user.CheckGroupPermission(c, []int64{8, 7}))
	require.EqualError(t, user.CheckGroupPermission(c, []int64{8}), "forbidden")
	require.EqualError(t, user.CheckGroupPermission(c, nil), "forbidden")
	require.NoError(t, (&User{RolesLst: []string{AdminRole}}).CheckGroupPermission(c, nil))
}

func TestUser_NopriIdentsRequiresWritableBusinessGroup(t *testing.T) {
	c := permissionContext(t, &UserGroupMember{}, &BusiGroupMember{}, &Target{}, &TargetBusiGroup{})
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 3, GroupId: 7}).Error)
	require.NoError(t, c.DB.Create(&[]BusiGroupMember{
		{BusiGroupId: 10, UserGroupId: 7, PermFlag: "rw"},
		{BusiGroupId: 11, UserGroupId: 7, PermFlag: "ro"},
	}).Error)
	require.NoError(t, c.DB.Create(&[]Target{{Ident: "writable"}, {Ident: "readonly"}}).Error)
	require.NoError(t, c.DB.Create(&[]TargetBusiGroup{
		{TargetIdent: "writable", GroupId: 10}, {TargetIdent: "readonly", GroupId: 11},
	}).Error)
	user := User{Id: 3}

	denied, err := user.NopriIdents(c, []string{"writable", "readonly", "missing"})

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"readonly", "missing"}, denied)
	outsider := User{Id: 4}
	denied, err = outsider.NopriIdents(c, []string{"writable"})
	require.NoError(t, err)
	require.Equal(t, []string{"writable"}, denied)
	admin := User{RolesLst: []string{AdminRole}}
	denied, err = admin.NopriIdents(c, []string{"missing"})
	require.NoError(t, err)
	require.Empty(t, denied)
}

func TestUser_UpdateDisabledProtectsLastEnabledAdmin(t *testing.T) {
	c := permissionContext(t, &User{})
	require.NoError(t, c.DB.Create(&[]User{
		{Id: 1, Username: "root", Roles: AdminRole, Password: "secret", Email: "root@example.test"},
		{Id: 2, Username: "disabled-admin", Roles: AdminRole, Disabled: UserDisabled},
	}).Error)
	user := User{Id: 1}

	err := user.UpdateDisabled(c, UserDisabled, "operator")

	require.EqualError(t, err, "cannot disable the last enabled admin user")
	var stored User
	require.NoError(t, c.DB.First(&stored, 1).Error)
	require.Equal(t, UserEnabled, stored.Disabled)
	require.Equal(t, AdminRole, stored.Roles)
	require.Equal(t, "secret", stored.Password)
	require.EqualError(t, (&User{}).UpdateDisabled(c, UserDisabled, "operator"), "user id is required")
}

func TestUser_UpdateDisabledPreservesOtherFieldsAndCanBeReversed(t *testing.T) {
	c := permissionContext(t, &User{})
	require.NoError(t, c.DB.Create(&[]User{
		{Id: 1, Username: "alice", Roles: AdminRole, Password: "secret", Email: "a@example.test"},
		{Id: 2, Username: "root", Roles: AdminRole},
	}).Error)
	user := User{Id: 1}

	require.NoError(t, user.UpdateDisabled(c, UserDisabled, "operator"))
	var stored User
	require.NoError(t, c.DB.First(&stored, 1).Error)

	require.True(t, stored.IsDisabled())
	require.Equal(t, "operator", stored.UpdateBy)
	require.Equal(t, "secret", stored.Password)
	require.Equal(t, "a@example.test", stored.Email)
	require.Equal(t, AdminRole, stored.Roles)
	require.NoError(t, user.UpdateDisabled(c, UserEnabled, "operator"))
	require.NoError(t, c.DB.First(&stored, 1).Error)
	require.False(t, stored.IsDisabled())
}

func TestUser_VerifyNormalizesUsernameAndContactTokens(t *testing.T) {
	user := User{Username: " alice ", Email: "alice@example.test",
		Contacts: ormx.JSONObj(`{"wecom_robot_token":"abc\u200bdef\u0000","custom":"keep spaces"}`)}

	err := user.Verify()

	require.NoError(t, err)
	require.Equal(t, "alice", user.Username)
	require.JSONEq(t, `{"wecom_robot_token":"abcdef","custom":"keep spaces"}`, string(user.Contacts))
}

func TestUser_VerifyRejectsInvalidIdentityAndContactFields(t *testing.T) {
	cases := []struct {
		name string
		user User
		want string
	}{
		{name: "blank username", user: User{Username: " "}, want: "Username is blank"},
		{name: "unsafe username", user: User{Username: "<script>"}, want: "Username has invalid characters"},
		{
			name: "unsafe nickname",
			user: User{Username: "alice", Nickname: "<script>"},
			want: "Nickname has invalid characters",
		},
		{name: "invalid phone", user: User{Username: "alice", Phone: "not-a-phone"}, want: "Phone invalid"},
		{name: "invalid email", user: User{Username: "alice", Email: "not-an-email"}, want: "Email invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.EqualError(t, tc.user.Verify(), tc.want)
		})
	}
}

func TestUser_ExtractTokenResolvesChannelAliasesAndContactFields(t *testing.T) {
	user := User{Email: "a@example.test", Phone: "13812345678", Contacts: ormx.JSONObj(
		`{"dingtalk_robot_token":"ding","wecom_robot_token":"wx","feishu_robot_token":"fs",` +
			`"lark_robot_token":"lk","mm_webhook_url":"mm","telegram_robot_token":"tg","custom":"custom-token"}`)}
	cases := []struct {
		channel string
		want    string
	}{
		{channel: Dingtalk, want: "ding"},
		{channel: Wecom, want: "wx"},
		{channel: Feishu, want: "fs"},
		{channel: FeishuCard, want: "fs"},
		{channel: Lark, want: "lk"},
		{channel: LarkCard, want: "lk"},
		{channel: Mm, want: "mm"},
		{channel: Telegram, want: "tg"},
		{channel: Email, want: "a@example.test"},
		{channel: Phone, want: "13812345678"},
		{channel: "custom", want: "custom-token"},
		{channel: "missing"},
	}
	for _, tc := range cases {
		t.Run(tc.channel, func(t *testing.T) {
			token, exists := user.ExtractToken(tc.channel)

			require.Equal(t, tc.want, token)
			require.Equal(t, tc.want != "", exists)
		})
	}
}

func TestUserGets_FiltersAndSanitizesFrontendResults(t *testing.T) {
	c := permissionContext(t, &User{}, &UserGroup{}, &UserGroupMember{}, &BusiGroup{}, &BusiGroupMember{})
	require.NoError(t, c.DB.Create(&[]User{
		{Id: 1, Username: "alice", Nickname: "Alice Ops", Roles: "viewer", Password: "secret", LastActiveTime: 150},
		{Id: 2, Username: "bob", Roles: AdminRole, LastActiveTime: 150, Disabled: UserDisabled},
		{Id: 3, Username: "charlie", LastActiveTime: 300},
	}).Error)
	require.NoError(t, c.DB.Create(&UserGroup{Id: 7, Name: "ops"}).Error)
	require.NoError(t, c.DB.Create(&BusiGroup{Id: 10, Name: "production"}).Error)
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 1, GroupId: 7}).Error)
	require.NoError(t, c.DB.Create(&BusiGroupMember{BusiGroupId: 10, UserGroupId: 7, PermFlag: "rw"}).Error)
	enabled := UserEnabled

	users, err := UserGets(c, "Ops", 10, 0, 100, 200, "username", false, nil, nil, nil, &enabled)

	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, "alice", users[0].Username)
	require.Empty(t, users[0].Password)
	require.Equal(t, []string{"viewer"}, users[0].RolesLst)
	require.False(t, users[0].Admin)
	require.Len(t, users[0].UserGroupsRes, 1)
	require.Equal(t, "ops", users[0].UserGroupsRes[0].Name)
	require.Len(t, users[0].BusiGroupsRes, 1)
	require.Equal(t, "production", users[0].BusiGroupsRes[0].Name)
	total, err := UserTotal(c, "Ops", 100, 200, &enabled)
	require.NoError(t, err)
	require.Equal(t, int64(1), total)
}

func TestUser_BusiGroupsListsOnlyMembershipGroupsForNonAdmin(t *testing.T) {
	c := permissionContext(t, &BusiGroup{}, &BusiGroupMember{}, &UserGroupMember{})
	require.NoError(t, c.DB.Create(&[]BusiGroup{{Id: 10, Name: "ops"}, {Id: 11, Name: "other"}}).Error)
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 1, GroupId: 7}).Error)
	require.NoError(t, c.DB.Create(&BusiGroupMember{BusiGroupId: 10, UserGroupId: 7}).Error)

	groups, err := (&User{Id: 1}).BusiGroups(c, 10, "")

	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, int64(10), groups[0].Id)
	groups, err = (&User{RolesLst: []string{AdminRole}}).BusiGroups(c, 10, "")
	require.NoError(t, err)
	require.Len(t, groups, 2)
	groups, err = (&User{Id: 2}).BusiGroups(c, 10, "")
	require.NoError(t, err)
	require.Empty(t, groups)
}

func TestUser_UserGroupsIncludesOwnedAndJoinedGroups(t *testing.T) {
	c := permissionContext(t, &UserGroup{}, &UserGroupMember{})
	require.NoError(t, c.DB.Create(&[]UserGroup{
		{Id: 7, Name: "joined", CreateBy: "other"}, {Id: 8, Name: "owned", CreateBy: "alice"},
		{Id: 9, Name: "unrelated", CreateBy: "other"},
	}).Error)
	require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 1, GroupId: 7}).Error)

	groups, err := (&User{Id: 1, Username: "alice"}).UserGroups(c, 10, "")

	require.NoError(t, err)
	require.Len(t, groups, 2)
	require.Equal(t, "joined", groups[0].Name)
	require.Equal(t, "owned", groups[1].Name)
	groups, err = (&User{Id: 2, Username: "alice"}).UserGroups(c, 10, "owned")
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Equal(t, "owned", groups[0].Name)
}

func TestValidateOrderField_RejectsSQLExpressions(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "default", want: "username"},
		{name: "simple", input: "nickname", want: "nickname"},
		{name: "qualified", input: "users.username", want: "users.username"},
		{name: "semicolon", input: "id; DROP TABLE users", want: "username"},
		{name: "expression", input: "length(username)", want: "username"},
		{name: "numeric", input: "123", want: "username"},
		{name: "hex literal", input: "0xff", want: "username"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, validateOrderField(tc.input, "username"))
		})
	}
}
