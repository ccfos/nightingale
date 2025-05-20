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

type EventProcessorCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats

	sync.RWMutex
	eventPipelines map[int64]*models.EventPipeline // key: pipeline id
}

func NewEventProcessorCache(ctx *ctx.Context, stats *Stats) *EventProcessorCacheType {
	epc := &EventProcessorCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		stats:           stats,
		eventPipelines:  make(map[int64]*models.EventPipeline),
	}
	epc.SyncEventProcessors()
	return epc
}

func (epc *EventProcessorCacheType) Reset() {
	epc.Lock()
	defer epc.Unlock()

	epc.statTotal = -1
	epc.statLastUpdated = -1
	epc.eventPipelines = make(map[int64]*models.EventPipeline)
}

func (epc *EventProcessorCacheType) StatChanged(total, lastUpdated int64) bool {
	if epc.statTotal == total && epc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}

func (epc *EventProcessorCacheType) Set(m map[int64]*models.EventPipeline, total, lastUpdated int64) {
	epc.Lock()
	epc.eventPipelines = m
	epc.Unlock()

	// only one goroutine used, so no need lock
	epc.statTotal = total
	epc.statLastUpdated = lastUpdated
}

func (epc *EventProcessorCacheType) Get(processorId int64) *models.EventPipeline {
	epc.RLock()
	defer epc.RUnlock()
	return epc.eventPipelines[processorId]
}

func (epc *EventProcessorCacheType) GetProcessorsById(processorId int64) []models.Processor {
	epc.RLock()
	defer epc.RUnlock()

	eventPipeline, ok := epc.eventPipelines[processorId]
	if !ok {
		return []models.Processor{}
	}

	return eventPipeline.Processors
}

func (epc *EventProcessorCacheType) GetProcessorIds() []int64 {
	epc.RLock()
	defer epc.RUnlock()

	count := len(epc.eventPipelines)
	list := make([]int64, 0, count)
	for eid := range epc.eventPipelines {
		list = append(list, eid)
	}

	return list
}

func (epc *EventProcessorCacheType) SyncEventProcessors() {
	err := epc.syncEventProcessors()
	if err != nil {
		fmt.Println("failed to sync event processors:", err)
		exit(1)
	}

	go epc.loopSyncEventProcessors()
}

func (epc *EventProcessorCacheType) loopSyncEventProcessors() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := epc.syncEventProcessors(); err != nil {
			logger.Warning("failed to sync event processors:", err)
		}
	}
}

func (epc *EventProcessorCacheType) syncEventProcessors() error {
	start := time.Now()

	stat, err := models.EventPipelineStatistics(epc.ctx)
	if err != nil {
		dumper.PutSyncRecord("event_processors", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec StatisticsGet for EventPipeline")
	}

	if !epc.StatChanged(stat.Total, stat.LastUpdated) {
		epc.stats.GaugeCronDuration.WithLabelValues("sync_event_processors").Set(0)
		epc.stats.GaugeSyncNumber.WithLabelValues("sync_event_processors").Set(0)
		dumper.PutSyncRecord("event_processors", start.Unix(), -1, -1, "not changed")
		return nil
	}

	lst, err := models.ListEventPipelines(epc.ctx)
	if err != nil {
		dumper.PutSyncRecord("event_processors", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to exec ListEventPipelines")
	}

	m := make(map[int64]*models.EventPipeline)
	for i := 0; i < len(lst); i++ {
		eventPipeline := lst[i]
		for _, p := range eventPipeline.ProcessorConfigs {
			processor, err := models.GetProcessorByType(p.Typ, p.Config)
			if err != nil {
				logger.Warningf("event_pipeline_id: %d, event:%+v, processor:%+v type not found", eventPipeline.ID, eventPipeline, p)
				continue
			}

			eventPipeline.Processors = append(eventPipeline.Processors, processor)
		}

		m[lst[i].ID] = eventPipeline
	}

	epc.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	epc.stats.GaugeCronDuration.WithLabelValues("sync_event_processors").Set(float64(ms))
	epc.stats.GaugeSyncNumber.WithLabelValues("sync_event_processors").Set(float64(len(m)))
	logger.Infof("timer: sync event processors done, cost: %dms, number: %d", ms, len(m))
	dumper.PutSyncRecord("event_processors", start.Unix(), ms, len(m), "success")

	return nil
}
