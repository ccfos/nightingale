package memsto

import (
	"encoding/json"
	"log"
	"regexp"
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

	targetCache *TargetCacheType
	ruleCache   *AlertRuleCacheType

	targetsByGroup map[int64][]*models.Target
	targetsByIdent map[string][]*models.Target
	targetsByTag   map[string][]*models.Target
	allTargets     map[string]*models.Target
}

func NewTargetOfAlertRuleCache(ctx *ctx.Context, engineName string, stats *Stats, targetCache *TargetCacheType, ruleCache *AlertRuleCacheType) *TargetsOfAlertRuleCacheType {
	tc := &TargetsOfAlertRuleCacheType{
		statTotal:       -1,
		statLastUpdated: -1,
		ctx:             ctx,
		engineName:      engineName,
		stats:           stats,
		targets:         make(map[string]map[int64][]string),
		targetCache:     targetCache,
		ruleCache:       ruleCache,
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
	rules := tc.ruleCache.GetAll()
	hostrules := make(map[int64]*models.AlertRule)
	for k, v := range rules {
		if v.Prod == "host" {
			hostrules[k] = v
		}
	}

	targetsByGroup := tc.targetsByGroup
	targetsByIdent := tc.targetsByIdent
	targetsByTag := tc.targetsByTag

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

		// 用于存放 tags 过滤的结果，先将所有 target 放到其中
		resmap := make(map[int64]*models.Target)
		for _, target := range tc.allTargets {
			resmap[target.Id] = target
		}

		// 遍历 rule 的 queries，根据不同的 key 进行过滤
		// inMap 为符合条件的 target，notInMap 为不符合条件的 target
		// inMap 和 notInMap 可能都为 nil，表示不需要过滤
		for _, q := range rule.Queries {
			var inMap map[int64]struct{}
			var notInMap map[int64]struct{}

			switch q.Key {
			case "group_ids":
				inMap, notInMap = filterGroupMap(targetsByGroup, q)
			case "tags":
				inMap, notInMap = filterTagMap(targetsByTag, q)
			case "hosts":
				inMap, notInMap = filterHostMap(targetsByIdent, q)
			}

			handleTargetFilterMap(resmap, inMap, notInMap)
		}

		// 将过滤后的结果放到 m 中
		for _, target := range resmap {
			if _, exists := m[target.EngineName]; !exists {
				m[target.EngineName] = make(map[int64][]string)
			}

			if _, exists := m[target.EngineName][hr.Id]; !exists {
				m[target.EngineName][hr.Id] = make([]string, 0)
			}

			m[target.EngineName][hr.Id] = append(m[target.EngineName][hr.Id], target.Ident)
		}
	}

	tc.Set(m, 0, 0)
	return nil
}

// 更新 target 相关的 map，根据不同的 key，包括 targetsByGroup, targetsByIdent, targetsByTag
func (tc *TargetsOfAlertRuleCacheType) updateTargetMaps() {
	allTargets := tc.targetCache.GetAll()

	targetsByGroup := make(map[int64][]*models.Target)
	targetsByIdent := make(map[string][]*models.Target)
	targetsByTag := make(map[string][]*models.Target)

	for _, target := range allTargets {
		if _, exists := targetsByGroup[target.GroupId]; !exists {
			targetsByGroup[target.GroupId] = make([]*models.Target, 0)
		}
		targetsByGroup[target.GroupId] = append(targetsByGroup[target.GroupId], target)

		if _, exists := targetsByIdent[target.Ident]; !exists {
			targetsByIdent[target.Ident] = make([]*models.Target, 0)
		}
		targetsByIdent[target.Ident] = append(targetsByIdent[target.Ident], target)

		// 将 hosttags 和 tags 都放到 targetsByTag 中
		for _, tag := range target.HostTags {
			if _, exists := targetsByTag[tag]; !exists {
				targetsByTag[tag] = make([]*models.Target, 0)
			}
			targetsByTag[tag] = append(targetsByTag[tag], target)
		}

		tags := strings.Split(target.Tags, " ")
		for _, tag := range tags {
			if tag == "" {
				continue
			}

			if _, exists := targetsByTag[tag]; !exists {
				targetsByTag[tag] = make([]*models.Target, 0)
			}

			targetsByTag[tag] = append(targetsByTag[tag], target)
		}

	}

	tc.targetsByGroup = targetsByGroup
	tc.targetsByIdent = targetsByIdent
	tc.targetsByTag = targetsByTag
	tc.allTargets = allTargets
}

// 根据 query 过滤 group id map 中 符合条件和不符合条件的 target，分别存放在 inMap 和 notInMap 中
// 当 q.Op == "==" 时，返回的 inMap 中包含所有符合条件的 target
// 当 q.Op == "!=" 时，返回的 notInMap 中包含所有不符合条件的 target
func filterGroupMap(targetMap map[int64][]*models.Target, q models.HostQuery) (inMap map[int64]struct{}, notInMap map[int64]struct{}) {
	if q.Op == "==" {
		inMap = make(map[int64]struct{})
		// 遍历 q.Values，将符合条件的 target 都放到新 map 中
		for _, v := range q.Values {
			key := v.(int64)
			if targets, exists := targetMap[key]; exists {
				// 筛选出符合条件的 target
				for _, target := range targets {
					inMap[target.Id] = struct{}{}
				}
			}
		}

		return inMap, nil
	} else {
		notInMap = make(map[int64]struct{})
		// 筛选出不符合条件的 target
		for _, v := range q.Values {
			key := v.(int64)
			if targets, exists := targetMap[key]; exists {
				for _, target := range targets {
					notInMap[target.Id] = struct{}{}
				}
			}
		}

		return nil, notInMap
	}
}

// 针对 tags 过滤，返回两个 map，一个是符合条件的 target，一个是不符合条件的 target
// 因为同一个 target 可能存在多个 tag，所以不能简单的将 tag 的 key 移除，而是需要知道具体的 target 是否需要移除
// 当 q.Op == "==" 时，返回的 inMap 中包含所有符合条件的 target
// 当 q.Op == "!=" 时，返回的 notInMap 中包含所有不符合条件的 target，这时 inMap 为 nil
// 上级可根据 inMap 是否为 nil 来判断是 == 还是 !=
func filterTagMap(targetMap map[string][]*models.Target, q models.HostQuery) (inMap map[int64]struct{}, notInMap map[int64]struct{}) {
	if q.Op == "==" {
		inMap = make(map[int64]struct{})
		notInMap = make(map[int64]struct{})
		for _, v := range q.Values {
			key := v.(string)
			if targets, exists := targetMap[key]; exists {
				// 筛选出符合条件的 target
				for _, target := range targets {
					inMap[target.Id] = struct{}{}
				}
			}
		}
	} else {
		// 直接从 targetMap 中删除对应的 key
		inMap = nil
		notInMap = make(map[int64]struct{})
		for _, v := range q.Values {
			key := v.(string)
			if targets, exists := targetMap[key]; exists {
				// 筛选出不符合条件的 target
				for _, target := range targets {
					notInMap[target.Id] = struct{}{}
				}
			}
		}
	}

	return inMap, notInMap
}

// // 根据 query 过滤 host map 中 符合条件和不符合条件的 target，分别存放在 inMap 和 notInMap 中
// 当 q.Op == "==" 时，返回的 inMap 中包含所有符合条件的 target
// 当 q.Op == "!=" 时，返回的 notInMap 中包含所有不符合条件的 target
// 当 q.Op == "=~" 时，模糊过滤，返回的 inMap 中包含所有符合条件的 target
// 当 q.Op == "!~" 时，模糊过滤，返回的 notInMap 中包含所有不符合条件的 target
// 在 ~ 的情况下，value 可能为通配符 * 或 %，支持模糊匹配
func filterHostMap(targetMap map[string][]*models.Target, q models.HostQuery) (inMap map[int64]struct{}, notInMap map[int64]struct{}) {
	if q.Op == "==" {
		inMap = make(map[int64]struct{})
		// 遍历 q.Values，将符合条件的 target 都放到 inMap 中
		for _, v := range q.Values {
			key := v.(string)
			if targets, exists := targetMap[key]; exists {
				for _, target := range targets {
					inMap[target.Id] = struct{}{}
				}
			}
		}

		return inMap, nil
	} else if q.Op == "!=" {
		notInMap = make(map[int64]struct{})
		// 遍历 q.Values，将不符合条件的 target 都放到 notInMap 中
		for _, v := range q.Values {
			key := v.(string)
			if targets, exists := targetMap[key]; exists {
				for _, target := range targets {
					notInMap[target.Id] = struct{}{}
				}
			}
		}

		return nil, notInMap
	} else if q.Op == "=~" {
		inMap = make(map[int64]struct{})
		for _, v := range q.Values {
			pattern := v.(string)
			regex := likePatternToRegex(pattern)
			re, err := regexp.Compile(regex)
			if err != nil {
				logger.Errorf("failed to compile regex:%s error:%v", regex, err)
				continue
			}

			for key := range targetMap {
				if re.MatchString(key) {
					for _, target := range targetMap[key] {
						inMap[target.Id] = struct{}{}
					}
				}
			}
		}

		return inMap, nil
	} else if q.Op == "!~" {
		notInMap = make(map[int64]struct{})
		for _, v := range q.Values {
			pattern := v.(string)
			regex := likePatternToRegex(pattern)
			re, err := regexp.Compile(regex)
			if err != nil {
				logger.Errorf("failed to compile regex:%s error:%v", regex, err)
				continue
			}

			for key := range targetMap {
				if re.MatchString(key) {
					for _, target := range targetMap[key] {
						notInMap[target.Id] = struct{}{}
					}
				}
			}
		}

		return nil, notInMap
	}

	return nil, nil
}

// 将 like 模式转换为正则表达式
// % * 匹配任意个字符
func likePatternToRegex(pattern string) string {
	var sb strings.Builder
	// 添加正则表达式起始标记
	sb.WriteString("^")
	for _, ch := range pattern {
		switch ch {
		case '%':
		case '*':
			// % 匹配任意个字符
			sb.WriteString(".*")
		default:
			// 对于其他特殊正则字符，需要转义
			if strings.ContainsRune(`.+?()|[]{}^$\\`, ch) {
				sb.WriteString("\\")
			}
			sb.WriteRune(ch)
		}
	}
	// 添加正则表达式结束标记
	sb.WriteString("$")
	return sb.String()
}

// 根据 inMap 和 notInMap 过滤 targetMap，将不符合条件的 target 删除
// inMap 中包含的 target 都是符合条件的 target
// notInMap 中包含的 target 都是不符合条件的 target
// resmap 为需要过滤的 map, 过滤后的结果会直接修改 resmap
func handleTargetFilterMap(targetMap map[int64]*models.Target, inMap map[int64]struct{}, notInMap map[int64]struct{}) {
	if inMap != nil {
		for key := range targetMap {
			if _, exists := inMap[key]; !exists {
				delete(targetMap, key)
			}
		}
	}

	if notInMap != nil {
		for key := range targetMap {
			if _, exists := notInMap[key]; exists {
				delete(targetMap, key)
			}
		}
	}
}
