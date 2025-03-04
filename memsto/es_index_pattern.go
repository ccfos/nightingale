package memsto

import (
	"log"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

type EsIndexPatternCacheType struct {
	ctx *ctx.Context

	sync.RWMutex
	indexPattern map[string]*models.EsIndexPattern // key: name
}

func NewEsIndexPatternCacheType(ctx *ctx.Context) *EsIndexPatternCacheType {
	ipc := &EsIndexPatternCacheType{
		ctx:          ctx,
		indexPattern: make(map[string]*models.EsIndexPattern),
	}

	ipc.SyncEsIndexPattern()
	return ipc
}

func (p *EsIndexPatternCacheType) Reset() {
	p.Lock()
	defer p.Unlock()

	p.indexPattern = make(map[string]*models.EsIndexPattern)
}

func (p *EsIndexPatternCacheType) Set(m map[string]*models.EsIndexPattern) {
	p.Lock()
	p.indexPattern = m
	p.Unlock()
}

func (p *EsIndexPatternCacheType) Get(name string) (*models.EsIndexPattern, bool) {
	p.RLock()
	defer p.RUnlock()

	ip, has := p.indexPattern[name]
	return ip, has
}

func (p *EsIndexPatternCacheType) SyncEsIndexPattern() {
	err := p.syncEsIndexPattern()
	if err != nil {
		log.Fatalln("failed to sync targets:", err)
	}

	go p.loopSyncEsIndexPattern()
}

func (p *EsIndexPatternCacheType) loopSyncEsIndexPattern() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := p.syncEsIndexPattern(); err != nil {
			logger.Warning("failed to sync host alert rule targets:", err)
		}
	}
}

func (p *EsIndexPatternCacheType) syncEsIndexPattern() error {
	lst, err := models.EsIndexPatternGets(p.ctx, "")
	if err != nil {
		return err
	}
	m := make(map[string]*models.EsIndexPattern, len(lst))
	for _, p := range lst {
		m[p.Name] = p
	}
	p.Set(m)

	return nil
}
