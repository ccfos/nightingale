package models

import (
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestR8dAdditionalQueryFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(*ctx.Context) error
	}{
		{"upgrade rules", func(c *ctx.Context) error { return AlertRuleUpgradeToV6(c, nil) }},
		{"upgrade events", func(c *ctx.Context) error { return AlertCurEventUpgradeToV6(c, nil) }},
		{"upgrade history", func(c *ctx.Context) error { return AlertHisEventUpgradeToV6(c, nil) }},
		{"upgrade mutes", func(c *ctx.Context) error { return AlertMuteUpgradeToV6(c, nil) }},
		{"upgrade subscriptions", func(c *ctx.Context) error { return AlertSubscribeUpgradeToV6(c, nil) }},
		{"upgrade recording", func(c *ctx.Context) error { return RecordingRuleUpgradeToV6(c, nil) }},
		{"add user", func(c *ctx.Context) error { return (&User{Username: "alice"}).Add(c) }},
		{"add rule", func(c *ctx.Context) error {
			return (&AlertRule{Name: "valid", Prod: METRIC, Cate: PROMETHEUS, RuleConfig: `{"queries":[{"prom_ql":"up","severity":1}]}`}).Add(c)
		}},
		{"upsert rule", func(c *ctx.Context) error { return (&AlertRule{Name: "valid"}).Upsert(c) }},
		{"config count", func(c *ctx.Context) error { return ConfigsSetWithUname(c, "key", "value", "user") }},
		{"config add", func(c *ctx.Context) error { return (&Configs{Ckey: "key"}).Add(c) }},
		{"config update", func(c *ctx.Context) error { return (&Configs{Id: 1, Ckey: "key"}).Update(c) }},
		{"config variable decrypt", func(c *ctx.Context) error { _, e := ConfigUserVariableGetDecryptMap(c, nil, ""); return e }},
		{"phone encryption", func(c *ctx.Context) error { return LoadPhoneEncryptionConfig(c) }},
		{"phone encryption enabled", func(c *ctx.Context) error { _, e := GetPhoneEncryptionEnabled(c); return e }},
		{"flashduty key", func(c *ctx.Context) error { _, e := ConfigsGetFlashDutyAppKey(c); return e }},
		{"site info", func(c *ctx.Context) error { _, e := ConfigsGetSiteInfo(c); return e }},
		{"user login config", func(c *ctx.Context) error { _, e := PassLogin(c, nil, "user", "pass"); return e }},
		{"assistant get chat", func(c *ctx.Context) error { _, e := AssistantChatGet(c, "chat"); return e }},
		{"assistant owner", func(c *ctx.Context) error { _, e := AssistantChatCheckOwner(c, "chat", 1); return e }},
		{"assistant list chats", func(c *ctx.Context) error { _, e := AssistantChatGetsByUserID(c, 1); return e }},
		{"assistant list messages", func(c *ctx.Context) error { _, e := AssistantMessageGetsByChat(c, "chat"); return e }},
		{"assistant max sequence", func(c *ctx.Context) error { _, e := AssistantMessageMaxSeqID(c, "chat"); return e }},
		{"assistant get message", func(c *ctx.Context) error { _, e := AssistantMessageGet(c, "chat", 1); return e }},
		{"group permission", func(c *ctx.Context) error { return (&User{Id: 1}).CheckGroupPermission(c, []int64{1}) }},
		{"modify team permission", func(c *ctx.Context) error {
			_, e := (&User{Id: 1}).CanModifyUserGroup(c, &UserGroup{Id: 1, CreateBy: "other"})
			return e
		}},
		{"business permission", func(c *ctx.Context) error { _, e := (&User{Id: 1}).CanDoBusiGroup(c, &BusiGroup{Id: 1}); return e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := pipelineContext(t, &User{})
			failure := errors.New("r8d query failure")
			const callback = "r8d:extra_query"
			require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) { tx.AddError(failure) }))
			t.Cleanup(func() { c.DB.Callback().Query().Remove(callback) })
			require.ErrorIs(t, tc.call(c), failure)
		})
	}
}
func TestR8dRSAConfigQueryStages(t *testing.T) {
	for _, stage := range []int{1, 2, 3} {
		t.Run(string(rune('0'+stage)), func(t *testing.T) {
			c := pipelineContext(t, &Configs{})
			n := 0
			failure := errors.New("r8d rsa config query failed")
			const cb = "r8d:rsa_query"
			require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register(cb, func(tx *gorm.DB) {
				n++
				if n == stage {
					tx.AddError(failure)
				}
			}))
			t.Cleanup(func() { c.DB.Callback().Query().Remove(cb) })
			_, _, _, err := GetRSAKeys(c)
			require.ErrorIs(t, err, failure)
			require.Equal(t, stage, n)
		})
	}
}
