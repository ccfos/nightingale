package router

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/evallog"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
)

// evalRecordsGet 查询本引擎节点某规则的评估执行记录。
// GET /v1/n9e/eval-records?rule_id=&datasource_id=&from=&to=&before=&limit=
// from/to 为 unix 秒，before 为毫秒游标（返回 ts < before 的记录），结果按 ts 倒序。
func (rt *Router) evalRecordsGet(c *gin.Context) {
	ruleId := ginx.QueryInt64(c, "rule_id")
	datasourceId := ginx.QueryInt64(c, "datasource_id", 0)
	to := ginx.QueryInt64(c, "to", 0)
	// from 缺省取 to 之前 1 小时，与 center 侧默认窗口一致。
	// 不能再默认 0：那会让 reader 从当前时刻一路扫到 1970 年（约 49 万次小时迭代），
	// 单次请求就能占住 handler 数秒。reader 侧另有保留期下界兜底，这里是第一道。
	defaultTo := to
	if defaultTo <= 0 {
		defaultTo = time.Now().Unix()
	}
	from := ginx.QueryInt64(c, "from", defaultTo-3600)
	before := ginx.QueryInt64(c, "before", 0)
	limit := int(ginx.QueryInt64(c, "limit", int64(evallog.DefaultQueryLimit)))
	if limit <= 0 || limit > evallog.MaxQueryLimit {
		limit = evallog.DefaultQueryLimit
	}
	if from < 0 {
		from = 0
	}

	var toMs int64
	if to > 0 {
		toMs = to * 1000
	}

	res, err := evallog.QueryRecords(ruleId, datasourceId, from*1000, toMs, before, limit)
	if err == evallog.ErrNotEnabled {
		// 该节点未开启 evallog，返回空列表而非错误，center 聚合时不受影响
		ginx.NewRender(c).Data(gin.H{"list": []evallog.EvalRecord{}, "enabled": false}, nil)
		return
	}
	// ErrBusy 走错误通道而不是空列表：center 会把它原样透到前端的失败节点列表里，
	// 用户看到的是"忙，稍后重试"，而不是一个会被读成"当时没有评估"的空结果
	ginx.Dangerous(err)

	// truncated/note 必须跟着结果一起返回：前端判断"还有更多"用的是本页条数是否等于
	// limit，字节预算生效时条数会变少，不带这个标记就会被读成"这段时间就这么多记录"
	ginx.NewRender(c).Data(gin.H{
		"list":      res.Records,
		"enabled":   true,
		"truncated": res.Truncated,
		"note":      res.Note,
	}, nil)
}
