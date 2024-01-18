package memsto

import (
	"sync"
	"time"
)

type DropIdentCacheType struct {
	sync.RWMutex
	idents map[string]int64
}

func NewDropIdentCache() *DropIdentCacheType {
	d := &DropIdentCacheType{
		idents: make(map[string]int64),
	}
	go d.CronDeleteExpired()
	return d
}

// Set ident
func (c *DropIdentCacheType) Set(ident string, ts int64) {
	c.Lock()
	c.idents[ident] = ts
	c.Unlock()
}

// check exists ident
func (c *DropIdentCacheType) Exists(ident string) bool {
	c.RLock()
	_, exists := c.idents[ident]
	c.RUnlock()
	return exists
}

func (c *DropIdentCacheType) CronDeleteExpired() {
	for {
		time.Sleep(60 * time.Second)
		c.deleteExpired()
	}
}

// cron delete expired ident
func (c *DropIdentCacheType) deleteExpired() {
	c.Lock()
	now := time.Now().Unix()
	for ident, ts := range c.idents {
		if ts < now-120 {
			delete(c.idents, ident)
		}
	}
	c.Unlock()
}
