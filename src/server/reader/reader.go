package reader

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/prom"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/prometheus/client_golang/api"
	"github.com/toolkits/pkg/logger"
)

var Client prom.API
var LocalPromOption PromOption

func Init() error {
	rf := strings.ToLower(strings.TrimSpace(config.C.ReaderFrom))
	if rf == "" || rf == "config" {
		return initFromConfig()
	}

	if rf == "database" {
		return initFromDatabase()
	}

	return fmt.Errorf("invalid configuration ReaderFrom: %s", rf)
}

func initFromConfig() error {
	opts := config.C.Reader

	if opts.Url == "" {
		logger.Warning("reader url is blank")
		return nil
	}

	cli, err := api.NewClient(api.Config{
		Address: opts.Url,
		RoundTripper: &http.Transport{
			// TLSClientConfig: tlsConfig,
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout: time.Duration(opts.DialTimeout) * time.Millisecond,
			}).DialContext,
			ResponseHeaderTimeout: time.Duration(opts.Timeout) * time.Millisecond,
			MaxIdleConnsPerHost:   opts.MaxIdleConnsPerHost,
		},
	})

	if err != nil {
		return err
	}

	Client = prom.NewAPI(cli, prom.ClientOptions{
		BasicAuthUser: opts.BasicAuthUser,
		BasicAuthPass: opts.BasicAuthPass,
		Headers:       opts.Headers,
	})

	return nil
}

func initFromDatabase() error {
	go func() {
		for {
			loadFromDatabase()
			time.Sleep(time.Second)
		}
	}()
	return nil
}

type PromOption struct {
	Url                 string
	User                string
	Pass                string
	Headers             []string
	Timeout             int64
	DialTimeout         int64
	MaxIdleConnsPerHost int
}

func (po *PromOption) Equal(target PromOption) bool {
	if po.Url != target.Url {
		return false
	}

	if po.User != target.User {
		return false
	}

	if po.Pass != target.Pass {
		return false
	}

	if po.Timeout != target.Timeout {
		return false
	}

	if po.DialTimeout != target.DialTimeout {
		return false
	}

	if po.MaxIdleConnsPerHost != target.MaxIdleConnsPerHost {
		return false
	}

	if len(po.Headers) != len(target.Headers) {
		return false
	}

	for i := 0; i < len(po.Headers); i++ {
		if po.Headers[i] != target.Headers[i] {
			return false
		}
	}

	return true
}

func loadFromDatabase() {
	cluster, err := models.AlertingEngineGetCluster(config.C.Heartbeat.Endpoint)
	if err != nil {
		logger.Errorf("failed to get current cluster, error: %v", err)
		return
	}

	ckey := "prom." + cluster + ".option"
	cval, err := models.ConfigsGet(ckey)
	if err != nil {
		logger.Errorf("failed to get ckey: %s, error: %v", ckey, err)
		return
	}

	if cval == "" {
		Client = nil
		return
	}

	var po PromOption
	err = json.Unmarshal([]byte(cval), &po)
	if err != nil {
		logger.Errorf("failed to unmarshal PromOption: %s", err)
		return
	}

	if Client == nil || !LocalPromOption.Equal(po) {
		cli, err := api.NewClient(api.Config{
			Address: po.Url,
			RoundTripper: &http.Transport{
				// TLSClientConfig: tlsConfig,
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout: time.Duration(po.DialTimeout) * time.Millisecond,
				}).DialContext,
				ResponseHeaderTimeout: time.Duration(po.Timeout) * time.Millisecond,
				MaxIdleConnsPerHost:   po.MaxIdleConnsPerHost,
			},
		})

		if err != nil {
			logger.Errorf("failed to NewPromClient: %v", err)
			return
		}

		Client = prom.NewAPI(cli, prom.ClientOptions{
			BasicAuthUser: po.User,
			BasicAuthPass: po.Pass,
			Headers:       po.Headers,
		})
	}
}
