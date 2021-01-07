package session

import (
	"fmt"
	"sync"
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/rdb/cache"
	"github.com/didi/nightingale/src/modules/rdb/config"
)

func newMemStorage(cf *config.SessionSection, opts *options) (storage, error) {
	st := &mStorage{
		data:   make(map[string]*models.Session),
		opts:   opts,
		config: cf,
	}

	go func() {
		t := time.NewTicker(time.Second * time.Duration(cf.GcInterval))
		defer t.Stop()
		for {
			select {
			case <-opts.ctx.Done():
				return
			case <-t.C:
				st.gc()
			}
		}
	}()

	return st, nil
}

type mStorage struct {
	sync.RWMutex
	data map[string]*models.Session

	opts   *options
	config *config.SessionSection
}

func (p *mStorage) all() int {
	p.RLock()
	defer p.RUnlock()
	return len(p.data)
}

func (p *mStorage) get(sid string) (*models.Session, error) {
	p.RLock()
	defer p.RUnlock()
	s, ok := p.data[sid]
	if !ok {
		return nil, fmt.Errorf("sid %s is not found", sid)
	}
	return s, nil
}

func (p *mStorage) insert(s *models.Session) error {
	p.Lock()
	defer p.Unlock()

	p.data[s.Sid] = s
	return nil
}

func (p *mStorage) del(sid string) error {
	p.Lock()
	defer p.Unlock()

	delete(p.data, sid)
	return nil
}

func (p *mStorage) update(s *models.Session) error {
	p.Lock()
	defer p.Unlock()

	p.data[s.Sid] = s
	return nil
}

func (p *mStorage) gc() {
	p.Lock()
	defer p.Unlock()

	idleTime := cache.AuthConfig().MaxConnIdleTime * 60
	if idleTime == 0 {
		return
	}

	expiresAt := time.Now().Unix() - idleTime
	keys := []string{}
	for k, v := range p.data {
		if v.UpdatedAt < expiresAt {
			keys = append(keys, k)
		}
	}

	for _, k := range keys {
		delete(p.data, k)
	}
}
