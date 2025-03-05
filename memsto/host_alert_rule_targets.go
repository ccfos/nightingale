package memsto

import (
	"encoding/json"
	"log"
	"strings"
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

	sync.RWMutex
	targets map[string]map[int64][]string // key: ident

	targetcache *TargetCacheType
	rulecache   *AlertRuleCacheType

	targetGroupIdMap map[int64][]*models.Target
	targetIndentMap  map[string][]*models.Target
	targetHostTagMap map[string][]*models.Target
	targetTagMap     map[string][]*models.Target
}

func NewTargetOfAlertRuleCache(ctx *ctx.Context, engineName string, stats *Stats, targetcache *TargetCacheType, rulecache *AlertRuleCacheType) *TargetsOfAlertRuleCacheType {
	tc := &TargetsOfAlertRuleCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		engineName:      engineName,
		stats:           stats,
		targets:         make(map[string]map[int64][]string),
		targetcache:     targetcache,
		rulecache:       rulecache,
	}

	tc.SyncTargets()
	return tc
}

func (tc *TargetsOfAlertRuleCacheType) Reset() {
	tc.Lock()
	defer tc.Unlock()

	tc.statTotal = -1
	tc.statLastUpdated = -1
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

func (tc *TargetsOfAlertRuleCacheType) SyncTargets() {
	err := tc.syncTargets()
	if err != nil {
		log.Fatalln("failed to sync targets:", err)
	}

	go tc.loopSyncTargets()
}

func (tc *TargetsOfAlertRuleCacheType) loopSyncTargets() {
	duration := time.Duration(9000) * time.Millisecond
	for {
		time.Sleep(duration)
		if err := tc.syncTargets(); err != nil {
			logger.Warning("failed to sync host alert rule targets:", err)
		}
	}
}

func (tc *TargetsOfAlertRuleCacheType) syncTargets() error {
	// 从缓存获取所有 targetmap
	tc.updateTargetMaps()
	m := make(map[string]map[int64][]string)

	// 从缓存获取所有 host alert rule
	rules := tc.rulecache.GetAll()
	hostrules := make(map[int64]*models.AlertRule)
	for k, v := range rules {
		if v.Prod == "host" {
			hostrules[k] = v
		}
	}

	for _, hr := range hostrules {
		var rule *models.HostRuleConfig
		if err := json.Unmarshal([]byte(hr.RuleConfig), &rule); err != nil {
			logger.Errorf("rule:%d rule_config:%s, error:%v", hr.Id, hr.RuleConfig, err)
			continue
		}

		if rule == nil {
			logger.Errorf("rule:%d rule_config:%s, error:rule is nil", hr.Id, hr.RuleConfig)
			continue
		}

		targetGroupIdMap := tc.targetGroupIdMap
		targetIndentMap := tc.targetIndentMap
		targetHostTagMap := tc.targetHostTagMap
		targetTagMap := tc.targetTagMap

		var targetHostTagMapResult map[int64]struct{}
		targetTagMapResult := make(map[int64]struct{})

		notintargets := make(map[int64]struct{}) // 用于筛选 != 的情况
		for _, q := range rule.Queries {
			switch q.Key {
			case "group_ids":
				targetGroupIdMap = filterMap(targetGroupIdMap, q, int64convert)
			case "tags":
				tinmap, tnotinmap := filterHostMap(targetTagMap, q, stringconvert)
				
				if tinmap != nil {
					if targetHostTagMapResult == nil {
						targetHostTagMapResult = tinmap
					} else {
						for k, _ := range targetHostTagMapResult {
							if _, exists := tinmap[k]; !exists {
								delete(targetHostTagMapResult, k)
							}
						}
					}
				}

				for k := range tnotinmap {
					notintargets[k] = struct{}{}
				}


				htinmap, htnotinmap := filterHostMap(targetHostTagMap, q, stringconvert)

				if htinmap != nil {
					if targetTagMapResult == nil {
						targetTagMapResult = htinmap
					} else {
						for k := range targetTagMapResult {
							if _, exists := htinmap[k]; !exists {
								delete(targetTagMapResult, k)
							}
						}
					}
				}

				for k := range htnotinmap {
					notintargets[k] = struct{}{}
				}

			case "hosts":
				targetIndentMap = filterMap(targetIndentMap, q, stringconvert)
			}
		}

		// group_ids，indent 都需要匹配
		for _, ts := range targetGroupIdMap {
			for _, target := range ts {
				// 检测是否在 targetIndentMap 中
				if _, exists := targetIndentMap[target.Ident]; !exists {
					continue
				}

				// 检测是否有 tags 过滤，执行了一次就不会是 nil
				if targetHostTagMapResult != nil {
					// 检测是否在 notintargets 中
					if _, exists := notintargets[target.Id]; exists {
						continue
					}

					// 检测是否在 targetHostTagMapResult 或 targetTagMapResult 中
					if _, exists := targetHostTagMapResult[target.Id]; !exists {
						if _, exists := targetTagMapResult[target.Id]; !exists {
							continue
						}
					}
				}

				if _, exists := m[target.EngineName]; !exists {
					m[target.EngineName] = make(map[int64][]string)
				}

				if _, exists := m[target.EngineName][hr.Id]; !exists {
					m[target.EngineName][hr.Id] = make([]string, 0)
				}

				m[target.EngineName][hr.Id] = append(m[target.EngineName][hr.Id], target.Ident)
			}
		}
	}

	tc.Set(m, 0, 0)
	return nil
}

// 更新 target 相关的 map，根据不同的 key，包括 targetGroupIdMap, targetIndentMap, targetHostTagMap, targetTagMap
func (tc *TargetsOfAlertRuleCacheType) updateTargetMaps() {
	alltargets := tc.targetcache.GetAll()

	targetGroupIdMap := make(map[int64][]*models.Target)
	targetIndentMap := make(map[string][]*models.Target)
	targetHostTagMap := make(map[string][]*models.Target)
	targetTagMap := make(map[string][]*models.Target)

	for _, target := range alltargets {
		if _, exists := targetGroupIdMap[target.GroupId]; !exists {
			targetGroupIdMap[target.GroupId] = make([]*models.Target, 0)
		}
		targetGroupIdMap[target.GroupId] = append(targetGroupIdMap[target.GroupId], target)

		if _, exists := targetIndentMap[target.Ident]; !exists {
			targetIndentMap[target.Ident] = make([]*models.Target, 0)
		}
		targetIndentMap[target.Ident] = append(targetIndentMap[target.Ident], target)

		for _, tag := range target.HostTags {
			if _, exists := targetHostTagMap[tag]; !exists {
				targetHostTagMap[tag] = make([]*models.Target, 0)
			}
			targetHostTagMap[tag] = append(targetHostTagMap[tag], target)
		}

		tags := strings.Split(target.Tags, " ")
		for _, tag := range tags {
			if tag == "" {
				continue
			}

			if _, exists := targetTagMap[tag]; !exists {
				targetTagMap[tag] = make([]*models.Target, 0)
			}

			targetTagMap[tag] = append(targetTagMap[tag], target)
		}

	}

	tc.targetGroupIdMap = targetGroupIdMap
	tc.targetIndentMap = targetIndentMap
	tc.targetHostTagMap = targetHostTagMap
	tc.targetTagMap = targetTagMap
}

// 根据 query 过滤 map 中的 indent，返回新的 map，针对一个 target 对应一个 key 的情况
func filterMap[T comparable](targetMap map[T][]*models.Target, q models.HostQuery, convert func(interface{}) (T, bool)) map[T][]*models.Target {
	if q.Op == "==" {
		newMap := make(map[T][]*models.Target)
		// 遍历 q.Values，将符合条件的 target 都放到新 map 中
		for _, v := range q.Values {
			key, ok := convert(v)
			if !ok {
				continue
			}
			if targets, exists := targetMap[key]; exists {
				newMap[key] = targets
			}
		}

		return newMap
	} else {
		// 直接从 targetMap 中删除对应的 key
		for _, v := range q.Values {
			key, ok := convert(v)
			if !ok {
				continue
			}
			delete(targetMap, key)
		}

		return targetMap
	}
}

// 针对 tags 过滤，返回两个 map，一个是符合条件的 target，一个是不符合条件的 target
// 因为同一个 target 可能存在多个 tag，所以不能简单的将 tag 的 key 移除，而是需要知道具体的 target 是否需要移除
// 当 q.Op == "==" 时，返回的 inmap 中包含所有符合条件的 target
// 当 q.Op == "!=" 时，返回的 notinmap 中包含所有不符合条件的 target，这时 inmap 为 nil
// 上级可根据 inmap 是否为 nil 来判断是是 == 还是 !=
func filterHostMap[T comparable](targetMap map[T][]*models.Target, q models.HostQuery, convert func(interface{}) (T, bool)) (inmap map[int64]struct{}, notinmap map[int64]struct{}) {
	if q.Op == "==" {
		inmap = make(map[int64]struct{})
		notinmap = make(map[int64]struct{})
		for _, v := range q.Values {
			key, ok := convert(v)
			if !ok {
				continue
			}
			if targets, exists := targetMap[key]; exists {
				// 筛选出符合条件的 target
				for _, target := range targets {		
					inmap[target.Id] = struct{}{}
				}
			}
		}
	} else {
		// 直接从 targetMap 中删除对应的 key
		inmap = nil
		notinmap = make(map[int64]struct{})
		for _, v := range q.Values {
			key, ok := convert(v)
			if !ok {
				continue
			}

			if targets, exists := targetMap[key]; exists {
				// 筛选出不符合条件的 target
				for _, target := range targets {
					notinmap[target.Id] = struct{}{}
				}
			}
		}
	}

	return inmap, notinmap
}

func int64convert(v interface{}) (int64, bool) {
	if id, ok := v.(int64); ok {
		return id, true
	}
	return 0, false
}

func stringconvert(v interface{}) (string, bool) {
	if tag, ok := v.(string); ok {
		return tag, true
	}
	return "", false
}