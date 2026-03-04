package provider

import (
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]NotifyChannelProvider // key = ident
}

var DefaultRegistry = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]NotifyChannelProvider),
	}
}

func InitNotifyChannel(ctx *ctx.Context) {
	if !ctx.IsCenter {
		return
	}

	for _, p := range DefaultRegistry.All() {
		for _, ch := range p.DefaultChannels() {
			ch.CreateBy = "system"
			ch.CreateAt = time.Now().Unix()
			ch.UpdateBy = "system"
			ch.UpdateAt = time.Now().Unix()
			err := ch.Upsert(ctx)
			if err != nil {
				logger.Warningf("notify channel init failed to upsert notify channels %v", err)
			}
		}
	}
}

// VerifyChannelConfig 按 ident 查找 Provider 并执行 Check，供 models 通过 VerifyByProvider 回调使用。
func VerifyChannelConfig(ncc *models.NotifyChannelConfig) error {
	p, ok := DefaultRegistry.Get(ncc.Ident)
	if !ok {
		return fmt.Errorf("unsupported channel ident: %s", ncc.Ident)
	}
	return p.Check(ncc)
}

func (r *Registry) Register(p NotifyChannelProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers[p.Ident()] = p
}

func (r *Registry) Get(ident string) (NotifyChannelProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[ident]
	return p, ok
}

func (r *Registry) All() []NotifyChannelProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := make([]NotifyChannelProvider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	return providers
}

func (r *Registry) AllDefaultChannels() []*models.NotifyChannelConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	channels := make([]*models.NotifyChannelConfig, 0)
	for _, p := range r.providers {
		channels = append(channels, p.DefaultChannels()...)
	}
	return channels
}
