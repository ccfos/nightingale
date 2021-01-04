package cache

import (
	"context"
	"sync"
	"time"

	"github.com/toolkits/pkg/logger"
)

type sessionCache struct {
	sync.RWMutex
}

func (p *sessionCache) loop(ctx context.Context, interval int) {
	func() {
		t := time.NewTicker(time.Duration(interval) * time.Second)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := p.update(); err != nil {
					logger.Errorf("sessionCache update err %s", err)
				}
			}
		}
	}()
}

func (p *sessionCache) update() error {
	return nil
}
