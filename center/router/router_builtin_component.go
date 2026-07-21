package router

import (
	"net/http"

	"github.com/ccfos/nightingale/v6/center/integration"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
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

	// 非源语言时用 README 语言副本覆盖返回值（DB 中存的是源语言 README）。
	// 用户改过的组件（UpdatedBy != system）原样返回：用户内容语言无关，
	// 与消息模板"自建不过滤"同一原则；副本缺失时回退源语言
	lang := integration.NormalizeLang(c.GetHeader("X-Language"))
	if lang != integration.LangSource && integration.BuiltinPayloadInFile != nil {
		if readmes := integration.BuiltinPayloadInFile.Readmes[lang]; readmes != nil {
			for i := range bc {
				if bc[i].UpdatedBy != SYSTEM {
					continue
				}
				if readme, ok := readmes[bc[i].Ident]; ok {
					bc[i].Readme = readme
				}
			}
		}
	}

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
		// 内置组件只允许启停：readme/logo 以 integrations 文件为源，不接受编辑
		// 回写——英文界面下 GET 返回的 readme 是语言副本，随表单整行落库会把
		// 源语言 README 覆盖掉，且 UpdatedBy 一旦不是 system，语言副本渲染
		// 与重启时的文件恢复会一并失效
		ginx.NewRender(c).Message(bc.UpdateDisabled(rt.Ctx, req.Disabled))
		return
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
