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

type TaskTplCache struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	tpls            map[int64]*models.TaskTpl
	sync.RWMutex
}

func NewTaskTplCache(ctx *ctx.Context) *TaskTplCache {
	ttc := &TaskTplCache{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		tpls:            make(map[int64]*models.TaskTpl),
	}

	ttc.SyncTaskTpl()
	return ttc
}

func (ttc *TaskTplCache) Set(tpls map[int64]*models.TaskTpl, total, lastUpdated int64) {
	ttc.Lock()
	ttc.tpls = tpls
	ttc.Unlock()

	ttc.statTotal = total
	ttc.statLastUpdated = lastUpdated
}

func (ttc *TaskTplCache) Get(id int64) *models.TaskTpl {
	ttc.Lock()
	defer ttc.Unlock()

	return ttc.tpls[id]
}

func (ttc *TaskTplCache) SyncTaskTpl() {
	if err := ttc.syncTaskTpl(); err != nil {
		fmt.Println("failed to sync task tpls:", err)
		exit(1)
	}
	go ttc.loopSyncTaskTpl()
}

func (ttc *TaskTplCache) syncTaskTpl() error {
	start := time.Now()
	stat, err := models.TaskTplStatistics(ttc.ctx)
	if err != nil {
		dumper.PutSyncRecord("task_tpls", start.Unix(), -1, -1, "failed to query statistics: "+err.Error())
		return errors.WithMessage(err, "failed to exec TaskTplStatistics")
	}

	if !ttc.StatChange(stat.Total, stat.LastUpdated) {
		dumper.PutSyncRecord("task_tpls", start.Unix(), -1, -1, "not changed")
		return nil
	}

	lst, err := models.TaskTplGetAll(ttc.ctx)
	if err != nil {
		dumper.PutSyncRecord("task_tpls", start.Unix(), -1, -1, "failed to query records: "+err.Error())
		return errors.WithMessage(err, "failed to exec TaskTplGetAll")
	}

	m := make(map[int64]*models.TaskTpl, len(lst))
	for _, tpl := range lst {
		m[tpl.Id] = tpl
	}

	ttc.Set(m, stat.Total, stat.LastUpdated)

	ms := time.Since(start).Milliseconds()
	logger.Infof("timer: sync task tpls done, cost: %dms, number: %d", ms, len(m))
	dumper.PutSyncRecord("task_tpls", start.Unix(), ms, len(m), "success")

	return nil
}

func (ttc *TaskTplCache) loopSyncTaskTpl() {
	d := time.Duration(9) * time.Second
	for {
		time.Sleep(d)
		if err := ttc.syncTaskTpl(); err != nil {
			logger.Warning("failed to sync task tpl:", err)
		}
	}
}

func (ttc *TaskTplCache) StatChange(total int64, lastUpdated int64) bool {
	if ttc.statTotal == total && ttc.statLastUpdated == lastUpdated {
		return false
	}

	return true
}
