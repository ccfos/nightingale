package memsto

import (
	"fmt"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/dumper"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

type MessageTemplateCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	templates map[int64]*models.MessageTemplate // key: template id
}

func NewMessageTemplateCache(ctx *ctx.Context, stats *Stats) *MessageTemplateCacheType {
	mtc := &MessageTemplateCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		templates:       make(map[int64]*models.MessageTemplate),
	}
	mtc.SyncMessageTemplates()
	return mtc
}

func (mtc *MessageTemplateCacheType) Reset() {
	mtc.Lock()
	defer mtc.Unlock()

	mtc.statTotal = -1
	mtc.statLastUpdated = -1
	mtc.templates = make(map[int64]*models.MessageTemplate)
}

func (mtc *MessageTemplateCacheType) StatChanged(total, lastUpdated int64) bool {
	if mtc.statTotal == total && mtc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (mtc *MessageTemplateCacheType) Set(m map[int64]*models.MessageTemplate, total, lastUpdated int64) {
	mtc.Lock()
	mtc.templates = m
	mtc.Unlock()

	// only one goroutine used, so no need lock
	mtc.statTotal = total
	mtc.statLastUpdated = lastUpdated
}

func (mtc *MessageTemplateCacheType) Get(templateId int64) *models.MessageTemplate {
	mtc.RLock()
	defer mtc.RUnlock()
	return mtc.templates[templateId]
}

func (mtc *MessageTemplateCacheType) GetTemplateIds() []int64 {
	mtc.RLock()
	defer mtc.RUnlock()

	count := len(mtc.templates)
	list := make([]int64, 0, count)
	for templateId := range mtc.templates {
		list = append(list, templateId)
	}

	return list
}

func (mtc *MessageTemplateCacheType) SyncMessageTemplates() {
	err := mtc.syncMessageTemplates()
	if err != nil {
		fmt.Println("failed to sync message templates:", err)
		exit(1)
	}

	go mtc.loopSyncMessageTemplates()
}

func (mtc *MessageTemplateCacheType) loopSyncMessageTemplates() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := mtc.syncMessageTemplates(); err != nil {
			logger.Warning("failed to sync message templates:", err)
		}
	}
}

func (mtc *MessageTemplateCacheType) syncMessageTemplates() error {
	start := time.Now()
	stat, err := models.MessageTemplateStatistics(mtc.ctx)
	if err != nil {
		dumper.PutSyncRecord("message_templates", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec MessageTemplateStatistics")
	}

	if !mtc.StatChanged(stat.Total, stat.LastUpdated) {
		mtc.stats.GaugeCronDuration.WithLabelValues("sync_message_templates").Set(0)
		mtc.stats.GaugeSyncNumber.WithLabelValues("sync_message_templates").Set(0)
		dumper.PutSyncRecord("message_templates", start.Unix(), -1, -1, "not changed")
		return nil
	}

	lst, err := models.MessageTemplateGetsAll(mtc.ctx)
	if err != nil {
		dumper.PutSyncRecord("message_templates", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to exec MessageTemplateGetsAll")
	}

	m := make(map[int64]*models.MessageTemplate)
	for i := 0; i < len(lst); i++ {
		m[lst[i].ID] = lst[i]
	}

	mtc.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	mtc.stats.GaugeCronDuration.WithLabelValues("sync_message_templates").Set(float64(ms))
	mtc.stats.GaugeSyncNumber.WithLabelValues("sync_message_templates").Set(float64(len(m)))
	logger.Infof("timer: sync message templates done, cost: %dms, number: %d", ms, len(m))
	dumper.PutSyncRecord("message_templates", start.Unix(), ms, len(m), "success")

	return nil
}
