package models

import (
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestR8dMutationFailures(t *testing.T) {
	for _, tc := range []struct {
		name, kind string
		call       func(*ctx.Context) error
	}{
		{"delete rules", "delete", func(c *ctx.Context) error { return AlertRuleDels(c, []int64{1}) }},
		{"delete mutes", "delete", func(c *ctx.Context) error { return AlertMuteDel(c, []int64{1}) }},
		{"delete configs", "delete", func(c *ctx.Context) error { return ConfigsDel(c, []int64{1}) }},
		{"delete user memberships", "delete", func(c *ctx.Context) error { return UserDelByIds(c, []int64{1}) }},
		{"delete user", "delete", func(c *ctx.Context) error { return (&User{Id: 1, Username: "alice"}).Del(c) }},
		{"delete group", "delete", func(c *ctx.Context) error { return (&BusiGroup{Id: 1}).Del(c) }},
		{"delete recording rule", "delete", func(c *ctx.Context) error { return RecordingRuleDels(c, []int64{1}, 1) }},
		{"unbind targets", "delete", func(c *ctx.Context) error { return TargetUnbindBgids(c, []string{"node"}, []int64{1}) }},
		{"override targets", "delete", func(c *ctx.Context) error { return TargetOverrideBgids(c, []string{"node"}, []int64{1}, nil) }},
		{"bind targets", "create", func(c *ctx.Context) error { return TargetBindBgids(c, []string{"node"}, []int64{1}, nil) }},
		{"insert rules", "create", func(c *ctx.Context) error { return InsertAlertRule(c, []*AlertRule{{Name: "valid"}}) }},
		{"update active time", "update", func(c *ctx.Context) error { return UpdateUserLastActiveTime(c, 1, 100) }},
		{"update rule columns", "update", func(c *ctx.Context) error { return (&AlertRule{Id: 1}).UpdateColumn(c, "disabled", 1) }},
		{"update mute fields", "update", func(c *ctx.Context) error {
			return (&AlertMute{Id: 1}).UpdateFieldsMap(c, map[string]interface{}{"disabled": 1})
		}},
		{"update subscription fields", "update", func(c *ctx.Context) error {
			return (&AlertSubscribe{Id: 1}).UpdateFieldsMap(c, map[string]interface{}{"disabled": 1})
		}},
		{"save assistant chat", "update", func(c *ctx.Context) error { return AssistantChatSet(c, AssistantChat{ChatID: "chat"}) }},
		{"save assistant message", "update", func(c *ctx.Context) error { return AssistantMessageSet(c, AssistantMessage{ChatID: "chat", SeqID: 1}) }},
		{"delete assistant chat", "delete", func(c *ctx.Context) error { return AssistantChatDelete(c, "chat") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &User{}, &UserGroupMember{}, &UserToken{}, &BusiGroup{}, &BusiGroupMember{}, &AlertRule{}, &AlertMute{}, &AlertSubscribe{}, &Configs{}, &RecordingRule{}, &Target{}, &TargetBusiGroup{}, &AssistantChatRow{}, &AssistantMessageRow{}, &Board{}, &TaskTpl{})
			require.NoError(t, c.DB.Create(&User{Id: 1, Username: "alice"}).Error)
			failure := errors.New("r8d write rejected")
			const cb = "r8d:mutation"
			reject := func(tx *gorm.DB) { tx.AddError(failure) }
			switch tc.kind {
			case "delete":
				require.NoError(t, c.DB.Callback().Delete().Before("gorm:delete").Register(cb, reject))
				t.Cleanup(func() { c.DB.Callback().Delete().Remove(cb) })
			case "create":
				require.NoError(t, c.DB.Callback().Create().Before("gorm:create").Register(cb, reject))
				t.Cleanup(func() { c.DB.Callback().Create().Remove(cb) })
			case "update":
				require.NoError(t, c.DB.Callback().Update().Before("gorm:update").Register(cb, reject))
				t.Cleanup(func() { c.DB.Callback().Update().Remove(cb) })
			}
			require.ErrorIs(t, tc.call(c), failure)
			var stored User
			require.NoError(t, c.DB.First(&stored, 1).Error)
			require.Equal(t, "alice", stored.Username)
		})
	}
}
