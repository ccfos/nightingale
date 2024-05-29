package router

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
)

type Board struct {
	Name    string      `json:"name"`
	Tags    string      `json:"tags"`
	Configs interface{} `json:"configs"`
}

func (rt *Router) builtinPayloadsAdd(c *gin.Context) {
	var lst []models.BuiltinPayload
	ginx.BindJSON(c, &lst)

	username := Username(c)

	count := len(lst)
	if count == 0 {
		ginx.Bomb(http.StatusBadRequest, "input json is empty")
	}

	reterr := make(map[string]string)
	for i := 0; i < count; i++ {
		if lst[i].Type == "alert" {
			if strings.HasPrefix(strings.TrimSpace(lst[i].Content), "[") {
				// 处理多个告警规则模板的情况
				alertRules := []models.AlertRule{}
				if err := json.Unmarshal([]byte(lst[i].Content), &alertRules); err != nil {
					reterr[lst[i].Name] = err.Error()
				}

				for _, rule := range alertRules {
					contentBytes, err := json.Marshal(rule)
					if err != nil {
						reterr[rule.Name] = err.Error()
						continue
					}

					bp := models.BuiltinPayload{
						Type:      lst[i].Type,
						Component: lst[i].Component,
						Cate:      lst[i].Cate,
						Name:      rule.Name,
						Tags:      rule.AppendTags,
						Content:   string(contentBytes),
						CreatedBy: username,
						UpdatedBy: username,
					}

					if err := bp.Add(rt.Ctx, username); err != nil {
						reterr[bp.Name] = err.Error()
					}
				}
				continue
			}

			alertRule := models.AlertRule{}
			if err := json.Unmarshal([]byte(lst[i].Content), &alertRule); err != nil {
				reterr[lst[i].Name] = err.Error()
				continue
			}

			bp := models.BuiltinPayload{
				Type:      lst[i].Type,
				Component: lst[i].Component,
				Cate:      lst[i].Cate,
				Name:      alertRule.Name,
				Tags:      alertRule.AppendTags,
				Content:   lst[i].Content,
				CreatedBy: username,
				UpdatedBy: username,
			}

			if err := bp.Add(rt.Ctx, username); err != nil {
				reterr[bp.Name] = err.Error()
			}
		} else if lst[i].Type == "dashboard" {
			if strings.HasPrefix(strings.TrimSpace(lst[i].Content), "[") {
				// 处理多个告警规则模板的情况
				dashboards := []Board{}
				if err := json.Unmarshal([]byte(lst[i].Content), &dashboards); err != nil {
					reterr[lst[i].Name] = err.Error()
				}

				for _, dashboard := range dashboards {
					contentBytes, err := json.Marshal(dashboard)
					if err != nil {
						reterr[dashboard.Name] = err.Error()
						continue
					}

					bp := models.BuiltinPayload{
						Type:      lst[i].Type,
						Component: lst[i].Component,
						Cate:      lst[i].Cate,
						Name:      dashboard.Name,
						Tags:      dashboard.Tags,
						Content:   string(contentBytes),
						CreatedBy: username,
						UpdatedBy: username,
					}

					if err := bp.Add(rt.Ctx, username); err != nil {
						reterr[bp.Name] = err.Error()
					}
				}
				continue
			}

			dashboard := Board{}
			if err := json.Unmarshal([]byte(lst[i].Content), &dashboard); err != nil {
				reterr[lst[i].Name] = err.Error()
				continue
			}

			bp := models.BuiltinPayload{
				Type:      lst[i].Type,
				Component: lst[i].Component,
				Cate:      lst[i].Cate,
				Name:      dashboard.Name,
				Tags:      dashboard.Tags,
				Content:   lst[i].Content,
				CreatedBy: username,
				UpdatedBy: username,
			}

			if err := bp.Add(rt.Ctx, username); err != nil {
				reterr[bp.Name] = err.Error()
			}
		} else {
			if err := lst[i].Add(rt.Ctx, username); err != nil {
				reterr[lst[i].Name] = err.Error()
			}
		}

	}

	ginx.NewRender(c).Data(reterr, nil)
}

func (rt *Router) builtinPayloadsGets(c *gin.Context) {
	typ := ginx.QueryStr(c, "type", "")
	component := ginx.QueryStr(c, "component", "")
	cate := ginx.QueryStr(c, "cate", "")
	query := ginx.QueryStr(c, "query", "")

	lst, err := models.BuiltinPayloadGets(rt.Ctx, typ, component, cate, query)
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) builtinPayloadcatesGet(c *gin.Context) {
	typ := ginx.QueryStr(c, "type", "")
	component := ginx.QueryStr(c, "component", "")

	cates, err := models.BuiltinPayloadCates(rt.Ctx, typ, component)
	ginx.NewRender(c).Data(cates, err)
}

func (rt *Router) builtinPayloadGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")

	bp, err := models.BuiltinPayloadGet(rt.Ctx, "id = ?", id)
	if err != nil {
		ginx.Bomb(http.StatusInternalServerError, err.Error())
	}
	if bp == nil {
		ginx.Bomb(http.StatusNotFound, "builtin payload not found")
	}

	ginx.NewRender(c).Data(bp, nil)
}

func (rt *Router) builtinPayloadsPut(c *gin.Context) {
	var req models.BuiltinPayload
	ginx.BindJSON(c, &req)

	bp, err := models.BuiltinPayloadGet(rt.Ctx, "id = ?", req.ID)
	ginx.Dangerous(err)

	if bp == nil {
		ginx.NewRender(c, http.StatusNotFound).Message("No such builtin payload")
		return
	}

	if bp.Type == "alert" {
		alertRule := models.AlertRule{}
		if err := json.Unmarshal([]byte(req.Content), &alertRule); err != nil {
			ginx.Bomb(http.StatusBadRequest, err.Error())
		}

		bp.Name = alertRule.Name
		bp.Tags = alertRule.AppendTags
	} else if bp.Type == "dashboard" {
		dashboard := Board{}
		if err := json.Unmarshal([]byte(req.Content), &dashboard); err != nil {
			ginx.Bomb(http.StatusBadRequest, err.Error())
		}

		bp.Name = dashboard.Name
		bp.Tags = dashboard.Tags
	}

	username := Username(c)
	req.UpdatedBy = username

	ginx.NewRender(c).Message(bp.Update(rt.Ctx, req))
}

func (rt *Router) builtinPayloadsDel(c *gin.Context) {
	var req idsForm
	ginx.BindJSON(c, &req)

	req.Verify()

	ginx.NewRender(c).Message(models.BuiltinPayloadDels(rt.Ctx, req.Ids))
}
