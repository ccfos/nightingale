package router

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/strx"

	"github.com/gin-gonic/gin"
)

// parseAuthLevels 解析逗号分隔的 auth_level 字符串，支持不传(返回空)和传多个
func parseAuthLevels(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	parts := strings.Split(s, ",")
	levels := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if v, err := strconv.Atoi(p); err == nil {
			levels = append(levels, v)
		}
	}
	return levels
}

func (rt *Router) taskGets(c *gin.Context) {
	bgid := ginx.UrlParamInt64(c, "id")
	mine := ginx.QueryBool(c, "mine", false)
	days := ginx.QueryInt64(c, "days", 7)
	limit := ginx.QueryInt(c, "limit", 20)
	query := ginx.QueryStr(c, "query", "")
	authLevels := parseAuthLevels(ginx.QueryStr(c, "auth_level", ""))
	user := c.MustGet("user").(*models.User)

	creator := ""
	if mine {
		creator = user.Username
	}

	beginTime := time.Now().Unix() - days*24*3600

	total, err := models.TaskRecordTotal(rt.Ctx, []int64{bgid}, beginTime, creator, query, authLevels)
	ginx.Dangerous(err)

	list, err := models.TaskRecordGets(rt.Ctx, []int64{bgid}, beginTime, creator, query, authLevels, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	models.FillCreateByNicknames(rt.Ctx, list)

	ginx.NewRender(c).Data(gin.H{
		"total": total,
		"list":  list,
	}, nil)
}

func (rt *Router) taskGetsByGids(c *gin.Context) {
	gids := strx.IdsInt64ForAPI(ginx.QueryStr(c, "gids", ""), ",")
	if len(gids) > 0 {
		for _, gid := range gids {
			rt.bgroCheck(c, gid)
		}
	} else {
		me := c.MustGet("user").(*models.User)
		if !me.IsAdmin() {
			var err error
			gids, err = models.MyBusiGroupIds(rt.Ctx, me.Id)
			ginx.Dangerous(err)

			if len(gids) == 0 {
				ginx.NewRender(c).Data([]int{}, nil)
				return
			}
		}
	}

	mine := ginx.QueryBool(c, "mine", false)
	days := ginx.QueryInt64(c, "days", 7)
	limit := ginx.QueryInt(c, "limit", 20)
	query := ginx.QueryStr(c, "query", "")
	authLevels := parseAuthLevels(ginx.QueryStr(c, "auth_level", ""))
	user := c.MustGet("user").(*models.User)

	creator := ""
	if mine {
		creator = user.Username
	}

	beginTime := time.Now().Unix() - days*24*3600

	total, err := models.TaskRecordTotal(rt.Ctx, gids, beginTime, creator, query, authLevels)
	ginx.Dangerous(err)

	list, err := models.TaskRecordGets(rt.Ctx, gids, beginTime, creator, query, authLevels, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	models.FillCreateByNicknames(rt.Ctx, list)

	ginx.NewRender(c).Data(gin.H{
		"total": total,
		"list":  list,
	}, nil)
}

func (rt *Router) taskRecordAdd(c *gin.Context) {
	var f *models.TaskRecord
	ginx.BindJSON(c, &f)
	ginx.NewRender(c).Message(f.Add(rt.Ctx))
}

func (rt *Router) taskAdd(c *gin.Context) {
	var f models.TaskForm
	ginx.BindJSON(c, &f)

	taskId, err := TaskAdd(rt.Ctx, c, rt.Ibex.Enable, f)
	ginx.NewRender(c).Data(taskId, err)
}

func TaskAdd(ctx *ctx.Context, c *gin.Context, ibexEnable bool, f models.TaskForm) (int64, error) {
	if !ibexEnable {
		return 0, errors.New("This functionality has not been enabled. Please contact the system administrator to activate it.")
	}

	// 把 f.Hosts 中的空字符串过滤掉
	hosts := make([]string, 0, len(f.Hosts))
	for i := range f.Hosts {
		if strings.TrimSpace(f.Hosts[i]) != "" {
			hosts = append(hosts, strings.TrimSpace(f.Hosts[i]))
		}
	}
	f.Hosts = hosts

	bgid := ginx.UrlParamInt64(c, "id")
	user := c.MustGet("user").(*models.User)
	f.Creator = user.Username

	err := CheckTargetsExistByIndent(ctx, f.Hosts)
	if err != nil {
		return 0, err
	}

	err = f.Verify()
	if err != nil {
		return 0, err
	}

	f.HandleFH(f.Hosts[0])

	// check permission
	CheckTargetPerm(ctx, c, f.Hosts)

	// call ibex
	taskId, err := sender.TaskAdd(f, user.Username, ctx.IsCenter)
	ginx.Dangerous(err)

	if taskId <= 0 {
		ginx.Dangerous("created task.id is zero")
	}

	// write db
	record := models.TaskRecord{
		Id:           taskId,
		GroupId:      bgid,
		Title:        f.Title,
		Account:      f.Account,
		Batch:        f.Batch,
		Tolerance:    f.Tolerance,
		Timeout:      f.Timeout,
		Pause:        f.Pause,
		Script:       f.Script,
		Args:         f.Args,
		SystemCaller: f.SystemCaller,
		AuthLevel:    f.AuthLevel,
		CreateAt:     time.Now().Unix(),
		CreateBy:     f.Creator,
	}

	err = record.Add(ctx)
	return taskId, err
}
