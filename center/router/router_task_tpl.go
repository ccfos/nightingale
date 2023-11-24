package router

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/str"
)

func (rt *Router) taskTplGets(c *gin.Context) {
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 20)
	groupId := ginx.UrlParamInt64(c, "id")

	total, err := models.TaskTplTotal(rt.Ctx, []int64{groupId}, query)
	ginx.Dangerous(err)

	list, err := models.TaskTplGets(rt.Ctx, []int64{groupId}, query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"total": total,
		"list":  list,
	}, nil)
}

func (rt *Router) taskTplGetsByGids(c *gin.Context) {
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 20)

	gids := str.IdsInt64(ginx.QueryStr(c, "gids"), ",")
	if len(gids) == 0 {
		ginx.NewRender(c, http.StatusBadRequest).Message("arg(gids) is empty")
		return
	}

	for _, gid := range gids {
		rt.bgroCheck(c, gid)
	}

	total, err := models.TaskTplTotal(rt.Ctx, gids, query)
	ginx.Dangerous(err)

	list, err := models.TaskTplGets(rt.Ctx, gids, query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"total": total,
		"list":  list,
	}, nil)
}

func (rt *Router) taskTplGet(c *gin.Context) {
	tid := ginx.UrlParamInt64(c, "tid")

	tpl, err := models.TaskTplGet(rt.Ctx, "id = ?", tid)
	ginx.Dangerous(err)

	if tpl == nil {
		ginx.Bomb(404, "no such task template")
	}

	hosts, err := tpl.Hosts(rt.Ctx)

	ginx.NewRender(c).Data(gin.H{
		"tpl":   tpl,
		"hosts": hosts,
	}, err)
}

func (rt *Router) taskTplGetByService(c *gin.Context) {
	tid := ginx.UrlParamInt64(c, "tid")

	tpl, err := models.TaskTplGetById(rt.Ctx, tid)
	ginx.Dangerous(err)

	if tpl == nil {
		ginx.Bomb(404, "no such task template")
	}

	ginx.NewRender(c).Data(tpl, err)
}

type taskTplForm struct {
	Title     string   `json:"title" binding:"required"`
	Batch     int      `json:"batch"`
	Tolerance int      `json:"tolerance"`
	Timeout   int      `json:"timeout"`
	Pause     string   `json:"pause"`
	Script    string   `json:"script"`
	Args      string   `json:"args"`
	Tags      []string `json:"tags"`
	Account   string   `json:"account"`
	Hosts     []string `json:"hosts"`
}

func (rt *Router) taskTplAdd(c *gin.Context) {
	var f taskTplForm
	ginx.BindJSON(c, &f)

	user := c.MustGet("user").(*models.User)
	now := time.Now().Unix()

	sort.Strings(f.Tags)

	tpl := &models.TaskTpl{
		GroupId:   ginx.UrlParamInt64(c, "id"),
		Title:     f.Title,
		Batch:     f.Batch,
		Tolerance: f.Tolerance,
		Timeout:   f.Timeout,
		Pause:     f.Pause,
		Script:    f.Script,
		Args:      f.Args,
		Tags:      strings.Join(f.Tags, " ") + " ",
		Account:   f.Account,
		CreateBy:  user.Username,
		UpdateBy:  user.Username,
		CreateAt:  now,
		UpdateAt:  now,
	}

	ginx.NewRender(c).Message(tpl.Save(rt.Ctx, f.Hosts))
}

func (rt *Router) taskTplPut(c *gin.Context) {
	tid := ginx.UrlParamInt64(c, "tid")

	tpl, err := models.TaskTplGet(rt.Ctx, "id = ?", tid)
	ginx.Dangerous(err)

	if tpl == nil {
		ginx.NewRender(c).Message("no such task template")
		return
	}

	user := c.MustGet("user").(*models.User)

	var f taskTplForm
	ginx.BindJSON(c, &f)

	sort.Strings(f.Tags)

	tpl.Title = f.Title
	tpl.Batch = f.Batch
	tpl.Tolerance = f.Tolerance
	tpl.Timeout = f.Timeout
	tpl.Pause = f.Pause
	tpl.Script = f.Script
	tpl.Args = f.Args
	tpl.Tags = strings.Join(f.Tags, " ") + " "
	tpl.Account = f.Account
	tpl.UpdateBy = user.Username
	tpl.UpdateAt = time.Now().Unix()

	ginx.NewRender(c).Message(tpl.Update(rt.Ctx, f.Hosts))
}

func (rt *Router) taskTplDel(c *gin.Context) {
	tid := ginx.UrlParamInt64(c, "tid")

	tpl, err := models.TaskTplGet(rt.Ctx, "id = ?", tid)
	ginx.Dangerous(err)

	if tpl == nil {
		ginx.NewRender(c).Message(nil)
		return
	}

	ginx.NewRender(c).Message(tpl.Del(rt.Ctx))
}

type tplTagsForm struct {
	Ids  []int64  `json:"ids" binding:"required"`
	Tags []string `json:"tags" binding:"required"`
}

func (f *tplTagsForm) Verify() {
	if len(f.Ids) == 0 {
		ginx.Bomb(http.StatusBadRequest, "arg(ids) empty")
	}

	if len(f.Tags) == 0 {
		ginx.Bomb(http.StatusBadRequest, "arg(tags) empty")
	}

	newTags := make([]string, 0, len(f.Tags))
	for i := 0; i < len(f.Tags); i++ {
		tag := strings.TrimSpace(f.Tags[i])
		if tag == "" {
			continue
		}

		if str.Dangerous(tag) {
			ginx.Bomb(http.StatusBadRequest, "arg(tags) invalid")
		}

		newTags = append(newTags, tag)
	}

	f.Tags = newTags
	if len(f.Tags) == 0 {
		ginx.Bomb(http.StatusBadRequest, "arg(tags) empty")
	}
}

func (rt *Router) taskTplBindTags(c *gin.Context) {
	var f tplTagsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	username := c.MustGet("username").(string)

	for i := 0; i < len(f.Ids); i++ {
		tpl, err := models.TaskTplGet(rt.Ctx, "id = ?", f.Ids[i])
		ginx.Dangerous(err)

		if tpl == nil {
			continue
		}

		ginx.Dangerous(tpl.AddTags(rt.Ctx, f.Tags, username))
	}

	ginx.NewRender(c).Message(nil)
}

func (rt *Router) taskTplUnbindTags(c *gin.Context) {
	var f tplTagsForm
	ginx.BindJSON(c, &f)
	f.Verify()

	username := c.MustGet("username").(string)

	for i := 0; i < len(f.Ids); i++ {
		tpl, err := models.TaskTplGet(rt.Ctx, "id = ?", f.Ids[i])
		ginx.Dangerous(err)

		if tpl == nil {
			continue
		}

		ginx.Dangerous(tpl.DelTags(rt.Ctx, f.Tags, username))
	}

	ginx.NewRender(c).Message(nil)
}
