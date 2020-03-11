package routes

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/model"
)

func endpointGets(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")
	batch := queryStr(c, "batch", "")
	field := queryStr(c, "field", "ident")

	if !(field == "ident" || field == "alias") {
		errors.Bomb("field invalid")
	}

	total, err := model.EndpointTotal(query, batch, field)
	errors.Dangerous(err)

	list, err := model.EndpointGets(query, batch, field, limit, offset(c, limit, total))
	errors.Dangerous(err)

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

type endpointImportForm struct {
	Endpoints []string `json:"endpoints"`
}

func endpointImport(c *gin.Context) {
	var f endpointImportForm
	errors.Dangerous(c.ShouldBind(&f))
	renderMessage(c, model.EndpointImport(f.Endpoints))
}

type endpointForm struct {
	Alias string `json:"alias"`
}

func endpointPut(c *gin.Context) {
	var f endpointForm
	errors.Dangerous(c.ShouldBind(&f))

	id := urlParamInt64(c, "id")
	endpoint, err := model.EndpointGet("id", id)
	errors.Dangerous(err)

	if endpoint == nil {
		errors.Bomb("no such endpoint, id: %d", id)
	}

	endpoint.Alias = f.Alias
	renderMessage(c, endpoint.Update("alias"))
}

type endpointDelForm struct {
	Idents []string `json:"idents"`
}

func endpointDel(c *gin.Context) {
	var f endpointDelForm
	errors.Dangerous(c.ShouldBind(&f))

	if f.Idents == nil || len(f.Idents) == 0 {
		renderMessage(c, nil)
		return
	}

	ids, err := model.EndpointIdsByIdents(f.Idents)
	errors.Dangerous(err)

	renderMessage(c, model.EndpointDel(ids))
}

func endpointBindingsGet(c *gin.Context) {
	idents := strings.Split(mustQueryStr(c, "idents"), ",")

	ids, err := model.EndpointIdsByIdents(idents)
	errors.Dangerous(err)

	if ids == nil || len(ids) == 0 {
		errors.Bomb("endpoints not found")
	}

	bindings, err := model.EndpointBindings(ids)
	renderData(c, bindings, err)
}

func endpointByNodeIdsGets(c *gin.Context) {
	ids := str.IdsInt64(mustQueryStr(c, "ids"))
	var allLeafIds []int64
	for i := 0; i < len(ids); i++ {
		node, err := model.NodeGet("id", ids[i])
		errors.Dangerous(err)

		if node == nil {
			errors.Bomb("no such node")
		}

		leafIds, err := node.LeafIds()
		errors.Dangerous(err)

		allLeafIds = append(allLeafIds, leafIds...)
	}

	list, err := model.EndpointUnderLeafs(allLeafIds)
	errors.Dangerous(err)

	renderData(c, list, nil)
}
