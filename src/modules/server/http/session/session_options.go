package session

import (
	"context"
)

type options struct {
	ctx    context.Context
	cancel context.CancelFunc
	mem    bool
}

type Option interface {
	apply(*options)
}

type funcOption struct {
	f func(*options)
}

func (p *funcOption) apply(opt *options) {
	p.f(opt)
}

func newFuncOption(f func(*options)) *funcOption {
	return &funcOption{
		f: f,
	}
}

func WithCtx(ctx context.Context) Option {
	return newFuncOption(func(o *options) {
		o.ctx = ctx
		o.cancel = nil
	})
}

func WithMem() Option {
	return newFuncOption(func(o *options) {
		o.mem = true
	})
}
