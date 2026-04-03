package stream

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	nctx "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/google/uuid"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/toolkits/pkg/logger"
)

var holderOnce sync.Once
var streamHolder string

func streamInstanceHolder() string {
	holderOnce.Do(func() {
		hn, _ := os.Hostname()
		streamHolder = fmt.Sprintf("%s:%d:%s", hn, os.Getpid(), uuid.NewString())
	})
	return streamHolder
}

// RunnerDeps 启动一条 AppKey 对应的 Stream（在抢到 Redis 租约后）。
type RunnerDeps struct {
	Redis          storage.Redis
	Nctx           *nctx.Context
	AppKey         string
	AppSecret      string
	Proxy          string
	NotifyChannel  *models.NotifyChannelConfig // 用于构造与发消息一致的 HTTP Client
	LeaseTTL       time.Duration
}

// StartRunner 在后台维持连接，直到调用返回的 stop 函数。未抢到租约时立即返回 no-op stop。
func StartRunner(parent context.Context, deps RunnerDeps) (stop func()) {
	if deps.Redis == nil || deps.Nctx == nil || deps.Nctx.DB == nil {
		return func() {}
	}
	if deps.AppKey == "" || deps.AppSecret == "" {
		return func() {}
	}

	ttl := deps.LeaseTTL
	if ttl < 10*time.Second {
		ttl = 30 * time.Second
	}

	root, cancel := context.WithCancel(parent)
	done := make(chan struct{})

	go func() {
		defer close(done)
		lease := NewLeaderLease(deps.Redis, leaderRedisKey(deps.AppKey), streamInstanceHolder(), ttl)
		acquired, err := lease.TryAcquire(root)
		if err != nil {
			logger.Warningf("dingtalk stream lease acquire err appKey=%s: %v", deps.AppKey, err)
			return
		}
		if !acquired {
			logger.Debugf("dingtalk stream skip (not leader) appKey=%s", deps.AppKey)
			return
		}
		defer lease.Release(context.Background())
		renewCtx, renewCancel := context.WithCancel(root)
		defer renewCancel()
		go lease.StartRenew(renewCtx)
		defer lease.StopRenew()

		httpCli, err := models.GetHTTPClient(deps.NotifyChannel)
		if err != nil || httpCli == nil {
			logger.Warningf("dingtalk stream http client appKey=%s: %v", deps.AppKey, err)
			httpCli = httpClientFallback()
		}

		proc := newEventProcessor(EventHandlerDeps{
			Nctx:       deps.Nctx,
			Redis:      deps.Redis,
			ClientID:   deps.AppKey,
			AppSecret:  deps.AppSecret,
			HTTPClient: httpCli,
		})

		opts := []client.ClientOption{
			client.WithAppCredential(client.NewAppCredentialConfig(deps.AppKey, deps.AppSecret)),
			client.WithAutoReconnect(true),
		}
		if deps.Proxy != "" {
			opts = append(opts, client.WithProxy(deps.Proxy))
		}
		cli := client.NewStreamClient(opts...)
		cli.RegisterAllEventRouter(proc.onDataFrame)

		if err := cli.Start(root); err != nil {
			logger.Errorf("dingtalk stream start failed appKey=%s: %v", deps.AppKey, err)
			return
		}

		<-root.Done()
		cli.AutoReconnect = false
		cli.Close()
		logger.Infof("dingtalk stream stopped appKey=%s", deps.AppKey)
	}()

	return func() {
		cancel()
		<-done
	}
}

func httpClientFallback() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}
