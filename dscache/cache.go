package dscache

import (
	"io"
	"sync"

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
	datas map[string]map[int64]datasource.Datasource
	mutex *sync.RWMutex
}

var DsCache = Cache{
	datas: make(map[string]map[int64]datasource.Datasource),
	mutex: new(sync.RWMutex),
}

func (cs *Cache) Put(cate string, dsId int64, ds datasource.Datasource) {
	cs.mutex.Lock()
	if _, found := cs.datas[cate]; !found {
		cs.datas[cate] = make(map[int64]datasource.Datasource)
	}

	if _, found := cs.datas[cate][dsId]; found {
		if cs.datas[cate][dsId].Equal(ds) {
			cs.mutex.Unlock()
			return
		}
	}
	cs.mutex.Unlock()

	// InitClient() 在用户配置错误或远端不可用时, 会非常耗时, mutex被长期持有, 导致Get()会超时
	err := ds.InitClient()
	if err != nil {
		logger.Errorf("init plugin:%s %d %+v client fail: %v", cate, dsId, ds, err)
		// 防御性兜底: 当前 InitClient 实现在失败时通常不会留下半成品(各分支均在 return 前
		// 自行 Close 或直接未赋值), 此处 Close 多数情况下为 no-op. 保留是为了未来 InitClient
		// 出现部分初始化状态时不漏关. 注: gorm.Open 内部由 Dialector.Initialize 创建的
		// *sql.DB 当前架构下无法触达, 那条泄漏需要 InitCli 改为自管 *sql.DB 才能根治.
		closeIfPossible(cate, dsId, ds, "init failed")
		return
	}

	logger.Debugf("init plugin:%s %d %+v client success", cate, dsId, ds)
	cs.mutex.Lock()
	old := cs.datas[cate][dsId]
	cs.datas[cate][dsId] = ds
	cs.mutex.Unlock()
	// 替换旧实例时关闭旧值, 在锁外执行避免阻塞读路径.
	//
	// TODO(ABA): Put 当前是"读-检查-解锁-初始化-加锁-覆盖"模式, 中间窗口内可能有别的
	// goroutine 已成功写入更新的 ds. 本次 Put 仍会用本 goroutine 的 ds 无条件覆盖,
	// 把已生效的更新实例 Close 掉(可能正被 Get 出来的查询使用), 导致并发查询拿到
	// "sql: database is closed". *sql.DB.Close 会等待 in-flight 查询完成,
	// 缓解了一部分但不能根治. 根治需要给缓存条目加版本号, Put 写回前对比版本一致才覆盖,
	// 不一致就 Close 自己新建的 ds. 这是独立改造点, 不在本 PR 范围.
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

// GetAllIds 返回缓存中所有数据源的 ID，按类型分组
func (cs *Cache) GetAllIds() map[string][]int64 {
	cs.mutex.RLock()
	defer cs.mutex.RUnlock()
	result := make(map[string][]int64)
	for cate, dsMap := range cs.datas {
		ids := make([]int64, 0, len(dsMap))
		for dsId := range dsMap {
			ids = append(ids, dsId)
		}
		result[cate] = ids
	}
	return result
}
