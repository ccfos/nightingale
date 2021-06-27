package judge

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/config"
	"github.com/didi/nightingale/v5/models"
	"github.com/didi/nightingale/v5/naming"
	"github.com/toolkits/pkg/container/list"
	"github.com/toolkits/pkg/logger"
)

var (
	// 这个内存Queue放到judge的包里或alert的包里感觉都可以
	// 放到judge的包里，即当前的做法，相当于把alert看做judge的一个附属小功能
	// 这个Queue的核心作用就是削峰填谷，应对突然产生的大面积事件
	EventQueue *list.SafeListLimited

	// 上次同步全量告警规则的时间，全量同步都没做过，我这也不用处理PULL的规则了
	lastSyncTime int64
)

func Start(ctx context.Context) {
	// PUSH型的告警引擎，依赖内存里缓存的数据来做告警判断，两层map减小锁粒度
	initPointCaches()

	// 默认初始化的大小是1000万，相当于内存里有1000万事件，应该够用了
	EventQueue = list.NewSafeListLimited(10000000)

	// 开始心跳，对于PUSH型的数据我有策略了自然就可以处理了
	if err := heartbeat(config.Config.Heartbeat.LocalAddr); err != nil {
		fmt.Println(err)
		logger.Close()
		os.Exit(1)
	}

	go loopHeartbeat()

	// PULL型的策略不着急，等一段时间(等哈希环是稳态的)再开始周期性干活
	go syncPullRules(ctx)
}

func syncPullRules(ctx context.Context) {
	// 先等一会再干活，等大部分judge都上报心跳过了，哈希环不变了
	time.Sleep(time.Second * 33)
	for {
		syncPullRulesOnce(ctx)
		time.Sleep(time.Second * 9)
	}
}

func syncPullRulesOnce(ctx context.Context) {
	if cache.AlertRulesByMetric.LastSync == lastSyncTime {
		return
	}

	// 根据我自己的标识，去查找属于我的PULL型告警规则
	ident := config.Config.Heartbeat.LocalAddr

	rules := cache.AlertRules.Pulls()
	count := len(rules)
	mines := make([]models.AlertRule, 0, count)
	logger.Debugf("[got_one_pull_rule_for_all][ruleNum:%v]", count)
	for i := 0; i < count; i++ {

		instance, err := naming.HashRing.GetNode(fmt.Sprint(rules[i].Id))
		if err != nil {
			logger.Warningf("hashring: sharding pull rule(%d) fail: %v", rules[i].Id, err)
			continue
		}
		logger.Debugf("[got_one_pull_rule_hash_result][instance:%v][ident:%v][rule:%v]", instance, ident, rules[i])
		if instance == ident {
			// 属于我的
			mines = append(mines, *rules[i])
			logger.Debugf("[got_one_pull_rule_for_me][rule:%v]", rules[i])
		}
	}

	pullRuleManager.SyncRules(ctx, mines)
	lastSyncTime = cache.AlertRulesByMetric.LastSync
}

func loopHeartbeat() {
	interval := time.Duration(config.Config.Heartbeat.Interval) * time.Millisecond

	for {
		time.Sleep(interval)
		if err := heartbeat(config.Config.Heartbeat.LocalAddr); err != nil {
			logger.Warning(err)
		}
	}
}

func heartbeat(endpoint string) error {
	err := models.InstanceHeartbeat(config.EndpointName, endpoint)
	if err != nil {
		return fmt.Errorf("mysql.error: instance(service=%s, endpoint=%s) heartbeat fail: %v", config.EndpointName, endpoint, err)
	}
	return nil
}
