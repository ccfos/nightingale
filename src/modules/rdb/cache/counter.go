package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/didi/nightingale/src/models"
	"github.com/toolkits/pkg/logger"
	"xorm.io/xorm"
)

type Counter struct {
	sync.RWMutex
	Base int64 `json:"base"`
	Inc  int64 `json:"-"`
}

func (p *Counter) Add(a int64) {
	p.Lock()
	defer p.Unlock()
	p.Inc += a
}

func (p *Counter) Set(a int64) {
	p.Lock()
	defer p.RUnlock()
	p.Base = a
	p.Inc = 0
}

func (p *Counter) Get() int64 {
	p.RLock()
	defer p.RUnlock()
	return p.Base + p.Inc
}

func (p *Counter) LoadAndRstInc() int64 {
	p.Lock()
	defer p.Unlock()

	inc := p.Inc
	p.Inc = 0
	return inc
}

type counterCache struct {
	Login Counter `json:"login"`
}

func (p *counterCache) loop(ctx context.Context, interval int) {
	if err := p.update(); err != nil {
		logger.Errorf("configCache update err %s", err)
	}

	go func() {
		t := time.NewTicker(time.Duration(interval) * time.Second)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				if err := p.update(); err != nil {
					logger.Errorf("configCache update err %s", err)
				}
				return
			case <-t.C:
				if err := p.update(); err != nil {
					logger.Errorf("configCache update err %s", err)
				}
			}
		}
	}()

}

func (p *counterCache) update() (err error) {
	session := models.DB["rdb"].NewSession()
	defer session.Close()

	var counter *counterCache
	if counter, err = getCounter(session); err != nil {
		return err
	}

	inc := p.Login.LoadAndRstInc()
	counter.Login.Base += inc

	if err = setCounter(session, counter); err != nil {
		session.Rollback()
		p.Login.Add(inc)
	} else {
		session.Commit()
	}

	return
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

func setCounter(session *xorm.Session, counter *counterCache) error {
	var obj models.Configs

	buf, err := json.Marshal(counter)
	if err != nil {
		return err
	}

	has, err := session.Where("ckey=?", "cache.counter").Get(&obj)
	if err != nil {
		return err
	}

	if !has {
		_, err = session.Insert(models.Configs{
			Ckey: "cache.counter",
			Cval: string(buf),
		})
	} else {
		obj.Cval = string(buf)
		_, err = session.Where("ckey=?", "cache.counter").Cols("cval").Update(obj)
	}

	return err
}
