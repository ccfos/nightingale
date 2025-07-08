package router

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/ccfos/nightingale/v6/dscache"
	promsdk "github.com/ccfos/nightingale/v6/pkg/prom"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

type AlertStatusQuery struct {
	RuleId    int64  `json:"rule_id"`
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
	Labels    string `json:"labels,omitempty"` // 可选的标签过滤
}

func (rt *Router) QueryAlertStatus(c *gin.Context) {
	var query AlertStatusQuery
	ginx.BindJSON(c, &query)

	DefaultPromDatasourceId := atomic.LoadInt64(&dscache.PromDefaultDatasourceId)
	if DefaultPromDatasourceId == 0 {
		ginx.NewRender(c).Data(map[string]interface{}{
			"value":     nil,
			"timestamp": nil,
		}, nil)
		return
	}

	readerClient := rt.PromClients.GetCli(DefaultPromDatasourceId)
	if readerClient == nil {
		ginx.NewRender(c).Data(map[string]interface{}{
			"value":     nil,
			"timestamp": nil,
		}, nil)
		return
	}

	// 构建 PromQL 查询语句
	valueMetricName := fmt.Sprintf("n9e_alert_rule_%d_status_value", query.RuleId)
	timestampMetricName := fmt.Sprintf("n9e_alert_rule_%d_status_timestamp", query.RuleId)
	valuePromql := valueMetricName
	timestampPromql := timestampMetricName

	// 如果有标签过滤，添加到查询中
	if query.Labels != "" {
		valuePromql = fmt.Sprintf("%s{%s}", valueMetricName, query.Labels)
		timestampPromql = fmt.Sprintf("%s{%s}", timestampMetricName, query.Labels)
	}

	logger.Infof("Querying alert status: %s & %s, time: %v - %v", valuePromql, timestampPromql, query.StartTime, query.EndTime)

	// 使用当前时间进行查询（获取最新状态）
	value, warnings, err := readerClient.QueryRange(context.Background(), valuePromql, promsdk.Range{
		Start: time.Unix(query.StartTime, 0),
		End:   time.Unix(query.EndTime, 0),
		Step:  time.Minute,
	})

	if err != nil {
		ginx.NewRender(c).Data(map[string]interface{}{
			"value":     nil,
			"timestamp": nil,
		}, nil)
		return
	}

	if len(warnings) > 0 {
		logger.Warningf("Query warnings: %v with query: %s", warnings, valuePromql)
	}

	timestamp, warnings, err := readerClient.QueryRange(context.Background(), timestampPromql, promsdk.Range{
		Start: time.Unix(query.StartTime, 0),
		End:   time.Unix(query.EndTime, 0),
		Step:  time.Minute,
	})

	if err != nil {
		ginx.NewRender(c).Data(map[string]interface{}{
			"value":     nil,
			"timestamp": nil,
		}, nil)
		return
	}

	if len(warnings) > 0 {
		logger.Warningf("Query warnings: %v with query: %s & %s", warnings, valuePromql, timestampPromql)
	}

	ginx.NewRender(c).Data(map[string]interface{}{
		"value":     value,
		"timestamp": timestamp,
	}, nil)
}
