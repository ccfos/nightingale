package config

import (
	"bytes"
	"fmt"
	"github.com/didi/nightingale/src/modules/tsdb/backend/rpc"
	"github.com/didi/nightingale/src/modules/tsdb/cache"
	"github.com/didi/nightingale/src/modules/tsdb/index"
	"github.com/didi/nightingale/src/modules/tsdb/migrate"
	"github.com/didi/nightingale/src/modules/tsdb/rrdtool"
	"github.com/didi/nightingale/src/toolkits/logger"

	"github.com/spf13/viper"
	"github.com/toolkits/pkg/file"
)

type File struct {
	Filename string
	Body     []byte
}

type ConfYaml struct {
	Http           *HttpSection           `yaml:"http"`
	Rpc            *RpcSection            `yaml:"rpc"`
	RRD            rrdtool.RRDSection     `yaml:"rrd"`
	Logger         logger.LoggerSection   `yaml:"logger"`
	Migrate        migrate.MigrateSection `yaml:"migrate"`
	Index          index.IndexSection     `yaml:"index"`
	RpcClient      rpc.RpcClientSection   `yaml:"rpcClient"`
	Cache          cache.CacheSection     `yaml:"cache"`
	CallTimeout    int                    `yaml:"callTimeout"`
	IOWorkerNum    int                    `yaml:"ioWorkerNum"`
	FirstBytesSize int                    `yaml:"firstBytesSize"`
}

type HttpSection struct {
	Enabled bool `yaml:"enabled"`
}

type RpcSection struct {
	Enabled bool `yaml:"enabled"`
}

var (
	Config *ConfYaml
)

func GetCfgYml() *ConfYaml {
	return Config
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
	viper.SetDefault("rpc.enabled", true)

	viper.SetDefault("rrd.rra", map[int]int{
		1:    720,  // 原始点，假设10s一个点，则存2h，即720个点
		6:    4320, // 6个点归档为一个点，即1min一个点，3天的话是4320个点
		180:  1440, // 180个点归档为一个点，即30min一个点，1个月30天是1440个点
		1080: 2880, // 1080个点归档为一个点，即6h一个点存1年，按照360天算是2880个点
	})

	viper.SetDefault("rrd.enabled", true)
	viper.SetDefault("rrd.wait", true)
	viper.SetDefault("rrd.enabled", 100)    //每次从待落盘队列中间等待间隔，单位毫秒
	viper.SetDefault("rrd.batch", 100)      //每次从待落盘队列中获取数据的个数
	viper.SetDefault("rrd.concurrency", 20) //每次从待落盘队列中获取数据的个数
	viper.SetDefault("rrd.ioWorkerNum", 64) //同时落盘的io并发个数

	viper.SetDefault("cache.keepMinutes", 120)
	viper.SetDefault("cache.spanInSeconds", 900)   //每个数据块保存数据的时间范围，单位秒
	viper.SetDefault("cache.doCleanInMinutes", 10) //清理过期数据的周期，单位分钟
	viper.SetDefault("cache.flushDiskStepMs", 1000)

	viper.SetDefault("migrate.enabled", false)
	viper.SetDefault("migrate.concurrency", 2)
	viper.SetDefault("migrate.batch", 200)
	viper.SetDefault("migrate.replicas", 500)
	viper.SetDefault("migrate.connTimeout", 1000)
	viper.SetDefault("migrate.callTimeout", 3000)
	viper.SetDefault("migrate.maxConns", 32)
	viper.SetDefault("migrate.maxIdle", 32)

	viper.SetDefault("index.activeDuration", 90000)  //索引最大的保留时间，超过此数值，索引不会被重建，默认是1天+1小时
	viper.SetDefault("index.rebuildInterval", 21600) //重建索引的周期，单位为秒，默认是6h
	viper.SetDefault("index.hbsMod", "monapi")       //获取index心跳的模块

	viper.SetDefault("rpcClient", map[string]int{
		"maxConns":    320,  //查询和推送数据的并发个数
		"maxIdle":     320,  //建立的连接池的最大空闲数
		"connTimeout": 1000, //链接超时时间，单位毫秒
		"callTimeout": 3000, //访问超时时间，单位毫秒
	})

	err = viper.Unmarshal(&Config)
	if err != nil {
		return fmt.Errorf("Unmarshal %v", err)
	}

	return err
}

func GetInt(defaultVal, val int) int {
	if val != 0 {
		return val
	}
	return defaultVal
}

func GetString(defaultVal, val string) string {
	if val != "" {
		return val
	}
	return defaultVal
}

func GetBool(defaultVal, val bool) bool {
	if val != false {
		return val
	}
	return defaultVal
}
