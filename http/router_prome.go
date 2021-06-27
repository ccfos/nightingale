package http

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/prometheus/promql/parser"

	"github.com/didi/nightingale/v5/vos"
)

func checkPromeQl(c *gin.Context) {

	ql := c.Query("promql")
	_, err := parser.ParseExpr(ql)
	respD := &vos.PromQlCheckResp{}
	isCorrect := true
	if err != nil {

		isCorrect = false
		respD.ParseError = err.Error()
	}

	respD.QlCorrect = isCorrect
	renderData(c, respD, nil)
}
