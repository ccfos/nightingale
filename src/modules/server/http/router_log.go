package http

import (
	"github.com/didi/nightingale/v4/src/models"

	"github.com/gin-gonic/gin"
)

// 超管在后台查看所有登陆记录
func loginLogGets(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	username := queryStr(c, "username", "")
	btime := queryInt64(c, "btime")
	etime := queryInt64(c, "etime")

	total, err := models.LoginLogTotal(username, btime, etime)
	dangerous(err)

	list, err := models.LoginLogGets(username, btime, etime, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

// 超管在后台查看所有类型资源的操作记录
func operationLogGets(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")
	btime := queryInt64(c, "btime")
	etime := queryInt64(c, "etime")

	total, err := models.OperationLogTotal(query, btime, etime)
	dangerous(err)

	list, err := models.OperationLogQuery(query, btime, etime, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

// 查询具体某个资源的操作历史记录，一般用在资源详情页面
func operationLogResGets(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	btime := queryInt64(c, "btime")
	etime := queryInt64(c, "etime")
	rescl := queryStr(c, "rescl")
	resid := queryStr(c, "resid")

	total, err := models.OperationLogTotalByRes(rescl, resid, btime, etime)
	dangerous(err)

	list, err := models.OperationLogGetsByRes(rescl, resid, btime, etime, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func v1OperationLogResPost(c *gin.Context) {
	var f models.OperationLog
	bind(c, &f)
	renderMessage(c, f.New())
}
