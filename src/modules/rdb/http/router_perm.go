package http

import (
	"strings"

	"github.com/didi/nightingale/src/models"
	"github.com/gin-gonic/gin"
)

func v1CandoGlobalOp(c *gin.Context) {
	username := queryStr(c, "username")
	operation := queryStr(c, "op")
	has, err := models.UsernameCandoGlobalOp(username, operation)
	renderData(c, has, err)
}

func v1CandoNodeOp(c *gin.Context) {
	username := queryStr(c, "username")
	operation := queryStr(c, "op")
	nodeId := queryInt64(c, "nid")
	has, err := models.UsernameCandoNodeOp(username, operation, nodeId)
	renderData(c, has, err)
}

func v1CandoNodeOps(c *gin.Context) {
	username := queryStr(c, "username")
	ops := strings.Split(queryStr(c, "ops"), ",")
	nodeId := queryInt64(c, "nid")
	node := Node(nodeId)

	user, err := models.UserGet("username=?", username)
	dangerous(err)

	ret := make(map[string]bool)

	for i := 0; i < len(ops); i++ {
		has, err := user.HasPermByNode(node, ops[i])
		dangerous(err)

		ret[ops[i]] = has
	}

	renderData(c, ret, nil)
}
