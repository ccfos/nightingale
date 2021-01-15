package cache

import (
	"context"
	"sync"
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/toolkits/pkg/logger"
)

type configCache struct {
	sync.RWMutex
	authConfig *models.AuthConfig
}

func (p *configCache) AuthConfig() *models.AuthConfig {
	p.RLock()
	defer p.RUnlock()
	return p.authConfig
}

func (p *configCache) loop(ctx context.Context, interval int) {
	if err := p.update(); err != nil {
		logger.Errorf("configCache update err %s", err)
	}

	go func() {
		t := time.NewTicker(time.Duration(interval) * time.Second)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := p.update(); err != nil {
					logger.Errorf("configCache update err %s", err)
				}
			}
		}
	}()
}

func (p *configCache) update() error {
	authConfig, err := models.AuthConfigGet()
	if err != nil {
		return err
	}

	p.Lock()
	p.authConfig = authConfig
	p.Unlock()

	return nil
}
