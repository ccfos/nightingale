package memsto

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTargetsOfAlertRuleCache_UnchangedSignatureSkipsTargetQueries(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.AlertRule{}, &models.Target{}, &models.TargetBusiGroup{})
	require.NoError(t, db.Create(&models.AlertRule{
		Id: 1, Name: "all hosts", Prod: models.HOST, RuleConfig: `{"queries":[]}`, UpdateAt: 10,
	}).Error)
	require.NoError(t, db.Create(&models.Target{Ident: "node", EngineName: "edge-a", UpdateAt: 10}).Error)
	cache := &TargetsOfAlertRuleCacheType{ctx: c}
	require.NoError(t, cache.syncTargets())
	signature := cache.lastSig
	require.NotEmpty(t, signature)
	queries := 0
	callback := "round7:count-target-rows"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "target" && len(tx.Statement.Selects) == 0 {
			queries++
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Query().Remove(callback)) })

	err := cache.syncTargets()

	require.NoError(t, err)
	require.Zero(t, queries)
	require.Equal(t, signature, cache.lastSig)
	idents, ok := cache.Get("edge-a", 1)
	require.True(t, ok)
	require.Equal(t, []string{"node"}, idents)
}

func TestNotifyChannelCache_MissingHTTPConfigCreatesNoQueue(t *testing.T) {
	cache, _, _ := channelResourceCache()
	channel := &models.NotifyChannelConfig{ID: 2, RequestType: "http"}

	cache.Set(map[int64]*models.NotifyChannelConfig{2: channel}, 1, 10)

	require.Same(t, channel, cache.Get(2))
	require.Nil(t, cache.GetHttpClient(2))
	require.Empty(t, cache.channelsQueue)
	require.Empty(t, cache.queueQuitCh)
	require.False(t, cache.EnqueueNotifyTask(&NotifyTask{Request: &provider.NotifyRequest{Config: channel}}))
}

func TestNotifyChannelCache_ConcurrentLazyClientsShareOneInstance(t *testing.T) {
	cache := &NotifyChannelCacheType{
		channels: map[int64]*models.NotifyChannelConfig{1: {
			ID: 1, RequestType: "http", RequestConfig: &models.RequestConfig{
				HTTPRequestConfig: &models.HTTPRequestConfig{Timeout: 1700},
			},
		}},
		httpClient: make(map[int64]*http.Client),
	}
	const workers = 16
	start := make(chan struct{})
	clients := make(chan *http.Client, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			clients <- cache.GetHttpClient(1)
		}()
	}

	close(start)
	wg.Wait()
	close(clients)

	client := cache.httpClient[1]
	require.NotNil(t, client)
	t.Cleanup(client.CloseIdleConnections)
	require.Equal(t, 1700*time.Millisecond, client.Timeout)
	for got := range clients {
		require.Same(t, client, got)
	}
	require.Len(t, cache.httpClient, 1)
}

func TestNotifyChannelCache_UpdateRebuildsLazyClientWithNewTimeout(t *testing.T) {
	cache, _, _ := channelResourceCache()
	channel := &models.NotifyChannelConfig{
		ID: 1, RequestType: "feishuapp", UpdateAt: 11,
		RequestConfig: &models.RequestConfig{FeishuAppRequestConfig: &models.FeishuAppRequestConfig{}},
	}
	cache.Set(map[int64]*models.NotifyChannelConfig{1: channel}, 1, 11)
	first := cache.GetHttpClient(1)
	require.NotNil(t, first)
	t.Cleanup(first.CloseIdleConnections)
	replacement := *channel
	replacement.UpdateAt = 12
	replacement.RequestConfig = &models.RequestConfig{FeishuAppRequestConfig: &models.FeishuAppRequestConfig{
		Timeout: 2500,
	}}

	cache.Set(map[int64]*models.NotifyChannelConfig{1: &replacement}, 1, 12)
	second := cache.GetHttpClient(1)

	require.NotNil(t, second)
	t.Cleanup(second.CloseIdleConnections)
	require.NotSame(t, first, second)
	require.Equal(t, 2500*time.Millisecond, second.Timeout)
	require.Same(t, second, cache.GetHttpClient(1))
}

func TestProcessNotifyTask_RecordsNamedBotInsteadOfCredential(t *testing.T) {
	var targets []string
	cache := &NotifyChannelCacheType{ctx: ctx.NewContext(context.Background(), nil, true)}
	cache.SetNotifyRecordFunc(func(_ *ctx.Context, _ []*models.AlertCurEvent, _ int64, _, target, _ string, _ error) {
		targets = append(targets, target)
	})
	p := &recordingProvider{result: &provider.NotifyResult{Response: "accepted"}}
	req := &provider.NotifyRequest{
		Config: &models.NotifyChannelConfig{
			Name: "ops", RequestType: "http",
			RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: "$sendtos"}},
		},
		Events: []*models.AlertCurEvent{{Hash: "event"}}, Sendtos: []string{"alice", "bob"},
		CustomParams: map[string]string{"bot_name": " Operations ", "access_token": "private-credential"},
	}

	cache.processNotifyTask(&NotifyTask{Request: req, Provider: p})

	require.Equal(t, []string{"Operations"}, targets)
	require.Equal(t, [][]string{{"alice", "bob"}}, p.recipients)
	require.Equal(t, "private-credential", req.CustomParams["access_token"])
}

func TestUserTokenCache_FailedUsageWriteKeepsActivityForRetry(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.UserToken{})
	require.NoError(t, db.Create(&models.UserToken{Token: "recent", LastUsed: 1}).Error)
	now := time.Now().Unix()
	cache := &UserTokenCacheType{ctx: c, tokensLastUsed: map[string]int64{"recent": now}}
	failure := errors.New("usage update unavailable")
	failUpdate := true
	callback := "round7:fail-token-usage"
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		if failUpdate {
			tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { require.NoError(t, db.Callback().Update().Remove(callback)) })

	err := cache.updateUserTokenLastUsedTime()

	// Usage tracking is best effort; retaining the timestamp lets the next sync retry it.
	require.NoError(t, err)
	require.Equal(t, now, cache.tokensLastUsed["recent"])
	var row models.UserToken
	require.NoError(t, db.Where("token = ?", "recent").First(&row).Error)
	require.Equal(t, int64(1), row.LastUsed)
	failUpdate = false

	require.NoError(t, cache.updateUserTokenLastUsedTime())
	require.NoError(t, db.Where("token = ?", "recent").First(&row).Error)
	require.Equal(t, now, row.LastUsed)
}

func TestUserTokenCache_InjectRejectsEmptyTokenAndNilUser(t *testing.T) {
	alice := &models.User{Username: "alice"}
	cache := &UserTokenCacheType{tokens: map[string]*models.User{"existing": alice}}

	cache.Inject("", alice)
	cache.Inject("new", nil)
	cache.Inject("existing", nil)

	require.Equal(t, map[string]*models.User{"existing": alice}, cache.tokens)
}

func TestTargetCache_RefreshEmptyCacheDoesNotReadRedis(t *testing.T) {
	cache, server := round4TargetCache(t)
	before := server.CommandCount()

	cache.refreshBeatTime()
	cache.refreshHostMetas()

	require.Equal(t, before, server.CommandCount())
	require.Empty(t, cache.GetAll())
}

func TestTargetCache_RefreshWithoutRedisPreservesMetadata(t *testing.T) {
	cache := &TargetCacheType{}
	cache.Set(map[string]*models.Target{"node": {Ident: "node", CpuNum: 8}}, 1, 10)

	cache.refreshHostMetas()

	target, ok := cache.Get("node")
	require.True(t, ok)
	require.Equal(t, 8, target.CpuNum)
}

func TestSyncNotifyConfigs_ReconfiguresExistingWebhookTransport(t *testing.T) {
	db, c, _ := round2SyncDB(t, &models.Configs{})
	row := &models.Configs{Ckey: models.WEBHOOKKEY, Cval: `[{"url":"http://127.0.0.1/hook","timeout":2}]`}
	require.NoError(t, db.Create(row).Error)
	cache := &NotifyConfigCacheType{ctx: c, ConfigCache: &ConfigCache{}, webhooks: make(map[string]*models.Webhook)}
	require.NoError(t, cache.syncNotifyConfigs())
	first := cache.GetWebhooks()["http://127.0.0.1/hook"]
	t.Cleanup(first.Client.CloseIdleConnections)
	require.NoError(t, db.Model(row).Update("cval", `[{"url":"http://127.0.0.1/hook","timeout":7,"skip_verify":true}]`).Error)

	err := cache.syncNotifyConfigs()

	require.NoError(t, err)
	second := cache.GetWebhooks()["http://127.0.0.1/hook"]
	require.NotNil(t, second)
	t.Cleanup(second.Client.CloseIdleConnections)
	require.NotSame(t, first, second)
	require.Equal(t, 7*time.Second, second.Client.Timeout)
	require.True(t, second.Client.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify)
	require.Len(t, cache.GetWebhooks(), 1)
}
