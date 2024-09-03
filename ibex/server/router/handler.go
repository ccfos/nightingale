package router

import (
	"fmt"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"strconv"

	"io/ioutil"
	"net/http"
	"time"

	"github.com/ccfos/nightingale/v6/ibex/server/config"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errorx"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/str"
)

func (rou *Router) taskStdout(c *gin.Context) {
	meta := TaskMeta(rou.ctx, UrlParamsInt64(c, "id"))
	stdouts, err := meta.Stdouts(rou.ctx)
	ginx.NewRender(c).Data(stdouts, err)
}

func (rou *Router) taskStderr(c *gin.Context) {
	meta := TaskMeta(rou.ctx, UrlParamsInt64(c, "id"))
	stderrs, err := meta.Stderrs(rou.ctx)
	ginx.NewRender(c).Data(stderrs, err)
}

// TODO: 不能只判断task_action，还应该看所有的host执行情况
func (rou *Router) taskState(c *gin.Context) {
	action, err := models.TaskActionGet(rou.ctx, "id=?", UrlParamsInt64(c, "id"))
	if err != nil {
		ginx.NewRender(c).Data("", err)
		return
	}

	state := "done"
	if action != nil {
		state = action.Action
	}

	ginx.NewRender(c).Data(state, err)
}

func (rou *Router) taskResult(c *gin.Context) {
	id := UrlParamsInt64(c, "id")

	hosts, err := models.TaskHostStatus(rou.ctx, id)
	if err != nil {
		errorx.Bomb(500, "load task hosts of %d occur error %v", id, err)
	}

	ss := make(map[string][]string)
	total := len(hosts)
	for i := 0; i < total; i++ {
		s := hosts[i].Status
		ss[s] = append(ss[s], hosts[i].Host)
	}

	ginx.NewRender(c).Data(ss, nil)
}

func (rou *Router) taskHostOutput(c *gin.Context) {
	obj, err := models.TaskHostGet(rou.ctx, UrlParamsInt64(c, "id"), ginx.UrlParamStr(c, "host"))
	ginx.NewRender(c).Data(obj, err)
}

func (rou *Router) taskHostStdout(c *gin.Context) {
	id := UrlParamsInt64(c, "id")
	host := ginx.UrlParamStr(c, "host")

	if config.C.Output.ComeFrom == "database" || config.C.Output.ComeFrom == "" {
		obj, err := models.TaskHostGet(rou.ctx, id, host)
		ginx.NewRender(c).Data(obj.Stdout, err)
		return
	}

	if config.C.Output.AgtdPort <= 0 || config.C.Output.AgtdPort > 65535 {
		ginx.NewRender(c).Message(fmt.Errorf("remotePort(%d) invalid", config.C.Output.AgtdPort))
		return
	}

	url := fmt.Sprintf("http://%s:%d/output/%d/stdout.json", host, config.C.Output.AgtdPort, id)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(url)
	errorx.Dangerous(err)

	defer resp.Body.Close()

	bs, err := ioutil.ReadAll(resp.Body)
	errorx.Dangerous(err)

	c.Writer.Header().Set("Content-Type", "application/json; charset=UTF-8")
	c.Writer.Write(bs)
}

func (rou *Router) taskHostStderr(c *gin.Context) {
	id := UrlParamsInt64(c, "id")
	host := ginx.UrlParamStr(c, "host")

	if config.C.Output.ComeFrom == "database" || config.C.Output.ComeFrom == "" {
		obj, err := models.TaskHostGet(rou.ctx, id, host)
		ginx.NewRender(c).Data(obj.Stderr, err)
		return
	}

	if config.C.Output.AgtdPort <= 0 || config.C.Output.AgtdPort > 65535 {
		ginx.NewRender(c).Message(fmt.Errorf("remotePort(%d) invalid", config.C.Output.AgtdPort))
		return
	}

	url := fmt.Sprintf("http://%s:%d/output/%d/stderr.json", host, config.C.Output.AgtdPort, id)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(url)
	errorx.Dangerous(err)

	defer resp.Body.Close()

	bs, err := ioutil.ReadAll(resp.Body)
	errorx.Dangerous(err)

	c.Writer.Header().Set("Content-Type", "application/json; charset=UTF-8")
	c.Writer.Write(bs)
}

func (rou *Router) taskStdoutTxt(c *gin.Context) {
	id := UrlParamsInt64(c, "id")

	meta, err := models.TaskMetaGet(rou.ctx, "id = ?", id)
	if err != nil {
		c.String(500, err.Error())
		return
	}

	if meta == nil {
		c.String(404, "no such task")
		return
	}

	stdouts, err := meta.Stdouts(rou.ctx)
	if err != nil {
		c.String(500, err.Error())
		return
	}

	w := c.Writer

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	count := len(stdouts)
	for i := 0; i < count; i++ {
		if i != 0 {
			w.Write([]byte("\n\n"))
		}

		w.Write([]byte(stdouts[i].Host + ":\n"))
		w.Write([]byte(stdouts[i].Stdout))
	}
}

func (rou *Router) taskStderrTxt(c *gin.Context) {
	id := UrlParamsInt64(c, "id")

	meta, err := models.TaskMetaGet(rou.ctx, "id = ?", id)
	if err != nil {
		c.String(500, err.Error())
		return
	}

	if meta == nil {
		c.String(404, "no such task")
		return
	}

	stderrs, err := meta.Stderrs(rou.ctx)
	if err != nil {
		c.String(500, err.Error())
		return
	}

	w := c.Writer

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	count := len(stderrs)
	for i := 0; i < count; i++ {
		if i != 0 {
			w.Write([]byte("\n\n"))
		}

		w.Write([]byte(stderrs[i].Host + ":\n"))
		w.Write([]byte(stderrs[i].Stderr))
	}
}

type TaskStdoutData struct {
	Host   string `json:"host"`
	Stdout string `json:"stdout"`
}

type TaskStderrData struct {
	Host   string `json:"host"`
	Stderr string `json:"stderr"`
}

func (rou *Router) taskStdoutJSON(c *gin.Context) {
	task := TaskMeta(rou.ctx, UrlParamsInt64(c, "id"))

	host := ginx.QueryStr(c, "host", "")

	var hostsLen int
	var ret []TaskStdoutData

	if host != "" {
		obj, err := models.TaskHostGet(rou.ctx, task.Id, host)
		if err != nil {
			ginx.NewRender(c).Data("", err)
			return
		} else if obj == nil {
			ginx.NewRender(c).Data("", fmt.Errorf("task: %d, host(%s) not eixsts", task.Id, host))
			return
		} else {
			ret = append(ret, TaskStdoutData{
				Host:   host,
				Stdout: obj.Stdout,
			})
		}
	} else {
		hosts, err := models.TaskHostGets(rou.ctx, task.Id)
		if err != nil {
			ginx.NewRender(c).Data("", err)
			return
		}

		hostsLen = len(hosts)

		ret = make([]TaskStdoutData, 0, hostsLen)
		for i := 0; i < hostsLen; i++ {
			ret = append(ret, TaskStdoutData{
				Host:   hosts[i].Host,
				Stdout: hosts[i].Stdout,
			})
		}
	}

	ginx.NewRender(c).Data(ret, nil)
}

func (rou *Router) taskStderrJSON(c *gin.Context) {
	task := TaskMeta(rou.ctx, UrlParamsInt64(c, "id"))

	host := ginx.QueryStr(c, "host", "")

	var hostsLen int
	var ret []TaskStderrData

	if host != "" {
		obj, err := models.TaskHostGet(rou.ctx, task.Id, host)
		if err != nil {
			ginx.NewRender(c).Data("", err)
			return
		} else if obj == nil {
			ginx.NewRender(c).Data("", fmt.Errorf("task: %d, host(%s) not eixsts", task.Id, host))
			return
		} else {
			ret = append(ret, TaskStderrData{
				Host:   host,
				Stderr: obj.Stderr,
			})
		}
	} else {
		hosts, err := models.TaskHostGets(rou.ctx, task.Id)
		if err != nil {
			ginx.NewRender(c).Data("", err)
			return
		}

		hostsLen = len(hosts)

		ret = make([]TaskStderrData, 0, hostsLen)
		for i := 0; i < hostsLen; i++ {
			ret = append(ret, TaskStderrData{
				Host:   hosts[i].Host,
				Stderr: hosts[i].Stderr,
			})
		}
	}

	ginx.NewRender(c).Data(ret, nil)
}

type taskForm struct {
	Title          string   `json:"title" binding:"required"`
	Account        string   `json:"account" binding:"required"`
	Batch          int      `json:"batch"`
	Tolerance      int      `json:"tolerance"`
	Timeout        int      `json:"timeout"`
	Pause          string   `json:"pause"`
	Script         string   `json:"script" binding:"required"`
	Args           string   `json:"args"`
	Stdin          string   `json:"stdin"`
	Action         string   `json:"action" binding:"required"`
	Creator        string   `json:"creator" binding:"required"`
	Hosts          []string `json:"hosts" binding:"required"`
	AlertTriggered bool     `json:"alert_triggered"`
}

func (rou *Router) taskAdd(c *gin.Context) {
	var f taskForm
	ginx.BindJSON(c, &f)

	hosts := cleanHosts(f.Hosts)
	if len(hosts) == 0 {
		errorx.Bomb(http.StatusBadRequest, "arg(hosts) empty")
	}

	taskMeta := &models.TaskMeta{
		Title:     f.Title,
		Account:   f.Account,
		Batch:     f.Batch,
		Tolerance: f.Tolerance,
		Timeout:   f.Timeout,
		Pause:     f.Pause,
		Script:    f.Script,
		Args:      f.Args,
		Stdin:     f.Stdin,
		Creator:   f.Creator,
	}

	err := taskMeta.CleanFields()
	ginx.Dangerous(err)
	taskMeta.HandleFH(hosts[0])

	authUser := c.MustGet(gin.AuthUserKey).(string)
	// 任务类型分为"告警规则触发"和"n9e center用户下发"两种；
	// 边缘机房"告警规则触发"的任务不需要规划，并且它可能是失联的，无法使用db资源，所以放入redis缓存中，直接下发给agentd执行
	if !config.C.IsCenter && f.AlertTriggered {
		if err := taskMeta.Create(rou.ctx); err != nil {
			// 当网络不连通时，生成唯一的id，防止边缘机房中不同任务的id相同；
			// 方法是，redis自增id去防止同一个机房的不同n9e edge生成的id相同；
			// 但没法防止不同边缘机房生成同样的id，所以，生成id的数据不会上报存入数据库，只用于闭环执行。
			taskMeta.Id, err = storage.IdGet(rou.ctx.Redis)
			ginx.Dangerous(err)
		}
		if err == nil {
			taskHost := models.TaskHost{
				Id:     taskMeta.Id,
				Host:   hosts[0],
				Status: "running",
			}
			if err = taskHost.Create(rou.ctx); err != nil {
				logger.Warningf("task_add_fail: authUser=%s title=%s err=%s", authUser, taskMeta.Title, err.Error())
			}
		}

		// 缓存任务元信息和待下发的任务
		err = taskMeta.Cache(rou.ctx, hosts[0])
		ginx.Dangerous(err)

	} else {
		// 如果是中心机房，还是保持之前的逻辑
		err = taskMeta.Save(rou.ctx, hosts, f.Action)
		ginx.Dangerous(err)
	}

	logger.Infof("task_add_succ: authUser=%s title=%s", authUser, taskMeta.Title)

	ginx.NewRender(c).Data(taskMeta.Id, err)
}

func (rou *Router) taskGet(c *gin.Context) {
	meta := TaskMeta(rou.ctx, UrlParamsInt64(c, "id"))

	hosts, err := meta.Hosts(rou.ctx)
	errorx.Dangerous(err)

	action, err := meta.Action(rou.ctx)
	errorx.Dangerous(err)

	actionStr := ""
	if action != nil {
		actionStr = action.Action
	} else {
		meta.Done = true
	}

	ginx.NewRender(c).Data(gin.H{
		"meta":   meta,
		"hosts":  hosts,
		"action": actionStr,
	}, nil)
}

// 传进来一堆ids，返回已经done的任务的ids
func (rou *Router) doneIds(c *gin.Context) {
	ids := ginx.QueryStr(c, "ids", "")
	if ids == "" {
		errorx.Dangerous("arg(ids) empty")
	}

	idsint64 := str.IdsInt64(ids, ",")
	if len(idsint64) == 0 {
		errorx.Dangerous("arg(ids) empty")
	}

	exists, err := models.TaskActionExistsIds(rou.ctx, idsint64)
	errorx.Dangerous(err)

	dones := slice.SubInt64(idsint64, exists)
	ginx.NewRender(c).Data(gin.H{
		"list": dones,
	}, nil)
}

func (rou *Router) taskGets(c *gin.Context) {
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 20)
	creator := ginx.QueryStr(c, "creator", "")
	days := ginx.QueryInt64(c, "days", 7)

	before := time.Unix(time.Now().Unix()-days*24*3600, 0)

	total, err := models.TaskMetaTotal(rou.ctx, creator, query, before)
	errorx.Dangerous(err)

	list, err := models.TaskMetaGets(rou.ctx, creator, query, before, limit, ginx.Offset(c, limit))
	errorx.Dangerous(err)

	cnt := len(list)
	ids := make([]int64, cnt)
	for i := 0; i < cnt; i++ {
		ids[i] = list[i].Id
	}

	exists, err := models.TaskActionExistsIds(rou.ctx, ids)
	errorx.Dangerous(err)

	for i := 0; i < cnt; i++ {
		if slice.ContainsInt64(exists, list[i].Id) {
			list[i].Done = false
		} else {
			list[i].Done = true
		}
	}

	ginx.NewRender(c).Data(gin.H{
		"total": total,
		"list":  list,
	}, nil)
}

type actionForm struct {
	Action string `json:"action"`
}

func (rou *Router) taskAction(c *gin.Context) {
	meta := TaskMeta(rou.ctx, UrlParamsInt64(c, "id"))

	var f actionForm
	ginx.BindJSON(c, &f)

	action, err := models.TaskActionGet(rou.ctx, "id=?", meta.Id)
	errorx.Dangerous(err)

	if action == nil {
		errorx.Bomb(200, "task already finished, no more action can do")
	}

	ginx.NewRender(c).Message(action.Update(rou.ctx, f.Action))
}

func (rou *Router) taskHostAction(c *gin.Context) {
	host := ginx.UrlParamStr(c, "host")
	meta := TaskMeta(rou.ctx, UrlParamsInt64(c, "id"))

	noopWhenDone(rou.ctx, meta.Id)

	var f actionForm
	ginx.BindJSON(c, &f)

	if f.Action == "ignore" {
		errorx.Dangerous(meta.IgnoreHost(rou.ctx, host))

		action, err := models.TaskActionGet(rou.ctx, "id=?", meta.Id)
		errorx.Dangerous(err)

		if action != nil && action.Action == "pause" {
			ginx.NewRender(c).Data("you can click start to run the task", nil)
			return
		}
	}

	if f.Action == "kill" {
		errorx.Dangerous(meta.KillHost(rou.ctx, host))
	}

	if f.Action == "redo" {
		errorx.Dangerous(meta.RedoHost(rou.ctx, host))
	}

	ginx.NewRender(c).Message(nil)
}

func noopWhenDone(ctx *ctx.Context, id int64) {
	action, err := models.TaskActionGet(ctx, "id=?", id)
	errorx.Dangerous(err)

	if action == nil {
		errorx.Bomb(200, "task already finished, no more taskAction can do")
	}
}

type sqlCondForm struct {
	Table string
	Where string
	Args  []interface{}
}

func (rou *Router) tableRecordListGet(c *gin.Context) {
	var f sqlCondForm
	ginx.BindJSON(c, &f)
	switch f.Table {
	case models.TaskHostDoing{}.TableName():
		lst, err := models.TableRecordGets[[]models.TaskHostDoing](rou.ctx, f.Table, f.Where, f.Args)
		ginx.NewRender(c).Data(lst, err)
	case models.TaskMeta{}.TableName():
		lst, err := models.TableRecordGets[[]models.TaskMeta](rou.ctx, f.Table, f.Where, f.Args)
		ginx.NewRender(c).Data(lst, err)
	default:
		ginx.Bomb(http.StatusBadRequest, "table[%v] not support", f.Table)
	}
}

func (rou *Router) tableRecordCount(c *gin.Context) {
	var f sqlCondForm
	ginx.BindJSON(c, &f)
	ginx.NewRender(c).Data(models.TableRecordCount(rou.ctx, f.Table, f.Where, f.Args))
}

type markDoneForm struct {
	Id     int64
	Clock  int64
	Host   string
	Status string
	Stdout string
	Stderr string
}

func (rou *Router) markDone(c *gin.Context) {
	var f markDoneForm
	ginx.BindJSON(c, &f)
	ginx.NewRender(c).Message(models.MarkDoneStatus(rou.ctx, f.Id, f.Clock, f.Host, f.Status, f.Stdout, f.Stderr))
}

func (rou *Router) taskMetaAdd(c *gin.Context) {
	var f models.TaskMeta
	ginx.BindJSON(c, &f)
	err := f.Create(rou.ctx)
	ginx.NewRender(c).Data(f.Id, err)
}

func (rou *Router) taskHostAdd(c *gin.Context) {
	var f models.TaskHost
	ginx.BindJSON(c, &f)
	ginx.NewRender(c).Message(f.Upsert(rou.ctx))
}

func (rou *Router) taskHostUpsert(c *gin.Context) {
	var f []models.TaskHost
	ginx.BindJSON(c, &f)
	ginx.NewRender(c).Data(models.TaskHostUpserts(rou.ctx, f))
}

func UrlParamsInt64(c *gin.Context, field string) int64 {

	var params []gin.Param
	for _, p := range c.Params {
		if p.Key == "id" {
			params = append(params, p)
		}
	}

	var strval string
	if len(params) == 1 {
		strval = ginx.UrlParamStr(c, field)
	} else if len(params) == 2 {
		strval = params[1].Value
	} else {
		logger.Warningf("url param[%+v] not ok", params)
		errorx.Bomb(http.StatusBadRequest, "url param[%s] is blank", field)
	}

	intval, err := strconv.ParseInt(strval, 10, 64)
	if err != nil {
		errorx.Bomb(http.StatusBadRequest, "cannot convert %s to int64", strval)
	}

	return intval
}
