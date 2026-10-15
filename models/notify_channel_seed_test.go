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

// #3136 之前内置的「JSM Alert」是默认启用的通用 HTTP 媒介，规则参数同为 api_key：
// 没被改过的原地升级成原生 JSM 告警，改过的不动。
func TestInitNotifyChannelUpgradesLegacyJSMAlert(t *testing.T) {
	for _, tc := range []struct {
		updateBy string
		want     string
	}{{"system", models.RequestTypeJSMAlert}, {"root", "http"}} {
		c := seedTestDB(t)
		legacy := &models.NotifyChannelConfig{
			Name: "JSM Alert", Ident: models.JSMAlert, RequestType: "http", Enable: true, UpdateBy: tc.updateBy, CreateBy: "system",
			RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{
				URL: "https://api.atlassian.com/jsm/ops/integration/v2/alerts", Method: "POST", Timeout: 10000,
				Headers: map[string]string{"Authorization": "GenieKey {{$params.api_key}}"},
			}},
			ParamConfig: &models.NotifyParamConfig{Custom: models.Params{Params: []models.ParamItem{{Key: "api_key", CName: "API Key", Type: "string"}}}},
		}
		if err := models.Insert(c, legacy); err != nil {
			t.Fatal(err)
		}
		models.InitNotifyChannel(c)
		lst, err := models.NotifyChannelsGet(c, "ident = ?", models.JSMAlert)
		if err != nil || len(lst) != 1 {
			t.Fatalf("expected exactly one jsm_alert channel, got %d (err=%v)", len(lst), err)
		}
		if lst[0].RequestType != tc.want {
			t.Fatalf("update_by=%s: request_type=%s, want %s", tc.updateBy, lst[0].RequestType, tc.want)
		}
		if tc.want == models.RequestTypeJSMAlert && lst[0].ParamConfig.Custom.Params[0].Key != "api_key" {
			t.Fatalf("rule param api_key must stay first for existing rules: %+v", lst[0].ParamConfig)
		}
	}
}

// legacyWebhookSeed 是 #3136 之前内置种子写入的 SlackWebhook / MattermostWebhook：通用 HTTP、默认禁用、
// 规则侧参数 webhook_url + bot_name。
func legacyWebhookSeed(name, ident, updateBy string) *models.NotifyChannelConfig {
	return &models.NotifyChannelConfig{
		Name: name, Ident: ident, RequestType: "http", Enable: false, UpdateBy: updateBy, CreateBy: "system",
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{
			URL: "{{$params.webhook_url}}", Method: "POST", Timeout: 10000,
			Request: models.RequestDetail{Body: `{"text":  "{{$tpl.content}}"}`},
		}},
		ParamConfig: &models.NotifyParamConfig{Custom: models.Params{Params: []models.ParamItem{
			{Key: "webhook_url", CName: "Webhook Url", Type: "string"},
			{Key: "bot_name", CName: "Bot Name", Type: "string"},
		}}},
	}
}

func TestInitNotifyChannelUpgradesLegacySlackAndMattermostWebhooks(t *testing.T) {
	cases := []struct{ name, ident, requestType string }{
		{"SlackWebhook", models.SlackWebhook, models.RequestTypeSlackWebhook},
		{"MattermostWebhook", models.MattermostWebhook, models.RequestTypeMattermostWebhook},
	}
	for _, tc := range cases {
		row := func(c *ctx.Context) *models.NotifyChannelConfig {
			lst, err := models.NotifyChannelsGet(c, "ident = ?", tc.ident)
			if err != nil || len(lst) != 1 {
				t.Fatalf("%s: expected exactly one channel, got %d (err=%v)", tc.ident, len(lst), err)
			}
			return lst[0]
		}

		// 没被改过的旧种子原地升级成原生媒介，规则参数键不变，已有规则照常可用
		c := seedTestDB(t)
		if err := models.Insert(c, legacyWebhookSeed(tc.name, tc.ident, "system")); err != nil {
			t.Fatal(err)
		}
		models.InitNotifyChannel(c)
		ch := row(c)
		if ch.RequestType != tc.requestType || !ch.Enable {
			t.Fatalf("%s: untouched system row should become an enabled native channel, got %s", tc.ident, ch.RequestType)
		}
		if ch.ParamConfig == nil || len(ch.ParamConfig.Custom.Params) != 2 || ch.ParamConfig.Custom.Params[0].Key != "webhook_url" ||
			ch.ParamConfig.Custom.Params[1].Key != "bot_name" {
			t.Fatalf("%s: rule params must stay webhook_url + bot_name: %+v", tc.ident, ch.ParamConfig)
		}

		// 用户改过的旧记录不碰
		c = seedTestDB(t)
		if err := models.Insert(c, legacyWebhookSeed(tc.name, tc.ident, "root")); err != nil {
			t.Fatal(err)
		}
		models.InitNotifyChannel(c)
		if ch := row(c); ch.RequestType != "http" {
			t.Fatalf("%s: user-edited legacy row must be left alone, got %s", tc.ident, ch.RequestType)
		}

		// 新库直接种上
		c = seedTestDB(t)
		models.InitNotifyChannel(c)
		if ch := row(c); ch.RequestType != tc.requestType || !ch.Enable {
			t.Fatalf("%s: fresh db should get an enabled native channel, got %+v", tc.ident, ch)
		}
	}
}

func TestTeamsSeedIsIdempotentAndPreservesUserEdits(t *testing.T) {
	c := seedTestDB(t)
	models.InitNotifyChannel(c)
	models.InitNotifyChannel(c)
	rows, err := models.NotifyChannelsGet(c, "ident = ?", models.Teams)
	if err != nil || len(rows) != 1 {
		t.Fatalf("expected one Teams seed: %v, %v", rows, err)
	}
	row := rows[0]
	if row.RequestType != models.RequestTypeTeams || !row.Enable || row.RequestConfig.TeamsRequestConfig == nil || row.ParamConfig.Custom.Params[0].Key != "webhook_url" {
		t.Fatalf("invalid Teams seed: %+v", row)
	}
	edited := *row
	edited.UpdateBy = "root"
	edited.RequestConfig = &models.RequestConfig{TeamsRequestConfig: &models.TeamsRequestConfig{NativeNetworkConfig: models.NativeNetworkConfig{Timeout: 4567}}}
	if err := row.Update(c, edited); err != nil {
		t.Fatal(err)
	}
	models.InitNotifyChannel(c)
	row, err = models.NotifyChannelGet(c, "ident = ?", models.Teams)
	if err != nil || row.RequestConfig.TeamsRequestConfig.Timeout != 4567 {
		t.Fatalf("user settings overwritten: %+v, %v", row, err)
	}
}

// ntfy 不需要凭证，随启动内置；重复初始化不能多出一条，用户改过的设置不能被覆盖
func TestNtfySeedIsIdempotentAndPreservesUserEdits(t *testing.T) {
	c := seedTestDB(t)
	models.InitNotifyChannel(c)
	models.InitNotifyChannel(c)
	for ident, paramKey := range map[string]string{models.Ntfy: "topic"} {
		rows, err := models.NotifyChannelsGet(c, "ident = ?", ident)
		if err != nil || len(rows) != 1 {
			t.Fatalf("expected one %s seed: %v, %v", ident, rows, err)
		}
		row := rows[0]
		if row.RequestType != ident || !row.Enable || row.ParamConfig.Custom.Params[0].Key != paramKey || row.ParamConfig.Custom.Params[1].Key != "bot_name" {
			t.Fatalf("invalid %s seed: %+v", ident, row)
		}
		if n := row.RequestConfig.NativeNetwork(ident); n == nil || n.Timeout != 10000 {
			t.Fatalf("%s seed should carry default network settings: %+v", ident, row.RequestConfig)
		}
	}

	row, _ := models.NotifyChannelGet(c, "ident = ?", models.Ntfy)
	edited := *row
	edited.UpdateBy = "root"
	edited.RequestConfig = &models.RequestConfig{NtfyRequestConfig: &models.NtfyRequestConfig{ServerURL: "https://ntfy.example.com", Token: "tk_x"}}
	if err := row.Update(c, edited); err != nil {
		t.Fatal(err)
	}
	models.InitNotifyChannel(c)
	row, err := models.NotifyChannelGet(c, "ident = ?", models.Ntfy)
	if err != nil || row.RequestConfig.NtfyRequestConfig.ServerURL != "https://ntfy.example.com" {
		t.Fatalf("user settings overwritten: %+v, %v", row, err)
	}

	// 需要凭证的媒介不内置：带占位凭证的种子一发就失败
	for _, ident := range []string{models.Pushover, models.SplunkOnCall, models.Line} {
		if rows, _ := models.NotifyChannelsGet(c, "ident = ?", ident); len(rows) != 0 {
			t.Fatalf("%s must not be seeded", ident)
		}
	}
}
