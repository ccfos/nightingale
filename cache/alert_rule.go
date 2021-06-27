package cache

import (
	"sync"

	"github.com/didi/nightingale/v5/models"
)

type AlertRulesByMetricCache struct {
	sync.RWMutex
	Data        map[string][]*models.AlertRule // key是metric，便于后续检索
	MaxUpdateTs int64                          // 从数据库拿到的最大update_at
	RuleNum     int64                          // 从数据库中统计到的行数
	LastSync    int64                          // 保存上次全量同步时间
}

var (
	AlertRulesByMetric = &AlertRulesByMetricCache{Data: make(map[string][]*models.AlertRule)}
)

func (a *AlertRulesByMetricCache) GetBy(instance string) []*models.AlertRule {
	a.RLock()
	defer a.RUnlock()

	return a.Data[instance]
}

func (a *AlertRulesByMetricCache) SetAll(alertRulesMap map[string][]*models.AlertRule, lastUpdateTs, ruleNum, lastSync int64) {
	a.Lock()
	defer a.Unlock()

	a.Data = alertRulesMap
	a.MaxUpdateTs = lastUpdateTs
	a.RuleNum = ruleNum
	a.LastSync = lastSync
}

type AlertRulesTotalCache struct {
	sync.RWMutex
	Data map[int64]*models.AlertRule
}

var AlertRules = &AlertRulesTotalCache{Data: make(map[int64]*models.AlertRule)}

func (a *AlertRulesTotalCache) Get(id int64) (*models.AlertRule, bool) {
	a.RLock()
	defer a.RUnlock()

	alertRule, exists := a.Data[id]
	return alertRule, exists
}

func (a *AlertRulesTotalCache) SetAll(alertRulesMap map[int64]*models.AlertRule) {
	a.Lock()
	defer a.Unlock()

	a.Data = alertRulesMap
}

// 获取所有PULL型规则的列表
func (a *AlertRulesTotalCache) Pulls() []*models.AlertRule {
	a.RLock()
	defer a.RUnlock()

	cnt := len(a.Data)
	ret := make([]*models.AlertRule, 0, cnt)

	for _, rule := range a.Data {
		if rule.Type == models.PULL {
			ret = append(ret, rule)
		}
	}

	return ret
}
