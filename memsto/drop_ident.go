package memsto

import (
	"sync"
	"time"
)

type Item struct {
	Count int
	Ts    int64
}

type IdentCountCacheType struct {
	sync.RWMutex
	idents map[string]Item
}

func NewIdentCountCache() *IdentCountCacheType {
	d := &IdentCountCacheType{
		idents: make(map[string]Item),
	}
	go d.CronDeleteExpired()
	return d
}

// Set ident
func (c *IdentCountCacheType) Set(ident string, count int, ts int64) {
	c.Lock()
	item := Item{
		Count: count,
		Ts:    ts,
	}
	c.idents[ident] = item
	c.Unlock()
}

func (c *IdentCountCacheType) Increment(ident string, num int) {
	now := time.Now().Unix()
	c.Lock()
	if item, exists := c.idents[ident]; exists {
		item.Count += num
		item.Ts = now
		c.idents[ident] = item
	} else {
		item := Item{
			Count: num,
			Ts:    now,
		}
		c.idents[ident] = item
	}
	c.Unlock()
}

// check exists ident
func (c *IdentCountCacheType) Exists(ident string) bool {
	c.RLock()
	_, exists := c.idents[ident]
	c.RUnlock()
	return exists
}

func (c *IdentCountCacheType) Get(ident string) int {
	c.RLock()
	defer c.RUnlock()
	item, exists := c.idents[ident]
	if !exists {
		return 0
	}
	return item.Count
}

func (c *IdentCountCacheType) GetsAndFlush() map[string]Item {
	c.Lock()
	data := make(map[string]Item)
	for k, v := range c.idents {
		data[k] = v
	}
	c.idents = make(map[string]Item)
	c.Unlock()
	return data
}

func (c *IdentCountCacheType) CronDeleteExpired() {
	for {
		time.Sleep(60 * time.Second)
		c.deleteExpired()
	}
}

// cron delete expired ident
func (c *IdentCountCacheType) deleteExpired() {
	c.Lock()
	now := time.Now().Unix()
	for ident, item := range c.idents {
		if item.Ts < now-120 {
			delete(c.idents, ident)
		}
	}
	c.Unlock()
}
