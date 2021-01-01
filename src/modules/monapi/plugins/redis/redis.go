package redis

import (
	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/influxdata/telegraf"
	"github.com/influxdata/telegraf/plugins/inputs/redis"
)

func init() {
	collector.CollectorRegister(NewRedisCollector()) // for monapi
}

type RedisCollector struct {
	*collector.BaseCollector
}

func NewRedisCollector() *RedisCollector {
	return &RedisCollector{BaseCollector: collector.NewBaseCollector(
		"redis",
		collector.RemoteCategory,
		func() interface{} { return &RedisRule{} },
	)}
}

type RedisCommand struct {
	Command []string `label:"Command" json:"command,required" description:"" `
	Field   string   `label:"Field" json:"field,required" description:"metric name"`
	Type    string   `label:"Type" json:"type" description:"integer|string|float"`
}

type RedisRule struct {
	Servers  []string        `label:"Servers" json:"servers,required" description:"If no servers are specified, then localhost is used as the host." example:"tcp://localhost:6379"`
	Commands []*RedisCommand `label:"Commands" json:"commands" description:"Optional. Specify redis commands to retrieve values"`
	Password string          `label:"Password" json:"password" description:"specify server password"`
}

func (p *RedisRule) Validate() error {
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
		Servers:  p.Servers,
		Commands: commands,
		Password: p.Password,
	}, nil
}
