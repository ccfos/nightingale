package cache

import (
	"context"

	"github.com/didi/nightingale/src/models"
)

var (
	DefaultCache = &Cache{
		interval: 10, // Seconds
		config: configCache{
			authConfig: &models.DefaultAuthConfig,
		},
	}
)

func NewCache(interval int) *Cache {
	return &Cache{
		interval: interval,
	}
}

func AuthConfig() *models.AuthConfig {
	return DefaultCache.config.AuthConfig()
}

func Login() {
	DefaultCache.counter.Login.Add(1)
}

func GetCounter() map[string]interface{} {
	return map[string]interface{}{
		"login": DefaultCache.counter.Login.Get(),
	}
}

func Start() {
	DefaultCache.Start()
}

func Stop() {
	DefaultCache.Stop()
}

type Cache struct {
	session sessionCache
	config  configCache
	counter counterCache

	interval int
	ctx      context.Context
	cancel   context.CancelFunc
}

func (p *Cache) Start() {
	p.ctx, p.cancel = context.WithCancel(context.Background())

	p.config.loop(p.ctx, p.interval)
	p.counter.loop(p.ctx, p.interval)
	// p.session.loop(ctx, p.interval)
}

func (p *Cache) Stop() {
	p.cancel()
}
