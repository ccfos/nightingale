package router

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	promsdk "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/toolkits/pkg/logger"
)

type AlertStatusQuery struct {
	RuleId    int64  `json:"rule_id"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
	Labels    string `json:"labels,omitempty"` // 可选的标签过滤
}

func QueryAlertStatus(ctx *ctx.Context, promClients *prom.PromClientMap, query AlertStatusQuery) (interface{}, error) {
	DefaultPromDatasourceId := atomic.LoadInt64(&dscache.PromDefaultDatasourceId)
	if DefaultPromDatasourceId == 0 {
		return nil, fmt.Errorf("datasource id is 0")
	}

	readerClient := promClients.GetCli(DefaultPromDatasourceId)
	if readerClient == nil {
		return nil, fmt.Errorf("prometheus client not found for datasource id: %d", DefaultPromDatasourceId)
	}

	// 构建 PromQL 查询语句
	metricName := fmt.Sprintf("n9e_alert_rule_%d_status", query.RuleId)
	promql := metricName

	// 如果有标签过滤，添加到查询中
	if query.Labels != "" {
		promql = fmt.Sprintf("%s{%s}", metricName, query.Labels)
	}

	logger.Infof("Querying alert status: %s, time: %v - %v", promql, query.StartTime, query.EndTime)

	// 使用当前时间进行查询（获取最新状态）
	value, warnings, err := readerClient.QueryRange(context.Background(), promql, promsdk.Range{
		Start: time.Unix(query.StartTime, 0),
		End:   time.Unix(query.EndTime, 0),
		Step:  time.Minute,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to query prometheus: %v with query: %v", err, promql)
	}

	if len(warnings) > 0 {
		logger.Warningf("Query warnings: %v", warnings)
	}
	return value, nil
}
