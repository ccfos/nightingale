package timer

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/models"

	"github.com/toolkits/pkg/logger"
)

func SyncAlertRules() {
	if err := syncAlertRules(); err != nil {
		fmt.Println(err)
		exit(1)
	}

	go loopSyncAlertRules()
}

func loopSyncAlertRules() {
	randtime := rand.Intn(9000)
	fmt.Printf("timer: sync alert rules: random sleep %dms\n", randtime)
	time.Sleep(time.Duration(randtime) * time.Millisecond)

	interval := time.Duration(9) * time.Second

	for {
		time.Sleep(interval)
		if err := syncAlertRules(); err != nil {
			logger.Warning(err)
		}
	}
}

func syncAlertRules() error {
	start := time.Now()

	// 上次同步的时候同步了多少条rule，数据库中最近的更新时间，如果这俩信息都没变，说明DB中数据没变
	// 数据库中数据没变，那就不用再做操作了
	lastMaxUpdateTs := cache.AlertRulesByMetric.MaxUpdateTs
	ruleNum := cache.AlertRulesByMetric.RuleNum

	statistic, err := models.GetAlertRuleStatistic()
	if err != nil {
		return fmt.Errorf("sync alertRules getAlertRuleStatistics err: %v", err)
	}

	if statistic.Count == ruleNum && statistic.MaxUpdateAt == lastMaxUpdateTs {
		lastMaxUpdateStr := time.Unix(lastMaxUpdateTs, 0).Format("2006-01-02 15:04:05")
		logger.Debugf("[no_change_not_sync][LastUpdateAt:%+v][ruleNum:%+v]:", lastMaxUpdateStr, ruleNum)
		return nil
	}

	// 数据库中的记录和上次拉取的数据相比，发生变化，重新从数据库拉取最新数据
	logger.Debugf("[alert_rule_change_start_sync][last_num:%d this_num:%d][last_max_update_ts:%d this_max_update_ts:%d]:",
		ruleNum,
		statistic.Count,
		lastMaxUpdateTs,
		statistic.MaxUpdateAt)

	alertRules, err := models.AllAlertRules()
	alertRulesMap := make(map[int64]*models.AlertRule)

	if err != nil {
		return fmt.Errorf("sync alertRules [type=all] err: %v", err)
	}

	metricAlertRulesMap := make(map[string][]*models.AlertRule)
	for i := range alertRules {
		if err := alertRules[i].Decode(); err != nil {
			// 单个rule无法decode，直接忽略继续处理别的，等后面用户修复好了，数据库last_update信息变化，这里自然能感知
			logger.Warningf("syncAlertRule %v err:%v", alertRules[i], err)
			continue
		}

		alertRulesMap[alertRules[i].Id] = alertRules[i]
		if alertRules[i].Type == models.PUSH {
			metricAlertRulesMap[alertRules[i].FirstMetric] = append(metricAlertRulesMap[alertRules[i].FirstMetric], alertRules[i])
		}
	}

	cache.AlertRules.SetAll(alertRulesMap)
	cache.AlertRulesByMetric.SetAll(metricAlertRulesMap, statistic.MaxUpdateAt, statistic.Count, start.UnixNano())
	logger.Infof("[timer] sync alert rules done, found %d records, cost: %dms", statistic.Count, time.Since(start).Milliseconds())

	return nil
}
