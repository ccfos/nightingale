package models_test

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

func seedTestDB(t *testing.T) *ctx.Context {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.NotifyChannelConfig{}); err != nil {
		t.Fatalf("migrate notify_channel: %v", err)
	}
	return &ctx.Context{DB: db, IsCenter: true}
}

// legacyDiscord 是 #3136 之前内置种子写入的 Discord 媒介：通用 HTTP、默认禁用、规则侧参数 webhook_url。
func legacyDiscord(updateBy string) *models.NotifyChannelConfig {
	return &models.NotifyChannelConfig{
		Name: "Discord", Ident: models.Discord, RequestType: "http", Enable: false, UpdateBy: updateBy, CreateBy: "system",
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{
			URL: "{{$params.webhook_url}}", Method: "POST", Timeout: 10000,
			Request: models.RequestDetail{Body: `{"content": "{{$tpl.content}}"}`},
		}},
		ParamConfig: &models.NotifyParamConfig{Custom: models.Params{Params: []models.ParamItem{{Key: "webhook_url", CName: "Webhook Url", Type: "string"}}}},
	}
}

func discordRow(t *testing.T, c *ctx.Context) *models.NotifyChannelConfig {
	t.Helper()
	lst, err := models.NotifyChannelsGet(c, "ident = ?", models.Discord)
	if err != nil || len(lst) != 1 {
		t.Fatalf("expected exactly one discord channel, got %d (err=%v)", len(lst), err)
	}
	return lst[0]
}

// 从没被用户改过的旧种子记录原地升级成原生 Discord（规则参数键同为 webhook_url，已有规则照常可用）。
func TestInitNotifyChannelUpgradesUntouchedLegacyDiscord(t *testing.T) {
	c := seedTestDB(t)
	if err := models.Insert(c, legacyDiscord("system")); err != nil {
		t.Fatal(err)
	}
	models.InitNotifyChannel(c)
	ch := discordRow(t, c)
	if ch.RequestType != models.RequestTypeDiscord {
		t.Fatalf("untouched system row should become native discord, got %s", ch.RequestType)
	}
	if ch.ParamConfig == nil || len(ch.ParamConfig.Custom.Params) == 0 || ch.ParamConfig.Custom.Params[0].Key != "webhook_url" {
		t.Fatalf("rule param webhook_url must stay first for existing rules: %+v", ch.ParamConfig)
	}
}

// 用户动过的旧记录（UpdateBy 是用户名）一律不碰，仍是 http，发送时兜底到 callback。
func TestInitNotifyChannelKeepsUserEditedLegacyDiscord(t *testing.T) {
	c := seedTestDB(t)
	if err := models.Insert(c, legacyDiscord("root")); err != nil {
		t.Fatal(err)
	}
	models.InitNotifyChannel(c)
	ch := discordRow(t, c)
	if ch.RequestType != "http" || ch.RequestConfig.HTTPRequestConfig == nil {
		t.Fatalf("user-edited legacy row must be left alone, got request_type=%s", ch.RequestType)
	}
}

func TestInitNotifyChannelSeedsDiscordOnFreshDB(t *testing.T) {
	c := seedTestDB(t)
	models.InitNotifyChannel(c)
	if ch := discordRow(t, c); ch.RequestType != models.RequestTypeDiscord || !ch.Enable {
		t.Fatalf("fresh db should get an enabled native discord channel, got %+v", ch)
	}
}
