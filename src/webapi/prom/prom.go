package prom

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/didi/nightingale/v5/src/models"
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

type PromOption struct {
	Url                 string
	User                string
	Pass                string
	Headers             []string
	Timeout             int64
	DialTimeout         int64
	MaxIdleConnsPerHost int
}

func (cs *ClustersType) Put(name string, cluster *ClusterType) {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	cs.datas[name] = cluster

	// 把配置信息写入DB一份，这样n9e-server就可以直接从DB读取了
	po := PromOption{
		Url:                 cluster.Opts.Prom,
		User:                cluster.Opts.BasicAuthUser,
		Pass:                cluster.Opts.BasicAuthPass,
		Headers:             cluster.Opts.Headers,
		Timeout:             cluster.Opts.Timeout,
		DialTimeout:         cluster.Opts.DialTimeout,
		MaxIdleConnsPerHost: cluster.Opts.MaxIdleConnsPerHost,
	}

	bs, err := json.Marshal(po)
	if err != nil {
		logger.Fatal("failed to marshal PromOption:", err)
		return
	}

	key := "prom." + name + ".option"
	err = models.ConfigsSet(key, string(bs))
	if err != nil {
		logger.Fatal("failed to set PromOption ", key, " to database, error: ", err)
	}
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
		if cluster == nil {
			continue
		}
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
				Headers           map[string]string `json:"prometheus.headers"`
				PrometheusTimeout int64             `json:"prometheus.timeout"`
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
			old.Opts.Prom != item.Settings.PrometheusAddr ||
			!equalHeader(old.Opts.Headers, transformHeader(item.Settings.Headers)) {
			opt := config.ClusterOptions{
				Name:                item.Name,
				Prom:                item.Settings.PrometheusAddr,
				BasicAuthUser:       item.Settings.PrometheusBasic.PrometheusUser,
				BasicAuthPass:       item.Settings.PrometheusBasic.PrometheusPass,
				Timeout:             item.Settings.PrometheusTimeout,
				DialTimeout:         5000,
				MaxIdleConnsPerHost: 32,
				Headers:             transformHeader(item.Settings.Headers),
			}

			if strings.HasPrefix(opt.Prom, "https") {
				opt.UseTLS = true
				opt.InsecureSkipVerify = true
			}

			cluster := newClusterByOption(opt)
			if cluster == nil {
				continue
			}

			Clusters.Put(item.Name, cluster)
			continue
		}
	}
}

func newClusterByOption(opt config.ClusterOptions) *ClusterType {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: time.Duration(opt.DialTimeout) * time.Millisecond,
		}).DialContext,
		ResponseHeaderTimeout: time.Duration(opt.Timeout) * time.Millisecond,
		MaxIdleConnsPerHost:   opt.MaxIdleConnsPerHost,
	}

	if opt.UseTLS {
		tlsConfig, err := opt.TLSConfig()
		if err != nil {
			logger.Errorf("new cluster %s fail: %v", opt.Name, err)
			return nil
		}
		transport.TLSClientConfig = tlsConfig
	}

	cli, err := api.NewClient(api.Config{
		Address:      opt.Prom,
		RoundTripper: transport,
	})

	if err != nil {
		logger.Errorf("new client fail: %v", err)
		return nil
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

func equalHeader(a, b []string) bool {
	sort.Strings(a)
	sort.Strings(b)

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func transformHeader(header map[string]string) []string {
	var headers []string
	for k, v := range header {
		headers = append(headers, k)
		headers = append(headers, v)
	}
	return headers
}
