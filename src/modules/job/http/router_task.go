package http

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"
	"github.com/toolkits/pkg/slice"

	"github.com/didi/nightingale/src/common/address"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/job/config"
)

type taskForm struct {
	Title     string   `json:"title"`
	Account   string   `json:"account"`
	Batch     int      `json:"batch"`
	Tolerance int      `json:"tolerance"`
	Timeout   int      `json:"timeout"`
	Pause     string   `json:"pause"`
	Script    string   `json:"script"`
	Args      string   `json:"args"`
	Action    string   `json:"action"`
	Hosts     []string `json:"hosts"`
}

func taskPost(c *gin.Context) {
	user := loginUser(c)

	var f taskForm
	bind(c, &f)
	hosts := cleanHosts(f.Hosts)
	if len(hosts) == 0 {
		bomb("arg[hosts] empty")
	}

	checkTaskPerm(hosts, user, f.Account)

	task := &models.TaskMeta{
		Title:     f.Title,
		Account:   f.Account,
		Batch:     f.Batch,
		Tolerance: f.Tolerance,
		Timeout:   f.Timeout,
		Pause:     f.Pause,
		Script:    f.Script,
		Args:      f.Args,
		Creator:   user.Username,
	}

	dangerous(task.Save(hosts, f.Action))
	renderData(c, task.Id, nil)
}

func checkTaskPerm(hosts []string, user *models.User, account string) {
	if user.IsRooter() {
		return
	}

	ids, err := models.ResourceIdsByIdents(hosts)
	dangerous(err)

	if len(ids) == 0 {
		bomb("hosts invalid")
	}

	nopriv, err := user.NopriResIdents(ids, accountToOP(account))
	dangerous(err)

	if len(nopriv) > 0 {
		hostsStr := strings.Join(nopriv, ", ")
		logger.Errorf("no privilege, username: %s, run_account: %s, hosts: %s", user.Username, account, hostsStr)
		bomb("no privilege: %s", hostsStr)
	}
}

func taskGets(c *gin.Context) {
	username := loginUsername(c)

	query := queryStr(c, "query", "")
	limit := queryInt(c, "limit", 20)
	mine := queryBool(c, "mine", false)
	days := queryInt64(c, "days", 7)

	creator := username
	if !mine {
		creator = ""
	}

	before := time.Unix(time.Now().Unix()-days*24*3600, 0)

	total, err := models.TaskMetaTotal(creator, query, before)
	dangerous(err)

	list, err := models.TaskMetaGets(creator, query, before, limit, offset(c, limit))
	dangerous(err)

	cnt := len(list)
	ids := make([]int64, cnt)
	for i := 0; i < cnt; i++ {
		ids[i] = list[i].Id
	}

	exists, err := models.TaskActionExistsIds(ids)
	dangerous(err)

	for i := 0; i < cnt; i++ {
		if slice.ContainsInt64(exists, list[i].Id) {
			list[i].Done = false
		} else {
			list[i].Done = true
		}
	}

	renderData(c, gin.H{
		"total": total,
		"list":  list,
	}, nil)
}

func taskView(c *gin.Context) {
	meta := TaskMeta(urlParamInt64(c, "id"))

	hosts, err := meta.Hosts()
	dangerous(err)

	action, err := meta.Action()
	dangerous(err)

	actionStr := ""
	if action != nil {
		actionStr = action.Action
	} else {
		meta.Done = true
	}

	renderData(c, gin.H{
		"meta":   meta,
		"hosts":  hosts,
		"action": actionStr,
	}, nil)
}

type taskActionForm struct {
	Action string `json:"action"`
}

func taskActionPut(c *gin.Context) {
	user := loginUser(c)
	meta := TaskMeta(urlParamInt64(c, "id"))

	var f taskActionForm
	bind(c, &f)

	action, err := models.TaskActionGet("id=?", meta.Id)
	dangerous(err)

	if action == nil {
		bomb("Oops, action[%d] not found", meta.Id)
	}

	if meta.Creator != user.Username {
		hosts, err := meta.HostStrs()
		dangerous(err)
		checkTaskPerm(hosts, user, meta.Account)
	}

	renderMessage(c, action.Update(f.Action))
}

type taskHostForm struct {
	Action string `json:"action"`
	Host   string `json:"host"`
}

func taskHostPut(c *gin.Context) {
	user := loginUser(c)
	meta := TaskMeta(urlParamInt64(c, "id"))
	noopWhenDone(meta.Id)

	var f taskHostForm
	bind(c, &f)

	if meta.Creator != user.Username {
		checkTaskPerm([]string{f.Host}, user, meta.Account)
	}

	if f.Action == "ignore" {
		dangerous(meta.IgnoreHost(f.Host))

		action, err := models.TaskActionGet("id=?", meta.Id)
		dangerous(err)

		if action != nil && action.Action == "pause" {
			renderData(c, "you can click start to run the task", nil)
			return
		}
	}

	if f.Action == "kill" {
		dangerous(meta.KillHost(f.Host))
	}

	if f.Action == "redo" {
		dangerous(meta.RedoHost(f.Host))
	}

	renderMessage(c, nil)
}

func noopWhenDone(id int64) {
	action, err := models.TaskActionGet("id=?", id)
	dangerous(err)

	if action == nil {
		bomb("task already finished")
	}
}

func taskStdout(c *gin.Context) {
	meta := TaskMeta(urlParamInt64(c, "id"))
	stdouts, err := meta.Stdouts()
	renderData(c, stdouts, err)
}

func taskStderr(c *gin.Context) {
	meta := TaskMeta(urlParamInt64(c, "id"))
	stderrs, err := meta.Stderrs()
	renderData(c, stderrs, err)
}

func apiTaskState(c *gin.Context) {
	meta := TaskMeta(urlParamInt64(c, "id"))

	action, err := models.TaskActionGet("id=?", meta.Id)
	if err != nil {
		renderData(c, "", err)
		return
	}

	state := "done"
	if action != nil {
		state = action.Action
	}

	renderData(c, state, nil)
}

func apiTaskResult(c *gin.Context) {
	task := TaskMeta(urlParamInt64(c, "id"))

	hosts, err := models.TaskHostStatus(task.Id)
	if err != nil {
		bomb("load task hosts of %d occur error %v", task.Id, err)
	}

	ss := make(map[string][]string)
	total := len(hosts)
	for i := 0; i < total; i++ {
		s := hosts[i].Status
		ss[s] = append(ss[s], hosts[i].Host)
	}

	renderData(c, ss, nil)
}

func taskHostOutput(c *gin.Context) {
	meta := TaskMeta(urlParamInt64(c, "id"))
	obj, err := models.TaskHostGet(meta.Id, urlParamStr(c, "host"))
	renderData(c, obj, err)
}

func taskHostStdout(c *gin.Context) {
	id := urlParamInt64(c, "id")
	host := urlParamStr(c, "host")

	if config.Config.Output.ComeFrom == "database" || config.Config.Output.ComeFrom == "" {
		obj, err := models.TaskHostGet(id, host)
		renderData(c, obj.Stdout, err)
		return
	}

	if config.Config.Output.RemotePort <= 0 || config.Config.Output.RemotePort > 65535 {
		renderMessage(c, fmt.Errorf("remotePort[%d] invalid", config.Config.Output.RemotePort))
		return
	}

	url := fmt.Sprintf("http://%s:%d/output/%d/stdout.json", host, config.Config.Output.RemotePort, id)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(url)
	dangerous(err)

	defer resp.Body.Close()

	bs, err := ioutil.ReadAll(resp.Body)
	dangerous(err)

	c.Writer.Header().Set("Content-Type", "application/json; charset=UTF-8")
	c.Writer.Write(bs)
}

func taskHostStderr(c *gin.Context) {
	id := urlParamInt64(c, "id")
	host := urlParamStr(c, "host")

	if config.Config.Output.ComeFrom == "database" || config.Config.Output.ComeFrom == "" {
		obj, err := models.TaskHostGet(id, host)
		renderData(c, obj.Stderr, err)
		return
	}

	if config.Config.Output.RemotePort <= 0 || config.Config.Output.RemotePort > 65535 {
		renderMessage(c, fmt.Errorf("remotePort[%d] invalid", config.Config.Output.RemotePort))
		return
	}

	url := fmt.Sprintf("http://%s:%d/output/%d/stderr.json", host, config.Config.Output.RemotePort, id)
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(url)
	dangerous(err)

	defer resp.Body.Close()

	bs, err := ioutil.ReadAll(resp.Body)
	dangerous(err)

	c.Writer.Header().Set("Content-Type", "application/json; charset=UTF-8")
	c.Writer.Write(bs)
}

func taskStdoutTxt(c *gin.Context) {
	meta := TaskMeta(urlParamInt64(c, "id"))

	stdouts, err := meta.Stdouts()
	dangerous(err)

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

func taskStderrTxt(c *gin.Context) {
	meta := TaskMeta(urlParamInt64(c, "id"))

	stderrs, err := meta.Stderrs()
	dangerous(err)

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

func apiTaskJSONStdouts(c *gin.Context) {
	task := TaskMeta(urlParamInt64(c, "id"))

	host := queryStr(c, "host", "")

	var hostsLen int
	var ret []TaskStdoutData

	if host != "" {
		obj, err := models.TaskHostGet(task.Id, host)
		if err != nil {
			renderData(c, "", err)
			return
		} else if obj == nil {
			renderData(c, "", fmt.Errorf("task: %d, host(%s) not eixsts", task.Id, host))
			return
		} else {
			ret = append(ret, TaskStdoutData{
				Host:   host,
				Stdout: obj.Stdout,
			})
		}
	} else {
		hosts, err := models.TaskHostGets(task.Id)
		if err != nil {
			renderData(c, "", err)
			return
		}

		hostsLen = len(hosts)

		for i := 0; i < hostsLen; i++ {
			ret = append(ret, TaskStdoutData{
				Host:   hosts[i].Host,
				Stdout: hosts[i].Stdout,
			})
		}
	}

	renderData(c, ret, nil)
}

func apiTaskJSONStderrs(c *gin.Context) {
	task := TaskMeta(urlParamInt64(c, "id"))

	host := queryStr(c, "host", "")

	var hostsLen int
	var ret []TaskStderrData

	if host != "" {
		obj, err := models.TaskHostGet(task.Id, host)
		if err != nil {
			renderData(c, "", err)
			return
		} else if obj == nil {
			renderData(c, "", fmt.Errorf("task: %d, host(%s) not eixsts", task.Id, host))
			return
		} else {
			ret = append(ret, TaskStderrData{
				Host:   host,
				Stderr: obj.Stderr,
			})
		}
	} else {
		hosts, err := models.TaskHostGets(task.Id)
		if err != nil {
			renderData(c, "", err)
			return
		}

		hostsLen = len(hosts)

		for i := 0; i < hostsLen; i++ {
			ret = append(ret, TaskStderrData{
				Host:   hosts[i].Host,
				Stderr: hosts[i].Stderr,
			})
		}
	}

	renderData(c, ret, nil)
}

type callbackForm struct {
	Id          int64    `json:"id"`
	Sid         int64    `json:"sid"`
	Sname       string   `json:"sname"`
	NodePath    string   `json:"node_path"`
	Nid         int64    `json:"nid"`
	Endpoint    string   `json:"endpoint"`
	Priority    int      `json:"priority"`
	EventType   string   `json:"event_type"` // alert|recovery
	Category    int      `json:"category"`
	Status      uint16   `json:"status"`
	HashId      uint64   `json:"hashid"`
	Etime       int64    `json:"etime"`
	Value       string   `json:"value"`
	Info        string   `json:"info"`
	LastUpdator string   `json:"last_updator"`
	Groups      []string `json:"groups"`
	Users       []string `json:"users"`
}

// 这里偷个懒，如果回调失败，不通知相关人员了，直接看MON和JOB日志来排查吧
func taskCallback(c *gin.Context) {
	var f callbackForm
	bind(c, &f)

	etype := strings.ToLower(f.EventType)
	if !(etype == "alert" || etype == "problem") {
		logger.Infof("callback: not alert, no need to run task, nodeid:%d, nodepath:%s, sname:%s", f.Nid, f.NodePath, f.Sname)
		renderMessage(c, "not alert, no need to run task")
		return
	}

	// 如果给了就用给的，否则就用事件里边的
	host := queryStr(c, "host", "")
	if host == "" {
		host = f.Endpoint
	}

	if host == "" {
		logger.Errorf("callback: host is blank, nodeid:%d, nodepath:%s, sname:%s", f.Nid, f.NodePath, f.Sname)
		bomb("host is blank")
	}

	// tplid是必须的，要不然怎么知道跑哪个脚本
	tplid := queryInt64(c, "tplid", 0)
	if tplid == 0 {
		tplid = queryInt64(c, "tpl_id", 0)
	}

	if tplid == 0 {
		logger.Errorf("callback: tplid is 0, nodeid:%d, nodepath:%s, sname:%s", f.Nid, f.NodePath, f.Sname)
		bomb("tplid is 0")
	}

	tpl, err := models.TaskTplGet("id=?", tplid)
	if err != nil {
		logger.Errorf("callback: cannot query tpl[id:%d]:%v, nodeid:%d, nodepath:%s, sname:%s", tplid, err, f.Nid, f.NodePath, f.Sname)
		bomb("cannot query tpl[id:%d]:%v", tplid, err)
	}

	if tpl == nil {
		logger.Errorf("callback: tpl[id:%d] is nil, nodeid:%d, nodepath:%s, sname:%s", tplid, f.Nid, f.NodePath, f.Sname)
		bomb("tpl[id:%d] is nil", tplid)
	}

	// 策略的最后修改人员需要对机器有操作权限才可以
	user, err := models.UserGet("username=?", f.LastUpdator)
	if err != nil {
		logger.Errorf("UserGet by lastUpdator(%s) fail: %s", f.LastUpdator, err)
		dangerous(err)
	}

	if user == nil {
		bomb("user:%s not found", f.LastUpdator)
	}

	checkTaskPerm([]string{host}, user, tpl.Account)

	task := &models.TaskMeta{
		Title:     tpl.Title + " by " + f.Sname,
		Account:   tpl.Account,
		Tolerance: tpl.Tolerance,
		Timeout:   tpl.Timeout,
		Script:    tpl.Script,
		Args:      tpl.Args,
		Creator:   user.Username,
	}

	err = task.Save([]string{host}, "start")
	if err != nil {
		logger.Errorf("callback: cannot create task[tplid:%d]:%v, nodeid:%d, nodepath:%s, sname:%s", tplid, err, f.Nid, f.NodePath, f.Sname)
		bomb("cannot create task: %v", err)
	}

	renderMessage(c, nil)
}

// 这个数据结构是tt回调的时候使用的通用数据结构，里边既有工单基本信息，也有结构化数据，job这里只需要从中解析出结构化数据
type ttForm struct {
	Id       int64                  `json:"id" binding:"required"`
	RunUser  string                 `json:"runUser" binding:"required"`
	Form     map[string]interface{} `json:"form" binding:"required"`
	Approval int                    `json:"approval"`
}

// /api/job-ce/run/:id?hosts=10.3.4.5,10.4.5.6
func taskRunForTT(c *gin.Context) {
	var f ttForm
	bind(c, &f)

	action := c.Request.Host + c.Request.URL.Path
	if f.Approval == 2 {
		renderMessage(c, "该任务未通过审批")
		return
	}
	tpl := TaskTpl(urlParamInt64(c, "id"))
	arr, err := tpl.Hosts()
	dangerous(err)

	// 如果QueryString里带有hosts参数，就用QueryString里的机器列表
	// 否则就从结构化数据中解析hosts
	// 如果结构化数据中也没有，那只能有模板里的，模板里也没有就报错
	hosts := queryStr(c, "hosts", "")

	if hosts != "" {
		// 使用QueryString传过来的hosts
		tmp := cleanHosts(strings.Split(hosts, ","))
		if len(tmp) > 0 {
			arr = tmp
		}
	} else {
		if v, ok := f.Form["hosts"]; ok {
			hosts = v.(string)
			hosts = strings.ReplaceAll(hosts, "\r", ",")
			hosts = strings.ReplaceAll(hosts, "\n", ",")
			tmp := cleanHosts(strings.Split(hosts, ","))
			if len(tmp) > 0 {
				arr = tmp
			}
		}
	}

	if len(arr) == 0 {
		bomb("hosts empty")
	}

	// 校验权限
	user := loginUser(c)
	checkTaskPerm(arr, user, tpl.Account)

	task := &models.TaskMeta{
		Title:     tpl.Title,
		Account:   tpl.Account,
		Batch:     tpl.Batch,
		Tolerance: tpl.Tolerance,
		Timeout:   tpl.Timeout,
		Pause:     tpl.Pause,
		Script:    tpl.Script,
		Creator:   user.Username,
	}

	task.Args = ""
	for k, v := range f.Form {
		switch v.(type) {
		case string:
			if k == "hosts" {
				tmp := v.(string)
				tmp = strings.ReplaceAll(tmp, "\r", ",")
				tmp = strings.ReplaceAll(tmp, "\n", ",")
				tmpArray := cleanHosts(strings.Split(hosts, ","))
				if len(tmpArray) > 0 {
					v = strings.Join(tmpArray, ",")
				}

			}
			if len(v.(string)) < 1600 {
				task.Args += fmt.Sprintf("--%s=%s,,", k, v.(string))
			}
		case int:
			task.Args += fmt.Sprintf("--%s=%d,,", k, v.(int))
		case int64:
			task.Args += fmt.Sprintf("--%s=%d,,", k, v.(int64))
		case float64:
			//TODO 暂时不支持传非整型
			task.Args += fmt.Sprintf("--%s=%d,,", k, int64(v.(float64)))
		}
	}

	task.Args = strings.TrimSuffix(task.Args, ",,")

	dangerous(task.Save(arr, "start"))
	go func() {
		var arr2Map = map[string]int{}
		for _, a := range arr {
			arr2Map[a] = 1
		}

		for {
			var (
				restHosts = map[string]int{}
			)
			for h, _ := range arr2Map {
				th, err := models.TaskHostGet(task.Id, h)
				if err == nil {
					if th.Status == "killed" {
						reply := fmt.Sprintf("### Job通知推送\n* Job平台任务(ID:%d)在机器%s中执行失败，"+
							"原因为task被kill掉\n* 执行action接口地址为: %s\n* 标准输出: %s\n* 错误输出: %s\n",
							task.Id, h, action, th.Stdout, th.Stderr)
						err = TicketSender(f.Id, action, "task has been killed", reply, -1,
							nil)
						if err != nil {
							logger.Errorf("send callback to ticket, err: %v", err)
						}
					} else if th.Status == "failed" {
						reply := fmt.Sprintf("### Job通知推送\n* Job平台任务(ID:%d)在机器%s中执行失败，"+
							"详情见错误输出\n* 执行action接口地址为: %s\n* 标准输出: %s\n* 错误输出: %s\n",
							task.Id, h, action, th.Stdout, th.Stderr)
						err = TicketSender(f.Id, action, "run task failed", reply, -1,
							nil)
						if err != nil {
							logger.Errorf("send callback to ticket, err: %v", err)
						}
					} else if th.Status == "timeout" {
						reply := fmt.Sprintf("### Job通知推送\n* Job平台任务(ID:%d)在机器%s中执行超时"+
							"\n* 执行action接口地址为: %s\n* 标准输出: %s\n* 错误输出: %s\n",
							task.Id, h, action, th.Stdout, th.Stderr)
						err = TicketSender(f.Id, action, "run task failed", reply, -1,
							nil)
						if err != nil {
							logger.Errorf("send callback to ticket, err: %v", err)
						}
					} else if th.Status == "success" {
						reply := fmt.Sprintf("### Job通知推送\n* Job平台任务(ID:%d)在机器%s中执行成功"+
							"\n* 执行action接口地址为: %s\n* 标准输出: %s\n* 错误输出: %s\n",
							task.Id, h, action, th.Stdout, th.Stderr)
						err = TicketSender(f.Id, action, "task ", reply, 1,
							nil)
						if err != nil {
							logger.Errorf("send callback to ticket, err: %v", err)
						}
					} else {
						restHosts[h] = 1
					}
				} else {
					logger.Errorf("get task_host err: %v", err)
				}
			}

			arr2Map = restHosts
			time.Sleep(time.Second)
		}
	}()

	go func() {
		time.Sleep(time.Second)
		reply := fmt.Sprintf("[任务详情请关注Job平台任务(ID:%d)详情页地址](%s)", task.Id, fmt.Sprintf("/job/tasks/%d/result", task.Id))
		err = TicketSender(f.Id, action, "", reply, -1,
			nil)
		if err != nil {
			logger.Errorf("send callback to ticket, err: %v", err)
		}
	}()

	renderData(c, gin.H{"taskID": task.Id, "detailPage": fmt.Sprintf("/job/tasks/%d/result", task.Id)}, nil)
}

type ticketCallBackForm struct {
	TicketId   int64       `json:"ticketId" binding:"required"`
	ActionApi  string      `json:"actionApi" binding:"required"`
	SystemName string      `json:"systemName" binding:"required"`
	Success    int         `json:"success" binding:"required"`
	Reason     string      `json:"reason"`
	Info       interface{} `json:"info"`
	AutoReply  string      `json:"autoReply"`
}

func TicketSender(id int64, action, reason, reply string, result int, info interface{}) error {
	addr := address.GetHTTPListen("ticket")

	data := ticketCallBackForm{
		TicketId:  id,
		ActionApi: action,
		Success:   result,
		Reason:    reason,
		Info:      info,
		AutoReply: reply,
	}

	url := fmt.Sprintf("%s/v1/ticket/callback?systemName=job", addr)
	if !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
		url = "http://" + url
	}

	res, code, err := httplib.PostJSON(url, time.Second*5, data, map[string]string{"x-srv-token": "ticket-builtin-token"})
	if err != nil {
		logger.Errorf("call sender api failed, server: %v, data: %+v, err: %v, resp:%v, status code:%d", url, data, err, string(res), code)
		return err
	}

	if code != 200 {
		logger.Errorf("call sender api failed, server: %v, data: %+v, resp:%v, code:%d", url, data, string(res), code)
		return err
	}

	logger.Debugf("ticket response %s", string(res))

	return nil
}
