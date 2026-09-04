package dscache

import (
	"io"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/toolkits/pkg/logger"
)

// closeIfPossible 关闭实现了 io.Closer 的 datasource 实例。
// 通过类型断言实现，避免改动 datasource.Datasource 接口。
func closeIfPossible(cate string, dsId int64, ds datasource.Datasource, reason string) {
	if ds == nil {
		return
	}
	closer, ok := ds.(io.Closer)
	if !ok {
		return
	}
	if err := closer.Close(); err != nil {
		logger.Warningf("close plugin:%s %d (%s) failed: %v", cate, dsId, reason, err)
	}
}

type Cache struct {
	datas        map[string]map[int64]datasource.Datasource
	initAttempts map[string]map[int64]initAttempt
	mutex        *sync.RWMutex
}

type initAttempt struct {
	candidate  datasource.Datasource
	generation uint64
	inFlight   bool
	retryAfter time.Time
}

// 配置没变而远端持续不可达时，不应跟着 2 秒同步周期反复建连、打日志。
// 30 秒仍能很快感知恢复，同时把失败风暴降到原来的约 1/15。
var failedInitRetryInterval = 30 * time.Second

var DsCache = Cache{
	datas:        make(map[string]map[int64]datasource.Datasource),
	initAttempts: make(map[string]map[int64]initAttempt),
	mutex:        new(sync.RWMutex),
}

func (cs *Cache) Put(cate string, dsId int64, ds datasource.Datasource) {
	cs.mutex.Lock()
	if _, found := cs.datas[cate]; !found {
		cs.datas[cate] = make(map[int64]datasource.Datasource)
	}
	if cs.initAttempts == nil {
		cs.initAttempts = make(map[string]map[int64]initAttempt)
	}
	if _, found := cs.initAttempts[cate]; !found {
		cs.initAttempts[cate] = make(map[int64]initAttempt)
	}

	if _, found := cs.datas[cate][dsId]; found {
		if cs.datas[cate][dsId].Equal(ds) {
			cs.mutex.Unlock()
			return
		}
	}

	now := time.Now()
	previous, hasPrevious := cs.initAttempts[cate][dsId]
	if hasPrevious && previous.candidate.Equal(ds) {
		if previous.inFlight || now.Before(previous.retryAfter) {
			cs.mutex.Unlock()
			return
		}
	} else {
		previous.generation++
	}
	attempt := initAttempt{
		candidate:  ds,
		generation: previous.generation,
		inFlight:   true,
	}
	cs.initAttempts[cate][dsId] = attempt
	cs.mutex.Unlock()

	// InitClient() 在用户配置错误或远端不可用时, 会非常耗时, mutex被长期持有, 导致Get()会超时
	err := ds.InitClient()
	if err != nil {
		// Datasource errors may embed a DSN or URL credentials. Log only the type;
		// operators can identify the affected datasource by cate/id without leaking secrets.
		logger.Errorf("init plugin:%s %d client fail: error_type=%T", cate, dsId, err)
		// 防御性兜底: 当前 InitClient 实现在失败时通常不会留下半成品(各分支均在 return 前
		// 自行 Close 或直接未赋值), 此处 Close 多数情况下为 no-op. 保留是为了未来 InitClient
		// 出现部分初始化状态时不漏关. 注: gorm.Open 内部由 Dialector.Initialize 创建的
		// *sql.DB 当前架构下无法触达, 那条泄漏需要 InitCli 改为自管 *sql.DB 才能根治.
		closeIfPossible(cate, dsId, ds, "init failed")
		cs.mutex.Lock()
		current, found := cs.initAttempts[cate][dsId]
		if found && current.generation == attempt.generation {
			current.inFlight = false
			current.retryAfter = time.Now().Add(failedInitRetryInterval)
			cs.initAttempts[cate][dsId] = current
		}
		cs.mutex.Unlock()
		return
	}

	logger.Debugf("init plugin:%s %d client success", cate, dsId)
	cs.mutex.Lock()
	current, found := cs.initAttempts[cate][dsId]
	if !found || current.generation != attempt.generation {
		cs.mutex.Unlock()
		closeIfPossible(cate, dsId, ds, "superseded")
		return
	}
	old := cs.datas[cate][dsId]
	cs.datas[cate][dsId] = ds
	delete(cs.initAttempts[cate], dsId)
	cs.mutex.Unlock()
	// 替换旧实例时关闭旧值, 在锁外执行避免阻塞读路径.
	//
	if old != nil {
		closeIfPossible(cate, dsId, old, "replaced")
	}
}

func (cs *Cache) Get(cate string, dsId int64) (datasource.Datasource, bool) {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	if _, found := cs.datas[cate]; !found {
		return nil, false
	}

	if _, found := cs.datas[cate][dsId]; !found {
		return nil, false
	}

	return cs.datas[cate][dsId], true
}

func (cs *Cache) Delete(cate string, dsId int64) {
	cs.mutex.Lock()
	if attempts, found := cs.initAttempts[cate]; found {
		delete(attempts, dsId)
	}
	if _, found := cs.datas[cate]; !found {
		cs.mutex.Unlock()
		return
	}
	old := cs.datas[cate][dsId]
	delete(cs.datas[cate], dsId)
	cs.mutex.Unlock()

	if old != nil {
		closeIfPossible(cate, dsId, old, "deleted")
	}
	logger.Debugf("delete plugin:%s %d from cache", cate, dsId)
}

// GetAllIds 返回已成功缓存或仍在初始化/退避中的数据源 ID，按类型分组。
// 同步删除阶段需要看见失败过但尚未入 datas 的条目，避免配置删除后留下退避状态。
func (cs *Cache) GetAllIds() map[string][]int64 {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	result := make(map[string][]int64)
	for cate, dsMap := range cs.datas {
		ids := make([]int64, 0, len(dsMap)+len(cs.initAttempts[cate]))
		for dsId := range dsMap {
			ids = append(ids, dsId)
		}
		for dsId := range cs.initAttempts[cate] {
			if _, cached := dsMap[dsId]; !cached {
				ids = append(ids, dsId)
			}
		}
		result[cate] = ids
	}
	for cate, attempts := range cs.initAttempts {
		if _, found := result[cate]; found {
			continue
		}
		ids := make([]int64, 0, len(attempts))
		for dsId := range attempts {
			ids = append(ids, dsId)
		}
		result[cate] = ids
	}
	return result
}
