package prom

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/didi/nightingale/v5/src/pkg/prom"
	"github.com/didi/nightingale/v5/src/webapi/config"
	"github.com/prometheus/client_golang/api"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
)

type ClusterType struct {
	Opts       config.ClusterOptions
	Transport  *http.Transport
	PromClient prom.API
}

type ClustersType struct {
	datas map[string]*ClusterType
	mutex *sync.RWMutex
}

func (cs *ClustersType) Put(name string, cluster *ClusterType) {
	cs.mutex.Lock()
	cs.datas[name] = cluster
	cs.mutex.Unlock()
}

func (cs *ClustersType) Get(name string) (*ClusterType, bool) {
	cs.mutex.RLock()
	c, has := cs.datas[name]
	cs.mutex.RUnlock()
	return c, has
}

var Clusters = ClustersType{
	datas: make(map[string]*ClusterType),
	mutex: new(sync.RWMutex),
}

func Init() error {
	cf := strings.ToLower(strings.TrimSpace(config.C.ClustersFrom))
	if cf == "" || cf == "config" {
		return initClustersFromConfig()
	}

	if cf == "api" {
		return initClustersFromAPI()
	}

	return fmt.Errorf("invalid configuration ClustersFrom: %s", cf)
}

func initClustersFromConfig() error {
	opts := config.C.Clusters

	for i := 0; i < len(opts); i++ {
		cluster := newClusterByOption(opts[i])
		Clusters.Put(opts[i].Name, cluster)
	}

	return nil
}

type DSReply struct {
	RequestID string `json:"request_id"`
	Data      struct {
		Items []struct {
			Name     string `json:"name"`
			Settings struct {
				PrometheusAddr  string `json:"prometheus.addr"`
				PrometheusBasic struct {
					PrometheusUser string `json:"prometheus.user"`
					PrometheusPass string `json:"prometheus.password"`
				} `json:"prometheus.basic"`
				PrometheusTimeout int64 `json:"prometheus.timeout"`
			} `json:"settings,omitempty"`
		} `json:"items"`
	} `json:"data"`
}

func initClustersFromAPI() error {
	go func() {
		for {
			loadClustersFromAPI()
			time.Sleep(time.Second * 3)
		}
	}()
	return nil
}

func loadClustersFromAPI() {
	urls := config.C.ClustersFromAPIs
	if len(urls) == 0 {
		logger.Error("configuration(ClustersFromAPIs) empty")
		return
	}

	var reply DSReply

	count := len(urls)
	for _, i := range rand.Perm(count) {
		url := urls[i]

		res, err := httplib.Post(url).SetTimeout(time.Duration(3000) * time.Millisecond).Response()
		if err != nil {
			logger.Errorf("curl %s fail: %v", url, err)
			continue
		}

		if res.StatusCode != 200 {
			logger.Errorf("curl %s fail, status code: %d", url, res.StatusCode)
			continue
		}

		defer res.Body.Close()

		jsonBytes, err := io.ReadAll(res.Body)
		if err != nil {
			logger.Errorf("read response body of %s fail: %v", url, err)
			continue
		}
		logger.Debugf("curl %s success, response: %s", url, string(jsonBytes))

		err = json.Unmarshal(jsonBytes, &reply)
		if err != nil {
			logger.Errorf("unmarshal response body of %s fail: %v", url, err)
			continue
		}

		break
	}

	for _, item := range reply.Data.Items {
		if item.Settings.PrometheusAddr == "" {
			continue
		}

		if item.Name == "" {
			continue
		}

		old, has := Clusters.Get(item.Name)
		if !has ||
			old.Opts.BasicAuthUser != item.Settings.PrometheusBasic.PrometheusUser ||
			old.Opts.BasicAuthPass != item.Settings.PrometheusBasic.PrometheusPass ||
			old.Opts.Timeout != item.Settings.PrometheusTimeout ||
			old.Opts.Prom != item.Settings.PrometheusAddr {
			opt := config.ClusterOptions{
				Name:                item.Name,
				Prom:                item.Settings.PrometheusAddr,
				BasicAuthUser:       item.Settings.PrometheusBasic.PrometheusUser,
				BasicAuthPass:       item.Settings.PrometheusBasic.PrometheusPass,
				Timeout:             item.Settings.PrometheusTimeout,
				DialTimeout:         5000,
				MaxIdleConnsPerHost: 32,
			}

			Clusters.Put(item.Name, newClusterByOption(opt))
			continue
		}
	}
}

func newClusterByOption(opt config.ClusterOptions) *ClusterType {
	transport := &http.Transport{
		// TLSClientConfig: tlsConfig,
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: time.Duration(opt.DialTimeout) * time.Millisecond,
		}).DialContext,
		ResponseHeaderTimeout: time.Duration(opt.Timeout) * time.Millisecond,
		MaxIdleConnsPerHost:   opt.MaxIdleConnsPerHost,
	}

	cli, err := api.NewClient(api.Config{
		Address:      opt.Prom,
		RoundTripper: transport,
	})

	if err != nil {
		logger.Errorf("new client fail: %v", err)
	}

	cluster := &ClusterType{
		Opts:      opt,
		Transport: transport,
		PromClient: prom.NewAPI(cli, prom.ClientOptions{
			BasicAuthUser: opt.BasicAuthUser,
			BasicAuthPass: opt.BasicAuthPass,
			Headers:       opt.Headers,
		}),
	}

	return cluster
}
