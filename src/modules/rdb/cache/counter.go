package cache

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/didi/nightingale/src/models"
	"xorm.io/xorm"
)

type counter struct {
	sync.RWMutex
	base int64
	inc  int64
}

func (p *counter) Add(a int64) {
	p.Lock()
	defer p.Unlock()
	p.inc += a
}

func (p *counter) Set(a int64) {
}

type counterCache struct {
	login  int64
	login2 int64 //
}

func (p *counterCache) login() {
	atomic.AddInt64(&login, 1)
}

func (p *counterCache) start() {
}
func (p *counterCache) loop(ctx context.Context, interval int) {
}

func getCounter(session *xorm.Session) (*counterCache, error) {
	var obj models.Configs
	has, err := session.Where("ckey=?", "cache.counter").Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return &counterCache{}, nil
	}

	ret := &counterCache{}
	err = json.Unmarshal([]byte(obj.Cval), ret)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func (p *counterCache) update() (err error) {
	session := models.DB["rdb"].NewSession()
	defer session.Close()

	var counter *counterCache
	var login int64

	if counter, err = getCounter(session); err != nil {
		return err
	}

	login = atomic.LoadInt64(&p.login)
	counter.login += login

	defer func() {
		if err != nil {

			session.Rollback()
		} else {
			session.Commit()
		}
	}()

	atomic.AddInt64(&p.login, -login)

	buf, err := models.ConfigsGet("cache.counter")
}
