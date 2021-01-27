package prometheus

import (
	"fmt"
	"time"

	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"github.com/didi/nightingale/src/modules/monapi/plugins/prometheus/prometheus"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
)

func init() {
	collector.CollectorRegister(NewPrometheusCollector()) // for monapi
	i18n.DictRegister(langDict)
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"URLs": "网址",
			"An array of urls to scrape metrics from": "采集数据的网址",
			"URL Tag": "网址标签",
			"Url tag name (tag containing scrapped url. optional, default is \"url\")": "url 标签名称，默认值 \"url\"",
			"An array of Kubernetes services to scrape metrics from":                   "采集kube服务的地址",
			"Kubernetes config file contenct to create client from":                    "kube config 文件内容，用来连接kube服务",
			"Use bearer token for authorization. ('bearer_token' takes priority)":      "用户的Bearer令牌，优先级高于 username/password",
			"HTTP Basic Authentication username":                                       "HTTP认证用户名",
			"HTTP Basic Authentication password":                                       "HTTP认证密码",
			"RESP Timeout":                                                             "请求超时时间",
			"Specify timeout duration for slower prometheus clients":                   "k8s请求超时时间, 单位: 秒",
		},
	}
)

type PrometheusCollector struct {
	*collector.BaseCollector
}

func NewPrometheusCollector() *PrometheusCollector {
	return &PrometheusCollector{BaseCollector: collector.NewBaseCollector(
		"prometheus",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &PrometheusRule{} },
	)}
}

type PrometheusRule struct {
	URLs []string `label:"URLs" json:"urls,required" description:"An array of urls to scrape metrics from" example:"http://my-service-exporter:8080/metrics"`
	// URLTag string   `label:"URL Tag" json:"url_tag" description:"Url tag name (tag containing scrapped url. optional, default is \"url\")" example:"scrapeUrl"`
	// KubernetesServices      []string `label:"Kube Services" json:"kubernetes_services" description:"An array of Kubernetes services to scrape metrics from" example:"http://my-service-dns.my-namespace:9100/metrics"`
	// KubeConfigContent       string   `label:"Kube Conf" json:"kube_config_content" format:"file" description:"Kubernetes config file contenct to create client from"`
	// MonitorPods             bool     `label:"Monitor Pods" json:"monitor_kubernetes_pods" description:"Scrape Kubernetes pods for the following prometheus annotations:<br />- prometheus.io/scrape: Enable scraping for this pod<br />- prometheus.io/scheme: If the metrics endpoint is secured then you will need to<br />    set this to 'https' & most likely set the tls config.<br />- prometheus.io/path: If the metrics path is not /metrics, define it with this annotation.<br />- prometheus.io/port: If port is not 9102 use this annotation"`
	// PodNamespace            string   `label:"Pod Namespace" json:"monitor_kubernetes_pods_namespace" description:"Restricts Kubernetes monitoring to a single namespace" example:"default"`
	// KubernetesLabelSelector string   `label:"Kube Label Selector" json:"kubernetes_label_selector" description:"label selector to target pods which have the label" example:"env=dev,app=nginx"`
	// KubernetesFieldSelector string   `label:"Kube Field Selector" json:"kubernetes_field_selector" description:"field selector to target pods<br />eg. To scrape pods on a specific node" example:"spec.nodeName=$HOSTNAME"`
	// BearerTokenString       string   `label:"Bearer Token" json:"bearer_token_string" format:"file" description:"Use bearer token for authorization. ('bearer_token' takes priority)"`
	// Username                string   `label:"Username" json:"username" description:"HTTP Basic Authentication username"`
	// Password                string   `label:"Password" json:"password" format:"password" description:"HTTP Basic Authentication password"`
	ResponseTimeout int `label:"RESP Timeout" json:"response_timeout" default:"3" description:"Specify timeout duration for slower prometheus clients"`
	plugins.ClientConfig
}

func (p *PrometheusRule) Validate() error {
	if len(p.URLs) == 0 || p.URLs[0] == "" {
		return fmt.Errorf(" prometheus.rule unable to get urls")
	}
	return nil
}

func (p *PrometheusRule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	return &prometheus.Prometheus{
		URLs:   p.URLs,
		URLTag: "target",
		// KubernetesServices:      p.KubernetesServices,
		// KubeConfigContent:       p.KubeConfigContent,
		// MonitorPods:             p.MonitorPods,
		// PodNamespace:            p.PodNamespace,
		// KubernetesLabelSelector: p.KubernetesLabelSelector,
		// KubernetesFieldSelector: p.KubernetesFieldSelector,
		// BearerTokenString:       p.BearerTokenString,
		// Username:                p.Username,
		// Password:                p.Password,
		ResponseTimeout: time.Second * time.Duration(p.ResponseTimeout),
		MetricVersion:   2,
		Log:             plugins.GetLogger(),
		ClientConfig:    p.ClientConfig.TlsClientConfig(),
	}, nil
}
