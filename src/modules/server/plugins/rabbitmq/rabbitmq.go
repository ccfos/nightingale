package rabbitmq

import (
	"fmt"
	"reflect"
	"time"

	"github.com/didi/nightingale/v4/src/common/i18n"
	"github.com/didi/nightingale/v4/src/modules/server/collector"
	"github.com/didi/nightingale/v4/src/modules/server/plugins"

	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs/rabbitmq"
)

func init() {
	collector.CollectorRegister(NewRabbitMQCollector()) // for monapi
	i18n.DictRegister(langDict)
}

type RabbitMQCollector struct {
	*collector.BaseCollector
}

func NewRabbitMQCollector() *RabbitMQCollector {
	return &RabbitMQCollector{BaseCollector: collector.NewBaseCollector(
		"rabbitMQ",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &RabbitMQRule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"URL":              "URL",
			"Name":             "节点名称",
			"Username":         "用户名",
			"Password":         "密码",
			"header time out":  "请求超时时间",
			"client time out":  "连接超时时间",
			"nodes":            "MQ节点",
			"queues":           "队列",
			"exchanges":        "Exchange交换机",
			"QueueNameInclude": "包含队列",
			"QueueNameExclude": "排除队列",
		},
	}
)

type RabbitMQRule struct {
	URL                       string   `label:"URL" json:"url,required" example:"http://localhost:15672"`
	Name                      string   `label:"Name" json:"Name" description:"Tag added to rabbitmq_overview series"`
	Username                  string   `label:"Username" json:"username,required" description:"specify username"`
	Password                  string   `label:"Password" json:"password,required" format:"password" description:"specify server password"`
	ResponseHeaderTimeout     int      `label:"header time out" json:"header_timeout" default:"3" description:"for a server's response headers after fully writing the request"`
	ClientTimeout             int      `label:"client time out" json:"client_timeout" default:"4" description:"for a server's response headers after fully writing the request"`
	Nodes                     []string `label:"nodes" json:"nodes" description:"A list of nodes to gather as the rabbitmq_node measurement"`
	Queues                    []string `label:"queues" json:"queues" description:"A list of queues to gather as the rabbitmq_queue measurement"`
	Exchanges                 []string `label:"exchanges" json:"exchanges" description:"A list of exchanges to gather as the rabbitmq_exchange measurement"`
	QueueNameInclude          []string `label:"queue name include" json:"queue_name_include" description:"Queues to include."`
	QueueNameExclude          []string `label:"queue name exclude" json:"queue_name_exclude" description:"Queues to exclude."`
	FederationUpstreamInclude []string `label:"FederationUpstreamInclude" json:"federation_upstream_include" description:"exchange filters include"`
	FederationUpstreamExclude []string `label:"FederationUpstreamExclude" json:"federation_upstream_exclude" description:"exchange filters exclude"`
	plugins.ClientConfig
}

func (p *RabbitMQRule) Validate() error {
	if len(p.URL) == 0 || p.URL == "" {
		return fmt.Errorf("rabbitmq.rule.servers must be set")
	}
	return nil
}

func (p *RabbitMQRule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	mq := &rabbitmq.RabbitMQ{

		URL:          p.URL,
		Name:         p.Name,
		Username:     p.Username,
		Password:     p.Password,
		Nodes:        p.Nodes,
		Queues:       p.Queues,
		Exchanges:    p.Exchanges,
		QueueInclude: p.QueueNameInclude,
		QueueExclude: p.QueueNameExclude,
		ClientConfig: p.ClientConfig.TlsClientConfig(),
	}
	v := reflect.ValueOf(&(mq.ResponseHeaderTimeout.Duration)).Elem()
	v.Set(reflect.ValueOf(time.Second * time.Duration(p.ResponseHeaderTimeout)))
	v1 := reflect.ValueOf(&(mq.ClientTimeout.Duration)).Elem()
	v1.Set(reflect.ValueOf(time.Second * time.Duration(p.ClientTimeout)))
	return mq, nil
}
