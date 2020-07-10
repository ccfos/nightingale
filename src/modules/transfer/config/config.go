package config

import (
	"bytes"
	"fmt"
	"github.com/didi/nightingale/src/toolkits/identity"
	"github.com/didi/nightingale/src/toolkits/report"
	"strings"

	"github.com/didi/nightingale/src/modules/transfer/backend"
	"github.com/didi/nightingale/src/modules/transfer/backend/tsdb"
	"github.com/didi/nightingale/src/toolkits/logger"

	"github.com/spf13/viper"
	"github.com/toolkits/pkg/file"
)

type ConfYaml struct {
	Debug    bool                     `yaml:"debug"`
	MinStep  int                      `yaml:"minStep"`
	Logger   logger.LoggerSection     `yaml:"logger"`
	Backend  backend.BackendSection   `yaml:"backend"`
	HTTP     HTTPSection              `yaml:"http"`
	RPC      RPCSection               `yaml:"rpc"`
	Identity identity.IdentitySection `yaml:"identity"`
	Report   report.ReportSection     `yaml:"report"`
}

type IndexSection struct {
	Path    string `yaml:"path"`
	Timeout int    `yaml:"timeout"`
}

type LoggerSection struct {
	Dir       string `yaml:"dir"`
	Level     string `yaml:"level"`
	KeepHours uint   `yaml:"keepHours"`
}

type HTTPSection struct {
	Enabled bool   `yaml:"enabled"`
	Access  string `yaml:"access"`
}

type RPCSection struct {
	Enabled bool `yaml:"enabled"`
}

var (
	Config *ConfYaml
)

func NewClusterNode(addrs []string) *tsdb.ClusterNode {
	return &tsdb.ClusterNode{Addrs: addrs}
}

// map["node"]="host1,host2" --> map["node"]=["host1", "host2"]
func formatClusterItems(cluster map[string]string) map[string]*tsdb.ClusterNode {
	ret := make(map[string]*tsdb.ClusterNode)
	for node, clusterStr := range cluster {
		items := strings.Split(clusterStr, ",")
		nitems := make([]string, 0)
		for _, item := range items {
			nitems = append(nitems, strings.TrimSpace(item))
		}
		ret[node] = NewClusterNode(nitems)
	}

	return ret
}

func Parse(conf string) error {
	bs, err := file.ReadBytes(conf)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", conf, err)
	}

	viper.SetConfigType("yaml")
	err = viper.ReadConfig(bytes.NewBuffer(bs))
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", conf, err)
	}

	viper.SetDefault("http.enabled", true)
	viper.SetDefault("minStep", 1)

	viper.SetDefault("backend", map[string]interface{}{
		"datasource": "tsdb",
		"straPath":   "/api/portal/stras/effective?all=1",
	})

	viper.SetDefault("backend.judge", map[string]interface{}{
		"batch":       200, //每次拉取文件的个数
		"workerNum":   32,
		"maxConns":    2000, //查询和推送数据的并发个数
		"maxIdle":     32,   //建立的连接池的最大空闲数
		"connTimeout": 1000, //链接超时时间，单位毫秒
		"callTimeout": 3000, //访问超时时间，单位毫秒
		"hbsMod":      "monapi",
	})

	viper.SetDefault("backend.tsdb", map[string]interface{}{
		"enabled":      true,
		"name":         "tsdb",
		"batch":        200, //每次拉取文件的个数
		"workerNum":    32,
		"maxConns":     2000, //查询和推送数据的并发个数
		"maxIdle":      32,   //建立的连接池的最大空闲数
		"connTimeout":  1000, //链接超时时间，单位毫秒
		"callTimeout":  3000, //访问超时时间，单位毫秒
		"indexTimeout": 3000, //访问index超时时间，单位毫秒
		"replicas":     500,  //一致性hash虚拟节点
	})

	viper.SetDefault("backend.influxdb", map[string]interface{}{
		"enabled":   false,
		"name":      "influxdb",
		"batch":     200, //每次拉取文件的个数
		"maxRetry":  3,   //重试次数
		"workerNum": 32,
		"maxConns":  2000, //查询和推送数据的并发个数
		"timeout":   3000, //访问超时时间，单位毫秒
	})

	viper.SetDefault("backend.opentsdb", map[string]interface{}{
		"enabled":     false,
		"name":        "opentsdb",
		"batch":       200, //每次拉取文件的个数
		"maxRetry":    3,   //重试次数
		"workerNum":   32,
		"maxConns":    2000, //查询和推送数据的并发个数
		"maxIdle":     32,   //建立的连接池的最大空闲数
		"connTimeout": 1000, //链接超时时间，单位毫秒
		"callTimeout": 3000, //访问超时时间，单位毫秒
	})

	viper.SetDefault("backend.kafka", map[string]interface{}{
		"enabled":     false,
		"name":        "kafka",
		"maxRetry":    3,    //重试次数
		"connTimeout": 1000, //链接超时时间，单位毫秒
		"callTimeout": 3000, //访问超时时间，单位毫秒
	})

	viper.SetDefault("report", map[string]interface{}{
		"mod":      "transfer",
		"enabled":  true,
		"interval": 4000,
		"timeout":  3000,
		"api":      "api/hbs/heartbeat",
		"remark":   "",
	})

	err = viper.Unmarshal(&Config)
	if err != nil {
		return fmt.Errorf("cannot read yml[%s]: %v", conf, err)
	}

	Config.Backend.Tsdb.ClusterList = formatClusterItems(Config.Backend.Tsdb.Cluster)

	return err
}
