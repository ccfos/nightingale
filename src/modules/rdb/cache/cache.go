package cache

import (
	"context"

	"github.com/didi/nightingale/src/models"
)

var (
	DefaultCache = &Cache{
		interval: 10, // Seconds
		config: configCache{
			authConfig: &models.AuthConfig{
				MaxNumErr:          0,
				MaxOccurs:          0,
				MaxConnIdelTime:    0,
				LockTime:           0,
				PwdMinLenght:       0,
				PwdExpiresIn:       0,
				PwdIncludeUpper:    0,
				PwdIncludeLower:    0,
				PwdIncludeNumber:   0,
				PwdIncludeSpecChar: 0,
			},
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

func Start() {
	DefaultCache.Start()
}

func Stop() {
	DefaultCache.Stop()
}

type Cache struct {
	session sessionCache
	config  configCache

	interval int
	ctx      context.Context
	cancel   context.CancelFunc
}

func (p *Cache) Start() {
	p.ctx, p.cancel = context.WithCancel(context.Background())

	p.config.loop(p.ctx, p.interval)
	// p.session.loop(ctx, p.interval)
}

func (p *Cache) Stop() {
	p.cancel()
}
