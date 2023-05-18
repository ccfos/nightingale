package router

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/str"
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

	total, err := models.TaskRecordTotal(rt.Ctx, bgid, beginTime, creator, query)
	ginx.Dangerous(err)

	list, err := models.TaskRecordGets(rt.Ctx, bgid, beginTime, creator, query, limit, ginx.Offset(c, limit))
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"total": total,
		"list":  list,
	}, nil)
}

type taskForm struct {
	Title     string   `json:"title" binding:"required"`
	Account   string   `json:"account" binding:"required"`
	Batch     int      `json:"batch"`
	Tolerance int      `json:"tolerance"`
	Timeout   int      `json:"timeout"`
	Pause     string   `json:"pause"`
	Script    string   `json:"script" binding:"required"`
	Args      string   `json:"args"`
	Action    string   `json:"action" binding:"required"`
	Creator   string   `json:"creator"`
	Hosts     []string `json:"hosts" binding:"required"`
}

func (f *taskForm) Verify() error {
	if f.Batch < 0 {
		return fmt.Errorf("arg(batch) should be nonnegative")
	}

	if f.Tolerance < 0 {
		return fmt.Errorf("arg(tolerance) should be nonnegative")
	}

	if f.Timeout < 0 {
		return fmt.Errorf("arg(timeout) should be nonnegative")
	}

	if f.Timeout > 3600*24 {
		return fmt.Errorf("arg(timeout) longer than one day")
	}

	if f.Timeout == 0 {
		f.Timeout = 30
	}

	f.Pause = strings.Replace(f.Pause, "，", ",", -1)
	f.Pause = strings.Replace(f.Pause, " ", "", -1)
	f.Args = strings.Replace(f.Args, "，", ",", -1)

	if f.Title == "" {
		return fmt.Errorf("arg(title) is required")
	}

	if str.Dangerous(f.Title) {
		return fmt.Errorf("arg(title) is dangerous")
	}

	if f.Script == "" {
		return fmt.Errorf("arg(script) is required")
	}

	if str.Dangerous(f.Args) {
		return fmt.Errorf("arg(args) is dangerous")
	}

	if str.Dangerous(f.Pause) {
		return fmt.Errorf("arg(pause) is dangerous")
	}

	if len(f.Hosts) == 0 {
		return fmt.Errorf("arg(hosts) empty")
	}

	if f.Action != "start" && f.Action != "pause" {
		return fmt.Errorf("arg(action) invalid")
	}

	return nil
}

func (f *taskForm) HandleFH(fh string) {
	i := strings.Index(f.Title, " FH: ")
	if i > 0 {
		f.Title = f.Title[:i]
	}
	f.Title = f.Title + " FH: " + fh
}

func (rt *Router) taskRecordAdd(c *gin.Context) {
	var f *models.TaskRecord
	ginx.BindJSON(c, &f)
	ginx.NewRender(c).Message(f.Add(rt.Ctx))
}

func (rt *Router) taskAdd(c *gin.Context) {
	var f taskForm
	ginx.BindJSON(c, &f)

	bgid := ginx.UrlParamInt64(c, "id")
	user := c.MustGet("user").(*models.User)
	f.Creator = user.Username

	err := f.Verify()
	ginx.Dangerous(err)

	f.HandleFH(f.Hosts[0])

	// check permission
	rt.checkTargetPerm(c, f.Hosts)

	// call ibex
	taskId, err := TaskCreate(f, rt.NotifyConfigCache.GetIbex())
	ginx.Dangerous(err)

	if taskId <= 0 {
		ginx.Dangerous("created task.id is zero")
	}

	// write db
	record := models.TaskRecord{
		Id:           taskId,
		GroupId:      bgid,
		IbexAddress:  rt.NotifyConfigCache.GetIbex().Address,
		IbexAuthUser: rt.NotifyConfigCache.GetIbex().BasicAuthUser,
		IbexAuthPass: rt.NotifyConfigCache.GetIbex().BasicAuthPass,
		Title:        f.Title,
		Account:      f.Account,
		Batch:        f.Batch,
		Tolerance:    f.Tolerance,
		Timeout:      f.Timeout,
		Pause:        f.Pause,
		Script:       f.Script,
		Args:         f.Args,
		CreateAt:     time.Now().Unix(),
		CreateBy:     f.Creator,
	}

	err = record.Add(rt.Ctx)
	ginx.NewRender(c).Data(taskId, err)
}

func (rt *Router) taskProxy(c *gin.Context) {
	target, err := url.Parse(rt.NotifyConfigCache.GetIbex().Address)
	if err != nil {
		ginx.NewRender(c).Message("invalid ibex address: %s", rt.NotifyConfigCache.GetIbex().Address)
		return
	}

	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host

		// fe request e.g. /api/n9e/busi-group/:id/task/*url
		index := strings.Index(req.URL.Path, "/task/")
		if index == -1 {
			panic("url path invalid")
		}

		req.URL.Path = "/ibex/v1" + req.URL.Path[index:]

		if target.RawQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = target.RawQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = target.RawQuery + "&" + req.URL.RawQuery
		}

		if rt.NotifyConfigCache.GetIbex().BasicAuthUser != "" {
			req.SetBasicAuth(rt.NotifyConfigCache.GetIbex().BasicAuthUser, rt.NotifyConfigCache.GetIbex().BasicAuthPass)
		}
	}

	errFunc := func(w http.ResponseWriter, r *http.Request, err error) {
		ginx.NewRender(c, http.StatusBadGateway).Message(err)
	}

	proxy := &httputil.ReverseProxy{
		Director:     director,
		ErrorHandler: errFunc,
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}
