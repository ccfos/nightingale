package models

import (
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestR8dUserAssociationFailures(t *testing.T) {
	for _, tc := range []struct {
		name  string
		admin bool
		table string
		call  func(*User, *ctx.Context) error
	}{
		{"list memberships", false, "user_group_member", func(u *User, c *ctx.Context) error {
			_, e := UserGets(c, "", 10, 0, 0, 0, "username", false, nil, nil, nil, nil)
			return e
		}},
		{"list teams", false, "user_group", func(u *User, c *ctx.Context) error {
			_, e := UserGets(c, "", 10, 0, 0, 0, "username", false, nil, nil, nil, nil)
			return e
		}},
		{"list group relations", false, "busi_group_member", func(u *User, c *ctx.Context) error {
			_, e := UserGets(c, "", 10, 0, 0, 0, "username", false, nil, nil, nil, nil)
			return e
		}},
		{"list business groups", false, "busi_group", func(u *User, c *ctx.Context) error {
			_, e := UserGets(c, "", 10, 0, 0, 0, "username", false, nil, nil, nil, nil)
			return e
		}},
		{"groups memberships", false, "user_group_member", func(u *User, c *ctx.Context) error { _, e := u.BusiGroups(c, 10, "node"); return e }},
		{"groups relations", false, "busi_group_member", func(u *User, c *ctx.Context) error { _, e := u.BusiGroups(c, 10, "node"); return e }},
		{"groups query", false, "busi_group", func(u *User, c *ctx.Context) error { _, e := u.BusiGroups(c, 10, "node"); return e }},
		{"groups target", false, "target", func(u *User, c *ctx.Context) error { _, e := u.BusiGroups(c, 10, "node"); return e }},
		{"admin groups query", true, "busi_group", func(u *User, c *ctx.Context) error { _, e := u.BusiGroups(c, 10, "node"); return e }},
		{"admin target", true, "target", func(u *User, c *ctx.Context) error { _, e := u.BusiGroups(c, 10, "node"); return e }},
		{"admin target groups", true, "target_busi_group", func(u *User, c *ctx.Context) error { _, e := u.BusiGroups(c, 10, "node"); return e }},
		{"groups target relations", false, "target_busi_group", func(u *User, c *ctx.Context) error { _, e := u.BusiGroups(c, 10, "node"); return e }},
		{"teams memberships", false, "user_group_member", func(u *User, c *ctx.Context) error { _, e := u.UserGroups(c, 10, ""); return e }},
		{"admin teams", true, "user_group", func(u *User, c *ctx.Context) error { _, e := u.UserGroups(c, 10, "alice"); return e }},
		{"admin team lookup", true, "users", func(u *User, c *ctx.Context) error { _, e := u.UserGroups(c, 10, "alice"); return e }},
		{"admin team memberships", true, "user_group_member", func(u *User, c *ctx.Context) error { _, e := u.UserGroups(c, 10, "alice"); return e }},
		{"unauthorized memberships", false, "user_group_member", func(u *User, c *ctx.Context) error { _, e := u.NopriIdents(c, []string{"node"}); return e }},
		{"unauthorized relations", false, "busi_group_member", func(u *User, c *ctx.Context) error { _, e := u.NopriIdents(c, []string{"node"}); return e }},
		{"unauthorized targets", false, "target", func(u *User, c *ctx.Context) error { _, e := u.NopriIdents(c, []string{"node"}); return e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &User{}, &UserGroup{}, &UserGroupMember{}, &BusiGroup{}, &BusiGroupMember{}, &Target{}, &TargetBusiGroup{})
			require.NoError(t, c.DB.Create(&User{Id: 1, Username: "alice", Password: "secret"}).Error)
			require.NoError(t, c.DB.Create(&UserGroup{Id: 7, Name: "ops"}).Error)
			require.NoError(t, c.DB.Create(&UserGroupMember{UserId: 1, GroupId: 7}).Error)
			require.NoError(t, c.DB.Create(&BusiGroup{Id: 8, Name: "business"}).Error)
			require.NoError(t, c.DB.Create(&BusiGroupMember{BusiGroupId: 8, UserGroupId: 7, PermFlag: "rw"}).Error)
			require.NoError(t, c.DB.Create(&Target{Ident: "node"}).Error)
			require.NoError(t, c.DB.Create(&TargetBusiGroup{TargetIdent: "node", GroupId: 8}).Error)
			failure := errors.New("r8d association lookup failed")
			const callback = "r8d:association_failure"
			require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table == tc.table {
					tx.AddError(failure)
				}
			}))
			t.Cleanup(func() { c.DB.Callback().Query().Remove(callback) })
			u := &User{Id: 1, Username: "alice"}
			if tc.admin {
				u.RolesLst = []string{AdminRole}
			}
			require.ErrorIs(t, tc.call(u, c), failure)
		})
	}
}
