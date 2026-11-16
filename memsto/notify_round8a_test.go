package memsto

import (
	"errors"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"gopkg.in/gomail.v2"
	"gorm.io/gorm"
)

func TestR8aNotifyConfigQueryFailuresPreserveSnapshot(t *testing.T) {
	for _, failAt := range []int{1, 2, 3} {
		t.Run([]string{"webhooks", "smtp", "script"}[failAt-1], func(t *testing.T) {
			db, c, _ := round2SyncDB(t, &models.Configs{})
			require.NoError(t, db.Create(&[]models.Configs{
				{Ckey: models.WEBHOOKKEY, Cval: `[{"url":"https://example.invalid/hook","timeout":3}]`},
				{Ckey: models.SMTP, Cval: "Host = \"mail.invalid\"\nPort = 2525"},
				{Ckey: models.NOTIFYSCRIPT, Cval: `{"content":"echo new","timeout":4}`},
			}).Error)
			old := &models.Webhook{Url: "https://example.invalid/old"}
			cache := &NotifyConfigCacheType{ctx: c, ConfigCache: &ConfigCache{}, webhooks: map[string]*models.Webhook{old.Url: old}, script: models.NotifyScript{Content: "old"}}
			calls := 0
			failure := errors.New("r8a config unavailable")
			require.NoError(t, db.Callback().Query().Before("gorm:query").Register("r8a:query", func(tx *gorm.DB) {
				calls++
				if calls == failAt {
					tx.AddError(failure)
				}
			}))
			t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove("r8a:query")) })
			require.ErrorIs(t, cache.syncNotifyConfigs(), failure)
			require.Equal(t, failAt, calls)
			require.Equal(t, "old", cache.GetNotifyScript().Content)
			if failAt == 1 {
				require.Same(t, old, cache.GetWebhooks()[old.Url])
			} else {
				require.NotContains(t, cache.GetWebhooks(), old.Url)
				hook := cache.GetWebhooks()["https://example.invalid/hook"]
				require.NotNil(t, hook)
				t.Cleanup(hook.Client.CloseIdleConnections)
			}
			if failAt == 3 {
				require.Equal(t, "mail.invalid", cache.GetSMTP().Host)
			} else {
				require.Empty(t, cache.GetSMTP().Host)
			}
		})
	}
}

func TestR8aNotifyConfigMalformedValuesAndDefaults(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.Configs{})
	require.NoError(t, db.Create(&[]models.Configs{
		{Ckey: models.WEBHOOKKEY, Cval: `{`},
		{Ckey: models.SMTP, Cval: `Host = [`},
		{Ckey: models.NOTIFYSCRIPT, Cval: `{`},
	}).Error)
	cache := &NotifyConfigCacheType{ctx: c, ConfigCache: &ConfigCache{}, webhooks: map[string]*models.Webhook{"old": {Url: "old"}}, script: models.NotifyScript{Content: "previous"}}
	// Malformed settings are logged; sync continues to process the remaining keys.
	require.NoError(t, cache.syncNotifyConfigs())
	require.Empty(t, cache.GetWebhooks())
	require.Empty(t, cache.GetSMTP().Host)
	require.Equal(t, "previous", cache.GetNotifyScript().Content)
	require.Equal(t, 10, cache.GetNotifyScript().Timeout)
}

func TestR8aEmailSenderShutdownWithoutDialing(t *testing.T) {
	cache := &NotifyChannelCacheType{}
	quit := make(chan struct{})
	close(quit)
	messages := make(chan *models.EmailContext)
	// Invalid settings return immediately, even without a shutdown signal.
	cache.startEmailSender(1, &models.SMTPRequestConfig{}, messages, make(chan struct{}))
	require.Empty(t, messages)
	cache.startEmailSender(1, &models.SMTPRequestConfig{Host: "mail.invalid", Port: 25, InsecureSkipVerify: true}, messages, quit)
	require.Empty(t, messages)
	close(messages)
	cache.startEmailSender(1, &models.SMTPRequestConfig{Host: "mail.invalid", Port: 25}, messages, make(chan struct{}))
	require.Nil(t, cache.dialSmtp(quit, gomail.NewDialer("mail.invalid", 25, "", "")))
}
