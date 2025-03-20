package pconf

import (
	"log"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/tlsx"

	"github.com/prometheus/common/model"
)

type Pushgw struct {
	BusiGroupLabelKey       string
	IdentMetrics            []string
	IdentStatsThreshold     int
	IdentDropThreshold      int // 每分钟单个 ident 的样本数超过该阈值，则丢弃
	WriteConcurrency        int
	LabelRewrite            bool
	ForceUseServerTS        bool
	QueueSizeOfMetricPrefix uint64 // 对于没有 ident 的监控数据，使用 metric 划分队列，第一版是使用 metric 前俩字符，第二版是 metric 前俩字符 + hash(metric后面字符) % QueueSizeOfMetricPrefix
	DebugSample             map[string]string
	DropSample              []map[string]string
	WriterOpt               WriterGlobalOpt
	Writers                 []WriterOptions
	KafkaWriters            []KafkaWriterOptions
}

type WriterGlobalOpt struct {
	QueueMaxSize            int
	QueuePopSize            int
	AllQueueMaxSize         int
	AllQueueMaxSizeInterval int
	RetryCount              int
	RetryInterval           int64
	OverLimitStatusCode     int
}

type WriterOptions struct {
	Url           string
	BasicAuthUser string
	BasicAuthPass string

	Timeout               int64
	DialTimeout           int64
	TLSHandshakeTimeout   int64
	ExpectContinueTimeout int64
	IdleConnTimeout       int64
	KeepAlive             int64

	MaxConnsPerHost     int
	MaxIdleConns        int
	MaxIdleConnsPerHost int

	Headers []string

	WriteRelabels []*RelabelConfig

	tlsx.ClientConfig

	// writer 是在配置文件中写死的，不支持动态更新，所以启动的时候就初始化好
	// 后面大概率也不需要动态更新，pushgw 甚至想单独拆出来作为一个独立的进程提供服务
	HTTPTransport *http.Transport
}

type SASLConfig struct {
	Enable       bool
	User         string
	Password     string
	Mechanism    string
	Version      int16
	Handshake    bool
	AuthIdentity string
}

type KafkaWriterOptions struct {
	Typ     string
	Brokers []string
	Topic   string
	Version string
	Timeout int64

	SASL *SASLConfig

	WriteRelabels []*RelabelConfig
}

type RelabelConfig struct {
	SourceLabels  model.LabelNames `json:"source_labels"`
	Separator     string           `json:"separator"`
	Regex         string           `json:"regex"`
	RegexCompiled *regexp.Regexp
	If            string `json:"if"`
	IfRegex       *regexp.Regexp
	Modulus       uint64 `json:"modulus"`
	TargetLabel   string `json:"target_label"`
	Replacement   string `json:"replacement"`
	Action        string `json:"action"`
}

func (p *Pushgw) PreCheck() {
	if p.BusiGroupLabelKey == "" {
		p.BusiGroupLabelKey = "busigroup"
	}

	if p.WriterOpt.QueueMaxSize <= 0 {
		p.WriterOpt.QueueMaxSize = 10000000
	}

	if p.WriterOpt.QueuePopSize <= 0 {
		p.WriterOpt.QueuePopSize = 1000
	}

	if p.WriterOpt.AllQueueMaxSize <= 0 {
		p.WriterOpt.AllQueueMaxSize = 5000000
	}

	if p.WriterOpt.AllQueueMaxSizeInterval <= 0 {
		p.WriterOpt.AllQueueMaxSizeInterval = 200
	}

	if p.WriterOpt.RetryCount <= 0 {
		p.WriterOpt.RetryCount = 1000
	}

	if p.WriterOpt.RetryInterval <= 0 {
		p.WriterOpt.RetryInterval = 1
	}

	if p.WriterOpt.OverLimitStatusCode <= 0 {
		p.WriterOpt.OverLimitStatusCode = 499
	}

	if p.WriteConcurrency <= 0 {
		p.WriteConcurrency = 5000
	}

	if p.IdentStatsThreshold <= 0 {
		p.IdentStatsThreshold = 1500
	}

	if p.IdentDropThreshold <= 0 {
		p.IdentDropThreshold = 5000000
	}

	if p.QueueSizeOfMetricPrefix <= 0 {
		p.QueueSizeOfMetricPrefix = 100
	}

	for index := range p.Writers {
		for _, relabel := range p.Writers[index].WriteRelabels {
			if relabel.Regex == "" {
				relabel.Regex = "(.*)"
			}

			regex, err := regexp.Compile("^(?:" + relabel.Regex + ")$")
			if err != nil {
				log.Fatalln("failed to compile regexp:", relabel.Regex, "error:", err)
			}

			relabel.RegexCompiled = regex

			if relabel.Separator == "" {
				relabel.Separator = ";"
			}

			if relabel.Action == "" {
				relabel.Action = "replace"
			}

			if relabel.Replacement == "" {
				relabel.Replacement = "$1"
			}
		}

		tlsConf, err := p.Writers[index].ClientConfig.TLSConfig()
		if err != nil {
			panic(err)
		}

		// 初始化 http transport
		p.Writers[index].HTTPTransport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   time.Duration(p.Writers[index].DialTimeout) * time.Millisecond,
				KeepAlive: time.Duration(p.Writers[index].KeepAlive) * time.Millisecond,
			}).DialContext,
			ResponseHeaderTimeout: time.Duration(p.Writers[index].Timeout) * time.Millisecond,
			TLSHandshakeTimeout:   time.Duration(p.Writers[index].TLSHandshakeTimeout) * time.Millisecond,
			ExpectContinueTimeout: time.Duration(p.Writers[index].ExpectContinueTimeout) * time.Millisecond,
			MaxConnsPerHost:       p.Writers[index].MaxConnsPerHost,
			MaxIdleConns:          p.Writers[index].MaxIdleConns,
			MaxIdleConnsPerHost:   p.Writers[index].MaxIdleConnsPerHost,
			IdleConnTimeout:       time.Duration(p.Writers[index].IdleConnTimeout) * time.Millisecond,
		}

		if tlsConf != nil {
			p.Writers[index].HTTPTransport.TLSClientConfig = tlsConf
		}
	}
}
