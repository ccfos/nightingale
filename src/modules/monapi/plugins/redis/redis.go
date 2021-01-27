package redis

import (
	"fmt"
	"strings"

	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/monapi/plugins"
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs/redis"
)

func init() {
	collector.CollectorRegister(NewRedisCollector()) // for monapi
	i18n.DictRegister(langDict)
}

type RedisCollector struct {
	*collector.BaseCollector
}

func NewRedisCollector() *RedisCollector {
	return &RedisCollector{BaseCollector: collector.NewBaseCollector(
		"redis",
		collector.RemoteCategory,
		func() collector.TelegrafPlugin { return &RedisRule{} },
	)}
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"Field":           "名称",
			"Type":            "类型",
			"Servers":         "服务",
			"specify servers": "指定服务器地址",
			"metric type":     "数据类型",
			"Optional. Specify redis commands to retrieve values": "设置服务器命令,采集数据名称",
			"Password":                "密码",
			"specify server password": "服务密码",
			"redis-cli command":       "redis-cli命令，如果参数中带有空格，请以数组方式设置参数",
			"metric name":             "变量名称，采集时会加上前缀 redis_commands_",
		},
	}
)

type RedisCommand struct {
	Command []string `label:"Command" json:"command,required" example:"get sample_key" description:"redis-cli command"`
	Field   string   `label:"Field" json:"field,required" example:"sample_key" description:"metric name"`
	Type    string   `label:"Type" json:"type" enum:"[\"float\", \"integer\"]" default:"float" description:"metric type"`
}

type RedisRule struct {
	Servers  []string        `label:"Servers" json:"servers,required" description:"specify servers" example:"tcp://localhost:6379"`
	Commands []*RedisCommand `label:"Commands" json:"commands" description:"Optional. Specify redis commands to retrieve values"`
	Password string          `label:"Password" json:"password" format:"password" description:"specify server password"`
	plugins.ClientConfig
}

func (p *RedisRule) Validate() error {
	if len(p.Servers) == 0 || p.Servers[0] == "" {
		return fmt.Errorf("redis.rule.servers must be set")
	}
	for i, cmd := range p.Commands {
		if len(cmd.Command) == 0 {
			return fmt.Errorf("redis.rule.commands[%d].command must be set", i)
		}

		var command []string
		for i, cmd := range cmd.Command {
			if i == 0 {
				for _, v := range strings.Fields(cmd) {
					command = append(command, v)
				}
				continue
			}

			command = append(command, cmd)
		}
		cmd.Command = command

		if cmd.Field == "" {
			return fmt.Errorf("redis.rule.commands[%d].field must be set", i)
		}
		if cmd.Type == "" {
			cmd.Type = "float"
		}
	}
	return nil
}

func (p *RedisRule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	commands := make([]*redis.RedisCommand, len(p.Commands))
	for i, c := range p.Commands {
		cmd := &redis.RedisCommand{
			Field: c.Field,
			Type:  c.Type,
		}
		for _, v := range c.Command {
			cmd.Command = append(cmd.Command, v)
		}
		commands[i] = cmd
	}

	return &redis.Redis{
		Servers:      p.Servers,
		Commands:     commands,
		Password:     p.Password,
		Log:          plugins.GetLogger(),
		ClientConfig: p.ClientConfig.TlsClientConfig(),
	}, nil
}
