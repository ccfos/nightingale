package router

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/ccfos/nightingale/v6/center/integration"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"
)

type Board struct {
	Name    string      `json:"name"`
	Tags    string      `json:"tags"`
	Configs interface{} `json:"configs"`
	UUID    int64       `json:"uuid"`
	Note    string      `json:"note"`
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
					if rule.UUID == 0 {
						rule.UUID = time.Now().UnixMicro()
					}

					contentBytes, err := json.Marshal(rule)
					if err != nil {
						reterr[rule.Name] = err.Error()
						continue
					}

					bp := models.BuiltinPayload{
						Type:        lst[i].Type,
						ComponentID: lst[i].ComponentID,
						Cate:        lst[i].Cate,
						Name:        rule.Name,
						Tags:        rule.AppendTags,
						UUID:        rule.UUID,
						Content:     string(contentBytes),
						CreatedBy:   username,
						UpdatedBy:   username,
					}

					if err := bp.Add(rt.Ctx, username); err != nil {
						reterr[bp.Name] = i18n.Sprintf(c.GetHeader("X-Language"), err.Error())
					}
				}
				continue
			}

			alertRule := models.AlertRule{}
			if err := json.Unmarshal([]byte(lst[i].Content), &alertRule); err != nil {
				reterr[lst[i].Name] = err.Error()
				continue
			}

			if alertRule.UUID == 0 {
				alertRule.UUID = time.Now().UnixMicro()
			}

			contentBytes, err := json.Marshal(alertRule)
			if err != nil {
				reterr[alertRule.Name] = err.Error()
				continue
			}

			bp := models.BuiltinPayload{
				Type:        lst[i].Type,
				ComponentID: lst[i].ComponentID,
				Cate:        lst[i].Cate,
				Name:        alertRule.Name,
				Tags:        alertRule.AppendTags,
				UUID:        alertRule.UUID,
				Content:     string(contentBytes),
				CreatedBy:   username,
				UpdatedBy:   username,
			}

			if err := bp.Add(rt.Ctx, username); err != nil {
				reterr[bp.Name] = i18n.Sprintf(c.GetHeader("X-Language"), err.Error())
			}
		} else if lst[i].Type == "dashboard" {
			if strings.HasPrefix(strings.TrimSpace(lst[i].Content), "[") {
				// 处理多个告警规则模板的情况
				dashboards := []Board{}
				if err := json.Unmarshal([]byte(lst[i].Content), &dashboards); err != nil {
					reterr[lst[i].Name] = err.Error()
				}

				for _, dashboard := range dashboards {
					if dashboard.UUID == 0 {
						dashboard.UUID = time.Now().UnixMicro()
					}

					contentBytes, err := json.Marshal(dashboard)
					if err != nil {
						reterr[dashboard.Name] = err.Error()
						continue
					}

					bp := models.BuiltinPayload{
						Type:        lst[i].Type,
						ComponentID: lst[i].ComponentID,
						Cate:        lst[i].Cate,
						Name:        dashboard.Name,
						Tags:        dashboard.Tags,
						UUID:        dashboard.UUID,
						Note:        dashboard.Note,
						Content:     string(contentBytes),
						CreatedBy:   username,
						UpdatedBy:   username,
					}

					if err := bp.Add(rt.Ctx, username); err != nil {
						reterr[bp.Name] = i18n.Sprintf(c.GetHeader("X-Language"), err.Error())
					}
				}
				continue
			}

			dashboard := Board{}
			if err := json.Unmarshal([]byte(lst[i].Content), &dashboard); err != nil {
				reterr[lst[i].Name] = i18n.Sprintf(c.GetHeader("X-Language"), err.Error())
				continue
			}

			if dashboard.UUID == 0 {
				dashboard.UUID = time.Now().UnixMicro()
			}

			contentBytes, err := json.Marshal(dashboard)
			if err != nil {
				reterr[dashboard.Name] = err.Error()
				continue
			}

			bp := models.BuiltinPayload{
				Type:        lst[i].Type,
				ComponentID: lst[i].ComponentID,
				Cate:        lst[i].Cate,
				Name:        dashboard.Name,
				Tags:        dashboard.Tags,
				UUID:        dashboard.UUID,
				Note:        dashboard.Note,
				Content:     string(contentBytes),
				CreatedBy:   username,
				UpdatedBy:   username,
			}

			if err := bp.Add(rt.Ctx, username); err != nil {
				reterr[bp.Name] = i18n.Sprintf(c.GetHeader("X-Language"), err.Error())
			}
		} else {
			if lst[i].Type == "collect" {
				c := make(map[string]interface{})
				if _, err := toml.Decode(lst[i].Content, &c); err != nil {
					reterr[lst[i].Name] = err.Error()
					continue
				}
			}

			if err := lst[i].Add(rt.Ctx, username); err != nil {
				reterr[lst[i].Name] = i18n.Sprintf(c.GetHeader("X-Language"), err.Error())
			}
		}

	}

	ginx.NewRender(c).Data(reterr, nil)
}

func (rt *Router) builtinPayloadsGets(c *gin.Context) {
	typ := ginx.QueryStr(c, "type", "")
	if typ == "" {
		ginx.Bomb(http.StatusBadRequest, "type is required")
		return
	}
	ComponentID := ginx.QueryInt64(c, "component_id", 0)

	cate := ginx.QueryStr(c, "cate", "")
	query := ginx.QueryStr(c, "query", "")

	lst, err := models.BuiltinPayloadGets(rt.Ctx, uint64(ComponentID), typ, cate, query)
	ginx.Dangerous(err)

	lstInFile, err := integration.BuiltinPayloadInFile.GetBuiltinPayload(typ, cate, query, uint64(ComponentID))
	ginx.Dangerous(err)

	if len(lstInFile) > 0 {
		lst = append(lst, lstInFile...)
	}

	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) builtinPayloadcatesGet(c *gin.Context) {
	typ := ginx.QueryStr(c, "type", "")
	ComponentID := ginx.QueryInt64(c, "component_id", 0)

	cates, err := models.BuiltinPayloadCates(rt.Ctx, typ, uint64(ComponentID))
	ginx.Dangerous(err)

	catesInFile, err := integration.BuiltinPayloadInFile.GetBuiltinPayloadCates(typ, uint64(ComponentID))
	ginx.Dangerous(err)

	// 使用 map 进行去重
	cateMap := make(map[string]bool)

	// 添加数据库中的分类
	for _, cate := range cates {
		cateMap[cate] = true
	}

	// 添加文件中的分类
	for _, cate := range catesInFile {
		cateMap[cate] = true
	}

	// 将去重后的结果转换回切片
	result := make([]string, 0, len(cateMap))
	for cate := range cateMap {
		result = append(result, cate)
	}

	ginx.NewRender(c).Data(result, nil)
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

	if req.Type == "alert" {
		alertRule := models.AlertRule{}
		if err := json.Unmarshal([]byte(req.Content), &alertRule); err != nil {
			ginx.Bomb(http.StatusBadRequest, err.Error())
		}

		req.Name = alertRule.Name
		req.Tags = alertRule.AppendTags
	} else if req.Type == "dashboard" {
		dashboard := Board{}
		if err := json.Unmarshal([]byte(req.Content), &dashboard); err != nil {
			ginx.Bomb(http.StatusBadRequest, err.Error())
		}

		req.Name = dashboard.Name
		req.Tags = dashboard.Tags
		req.Note = dashboard.Note
	} else if req.Type == "collect" {
		c := make(map[string]interface{})
		if _, err := toml.Decode(req.Content, &c); err != nil {
			ginx.Bomb(http.StatusBadRequest, err.Error())
		}
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

func (rt *Router) builtinPayloadsGetByUUID(c *gin.Context) {
	uuid := ginx.QueryInt64(c, "uuid")

	bp, err := models.BuiltinPayloadGet(rt.Ctx, "uuid = ?", uuid)
	ginx.Dangerous(err)

	if bp != nil {
		ginx.NewRender(c).Data(bp, nil)
	} else {
		ginx.NewRender(c).Data(integration.BuiltinPayloadInFile.IndexData[uuid], nil)
	}
}
