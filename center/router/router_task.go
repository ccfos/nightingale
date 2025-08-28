package router

import (
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/sender"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/strx"
	imodels "github.com/flashcatcloud/ibex/src/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/i18n"
)

func (rt *Router) taskGets(c *gin.Context) {
	bgid := ginx.UrlParamInt64(c, "id")
	mine := ginx.QueryBool(c, "mine", false)
	days := ginx.QueryInt64(c, "days", 7)
	limit := ginx.QueryInt(c, "limit", 20)
	query := ginx.QueryStr(c, "query", "")
	user := c.MustGet("user").(*models.User)

	creator := ""
	if mine {
		creator = user.Username
	}

	beginTime := time.Now().Unix() - days*24*3600

	total, err := models.TaskRecordTotal(rt.Ctx, []int64{bgid}, beginTime, creator, query)
	ginx.Dangerous(err)

	list, err := models.TaskRecordGets(rt.Ctx, []int64{bgid}, beginTime, creator, query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

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
	user := c.MustGet("user").(*models.User)

	creator := ""
	if mine {
		creator = user.Username
	}

	beginTime := time.Now().Unix() - days*24*3600

	total, err := models.TaskRecordTotal(rt.Ctx, gids, beginTime, creator, query)
	ginx.Dangerous(err)

	list, err := models.TaskRecordGets(rt.Ctx, gids, beginTime, creator, query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

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
	if !rt.Ibex.Enable {
		ginx.Bomb(400, i18n.Sprintf(c.GetHeader("X-Language"), "This functionality has not been enabled. Please contact the system administrator to activate it."))
		return
	}

	var f models.TaskForm
	ginx.BindJSON(c, &f)
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

	rt.checkTargetsExistByIndent(f.Hosts)

	err := f.Verify()
	ginx.Dangerous(err)

	f.HandleFH(f.Hosts[0])

	// check permission
	rt.checkTargetPerm(c, f.Hosts)

	// call ibex
	taskId, err := sender.TaskAdd(f, user.Username, rt.Ctx.IsCenter)
	ginx.Dangerous(err)

	if taskId <= 0 {
		ginx.Dangerous("created task.id is zero")
	}

	// write db
	record := models.TaskRecord{
		Id:        taskId,
		GroupId:   bgid,
		Title:     f.Title,
		Account:   f.Account,
		Batch:     f.Batch,
		Tolerance: f.Tolerance,
		Timeout:   f.Timeout,
		Pause:     f.Pause,
		Script:    f.Script,
		Args:      f.Args,
		CreateAt:  time.Now().Unix(),
		CreateBy:  f.Creator,
	}

	err = record.Add(rt.Ctx)
	ginx.NewRender(c).Data(taskId, err)
}

func (rt *Router) taskPut(c *gin.Context) {
	if !rt.Ibex.Enable {
		ginx.Bomb(400, i18n.Sprintf(c.GetHeader("X-Language"), "This functionality has not been enabled. Please contact the system administrator to activate it."))
		return
	}

	var task imodels.TaskHost
	ginx.BindJSON(c, &task)

	// 把 f.Hosts 中的空字符串过滤掉
	if task.Host == "" || task.Id <= 0 || task.Status == "" {
		ginx.Bomb(400, "invalid request body")
	}
	task.Host = strings.TrimSpace(task.Host)

	rt.checkTargetPerm(c, []string{task.Host})

	// update task host
	err := task.Upsert()
	if err != nil {
		ginx.Bomb(500, err.Error())
	}
	ginx.NewRender(c).Data(task.Id, nil)
}
