package http

import (
	"github.com/didi/nightingale/src/models"
	"github.com/gin-gonic/gin"
)

func nodeTrashGets(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")

	total, err := models.NodeTrashTotal(query)
	dangerous(err)

	list, err := models.NodeTrashGets(query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func nodeTrashRecycle(c *gin.Context) {
	var f idsForm
	bind(c, &f)
	renderMessage(c, models.NodeTrashRecycle(f.Ids))
}
