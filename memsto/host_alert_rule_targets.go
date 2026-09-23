package memsto

import (
	"log"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/toolkits/pkg/logger"
)

type TargetsOfAlertRuleCacheType struct {
	statTotal       int64
	statLastUpdated int64
	ctx             *ctx.Context
	stats           *Stats
	engineName      string

	// lastSig 记录上一轮同步时 host 规则 -> target 映射的输入签名，用于变更检测。
	// 仅由单一的同步 goroutine 读写，无需加锁。
	lastSig string

	sync.RWMutex
	targets map[string]map[int64][]string // key: ident
}

func NewTargetOfAlertRuleCache(ctx *ctx.Context, engineName string, stats *Stats) *TargetsOfAlertRuleCacheType {
	tc := &TargetsOfAlertRuleCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		engineName:      engineName,
		stats:           stats,
		targets:         make(map[string]map[int64][]string),
	}

	tc.SyncTargets()
	return tc
}

func (tc *TargetsOfAlertRuleCacheType) Reset() {
	tc.Lock()
	defer tc.Unlock()

	tc.statTotal = -1
	tc.statLastUpdated = -1
	tc.lastSig = ""
	tc.targets = make(map[string]map[int64][]string)
}

func (tc *TargetsOfAlertRuleCacheType) Set(m map[string]map[int64][]string, total, lastUpdated int64) {
	tc.Lock()
	tc.targets = m
	tc.Unlock()

	// only one goroutine used, so no need lock
	tc.statTotal = total
	tc.statLastUpdated = lastUpdated
}

func (tc *TargetsOfAlertRuleCacheType) Get(engineName string, rid int64) ([]string, bool) {
	tc.RLock()
	defer tc.RUnlock()
	m, has := tc.targets[engineName]
	if !has {
		return nil, false
	}

	lst, has := m[rid]
	return lst, has
}

// GetByEngine 返回某个引擎名下「规则 id -> 机器 ident 列表」的快照，供 center 的
// /v1/n9e/targets-of-alert-rule 直接从内存应答 edge / alert 的回源请求，
// 而不是每次请求都对全部 host 规则各扫一遍 target 表。
// 返回的是拷贝：外层 map 与内层切片都不与缓存共享，调用方可以随意持有或改写。
func (tc *TargetsOfAlertRuleCacheType) GetByEngine(engineName string) (map[int64][]string, bool) {
	tc.RLock()
	defer tc.RUnlock()

	m, has := tc.targets[engineName]
	if !has {
		return nil, false
	}

	ret := make(map[int64][]string, len(m))
	for rid, idents := range m {
		ret[rid] = append([]string(nil), idents...)
	}
	return ret, true
}

func (tc *TargetsOfAlertRuleCacheType) SyncTargets() {
	err := tc.syncTargets()
	if err != nil {
		log.Fatalln("failed to sync targets:", err)
	}

	go tc.loopSyncTargets()
}

func (tc *TargetsOfAlertRuleCacheType) loopSyncTargets() {
	// 30s：规则与机器的绑定关系无需 9s 级新鲜度，放宽周期并配合变更门降低 DB 压力
	duration := time.Duration(30000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := tc.syncTargets(); err != nil {
			logger.Warning("failed to sync host alert rule targets:", err)
		}
	}
}

func (tc *TargetsOfAlertRuleCacheType) syncTargets() error {
	// 变更门：host 规则 -> target 映射的输入（规则/归组/机器）未变化时跳过整轮重算，
	// 避免每个周期对所有 host 规则各发一条全表扫描的过滤 SQL。仅 center 直连 DB 时启用；
	// edge 走 HTTP 回源、无本地 DB，保持每轮同步。
	var sig string
	var sigOK bool
	if tc.ctx.IsCenter {
		s, err := models.HostAlertRuleTargetsSig(tc.ctx)
		if err != nil {
			logger.Warning("failed to compute host alert rule targets signature, sync anyway:", err)
		} else {
			sig, sigOK = s, true
			if sig == tc.lastSig {
				return nil
			}
		}
	}

	m, err := models.GetTargetsOfHostAlertRule(tc.ctx, tc.engineName)
	if err != nil {
		return err
	}
	logger.Debugf("get_targets_of_alert_rule total: %d engine_name:%s", len(m), tc.engineName)
	for k, v := range m {
		logger.Debugf("get_targets_of_alert_rule key:%s value:%v", k, v)
	}

	tc.Set(m, 0, 0)

	// 仅在成功取到签名且同步成功后更新；取签名失败时 lastSig 不变，下轮继续尝试，避免漏同步
	if sigOK {
		tc.lastSig = sig
	}
	return nil
}
