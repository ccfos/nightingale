package stream

import (
	"context"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	nctx "github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/client"
	"github.com/toolkits/pkg/logger"
)

// RunnerDeps 启动一条 AppKey 对应的 Stream。
type RunnerDeps struct {
	Nctx          *nctx.Context
	AppKey        string
	AppSecret     string
	Proxy         string
	NotifyChannel *models.NotifyChannelConfig // 用于构造与发消息一致的 HTTP Client
}

// StartRunner 在后台维持连接，直到调用返回的 stop 函数。
func StartRunner(parent context.Context, deps RunnerDeps) (stop func()) {
	if deps.Nctx == nil || deps.Nctx.DB == nil {
		return func() {}
	}
	if deps.AppKey == "" || deps.AppSecret == "" {
		return func() {}
	}

	root, cancel := context.WithCancel(parent)
	done := make(chan struct{})

	go func() {
		defer close(done)
		httpCli, err := models.GetHTTPClient(deps.NotifyChannel)
		if err != nil || httpCli == nil {
			logger.Warningf("dingtalk stream http client appKey=%s: %v", deps.AppKey, err)
			httpCli = httpClientFallback()
		}

		proc := newEventProcessor(EventHandlerDeps{
			Nctx:       deps.Nctx,
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
