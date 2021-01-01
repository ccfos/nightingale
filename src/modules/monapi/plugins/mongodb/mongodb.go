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

type RedisRule struct {
}

func (p *RedisRule) Validate() error {
	return nil
}

func (p *RedisRule) TelegrafInput() (telegraf.Input, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}

	return &redis.Redis{}, nil
}
