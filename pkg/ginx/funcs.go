package ginx

import (
	"github.com/gin-gonic/gin"
)

func Offset(c *gin.Context, limit int, pagenoVarName ...string) int {
	if limit <= 0 {
		limit = 10
	}

	pageno := "p"
	if len(pagenoVarName) > 0 {
		pageno = pagenoVarName[0]
	}

	page := QueryInt(c, pageno, 1)
	return (page - 1) * limit
}
