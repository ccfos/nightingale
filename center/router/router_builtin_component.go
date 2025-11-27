package router

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/ccfos/nightingale/v6/center/integration"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"gorm.io/gorm"
)

const SYSTEM = "system"

func (rt *Router) builtinComponentsAdd(c *gin.Context) {
	var lst []models.BuiltinComponent
	ginx.BindJSON(c, &lst)

	username := Username(c)

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	reterr := make(map[string]string)
	for i := 0; i < count; i++ {
		if err := lst[i].Add(rt.Ctx, username); err != nil {
			reterr[lst[i].Ident] = err.Error()
		}
	}

	ginx.NewRender(c).Data(reterr, nil)
}

func (rt *Router) builtinComponentsGets(c *gin.Context) {
	query := ginx.QueryStr(c, "query", "")
	disabled := ginx.QueryInt(c, "disabled", -1)

	bc, err := models.BuiltinComponentGets(rt.Ctx, query, disabled)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(bc, nil)
}

func (rt *Router) builtinComponentsPut(c *gin.Context) {
	var req models.BuiltinComponent
	ginx.BindJSON(c, &req)

	bc, err := models.BuiltinComponentGet(rt.Ctx, "id = ?", req.ID)
	ginx.Dangerous(err)

	if bc == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such builtin component")
		return
	}

	if bc.CreatedBy == SYSTEM {
		req.Ident = bc.Ident
	}

	username := Username(c)
	req.UpdatedBy = username

	err = models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{
			DB: tx,
		}

		txErr := models.BuiltinMetricBatchUpdateColumn(tCtx, "typ", bc.Ident, req.Ident, req.UpdatedBy)
		if txErr != nil {
			return txErr
		}

		txErr = bc.Update(tCtx, req)
		if txErr != nil {
			return txErr
		}
		return nil
	})

	ginx.NewRender(c).Message(err)
}

func (rt *Router) builtinComponentsDel(c *gin.Context) {
	var req idsForm
	ginx.BindJSON(c, &req)

	req.Verify()

	ginx.NewRender(c).Message(models.BuiltinComponentDels(rt.Ctx, req.Ids))
}

func (rt *Router) builtinComponentsAndPayloadsGets(c *gin.Context) {
	query := ginx.QueryStr(c, "query", "")
	lowerQuery := strings.ToLower(query)

	dbPayloads, err := models.BuiltinPayloadGetsAll(rt.Ctx)
	ginx.Dangerous(err)

	filePayloads, err := integration.BuiltinPayloadInFile.GetAllBuiltinPayloads("")
	ginx.Dangerous(err)

	allPayloads := make([]*models.BuiltinPayload, 0, len(dbPayloads)+len(filePayloads))
	allPayloads = append(allPayloads, dbPayloads...)
	allPayloads = append(allPayloads, filePayloads...)

	compToCatesMap := make(map[string]map[string]struct{})

	for i := range allPayloads {
		payload := allPayloads[i]
		if payload == nil {
			continue
		}
		compName := payload.Component
		if compName == "" {
			comp, err := models.BuiltinComponentGet(rt.Ctx, "id = ?", payload.ComponentID)
			ginx.Dangerous(err)
			if comp != nil {
				compName = comp.Ident
			} else {
				compName = fmt.Sprintf("unknown_component_id_%d", payload.ComponentID)
			}
		}

		// 查询
		if lowerQuery != "" {
			if !strings.Contains(strings.ToLower(compName), lowerQuery) &&
				!strings.Contains(strings.ToLower(payload.Cate), lowerQuery) {
				continue
			}
		}

		if payload.Cate == "" {
			continue
		}

		if _, ok := compToCatesMap[compName]; !ok {
			compToCatesMap[compName] = make(map[string]struct{})
		}
		compToCatesMap[compName][payload.Cate] = struct{}{}
	}

	ret := make([]map[string]interface{}, 0, len(compToCatesMap))
	for compName, cateSet := range compToCatesMap {
		cates := make([]string, 0, len(cateSet))
		for cate := range cateSet {
			cates = append(cates, cate)
		}
		sort.Strings(cates) // cate 排序

		item := make(map[string]interface{})
		item["component_name"] = compName
		item["cates"] = cates
		ret = append(ret, item)
	}

	// 对结果按 component_name 排序，保证分页/截断的稳定性
	sort.Slice(ret, func(i, j int) bool {
		return ret[i]["component_name"].(string) < ret[j]["component_name"].(string)
	})

	// 如果 query 为空，只返回前 20 个
	if query == "" && len(ret) > 20 {
		ret = ret[:20]
	}

	ginx.NewRender(c).Data(ret, nil)
}
