package provider

import (
	"fmt"
	"sync"

	"github.com/ccfos/nightingale/v6/models"
)

type Registry struct {
	mu        sync.RWMutex
	providers map[string]NotifyChannelProvider // key = ident
}

// requestTypeFallback：ident 未注册时按 request_type 兜底到通用 provider。
// 用于历史库里 ident 写得五花八门、但仍能按 request_type 找到合理 provider 的情况。
var requestTypeFallback = map[string]string{
	"http":      "callback",
	"script":    "script",
	"smtp":      "email",
	"flashduty": "flashduty",
	"pagerduty": "pagerduty",
}

var DefaultRegistry = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]NotifyChannelProvider),
	}
}

// VerifyChannelConfig 查找 Provider 并执行 Check，供 models 通过 VerifyByProvider 回调使用。
//
// 走 Registry.Resolve 而不是精确 ident 匹配：历史/自定义 ident (如 ident=my-webhook,
// request_type=http) 在发送路径能按 request_type 兜底到 callback provider 发送，
// 保存校验若只按 ident 精确查就会把它拦下来，造成「能发不能存」。统一走 Resolve
// 让两条路径看到同一张 provider 视图。
func VerifyChannelConfig(ncc *models.NotifyChannelConfig) error {
	if ncc == nil {
		return fmt.Errorf("nil channel config")
	}
	p, ok := DefaultRegistry.Resolve(ncc)
	if !ok {
		return fmt.Errorf("unsupported channel: ident=%s request_type=%s",
			ncc.Ident, ncc.RequestType)
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

// Resolve 根据 channel 配置找对应 provider：
//  1. 按 ident 精确查；
//  2. 找不到则按 request_type 兜底到通用 provider (callback/script/email/flashduty/pagerduty)；
//  3. 仍找不到返回 (nil, false)。
//
// 取代之前散落在 dispatch / router / models 中的三份 ident 映射逻辑。
func (r *Registry) Resolve(c *models.NotifyChannelConfig) (NotifyChannelProvider, bool) {
	if c == nil {
		return nil, false
	}
	if p, ok := r.Get(c.Ident); ok {
		return p, true
	}
	fallback, ok := requestTypeFallback[c.RequestType]
	if !ok {
		return nil, false
	}
	return r.Get(fallback)
}
