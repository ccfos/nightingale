package http

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/models"
)

func taskTplGets(c *gin.Context) {
	query := queryStr(c, "query", "")
	limit := queryInt(c, "limit", 20)

	nodeId := queryInt64(c, "nid")

	total, err := models.TaskTplTotal(nodeId, query)
	dangerous(err)

	list, err := models.TaskTplGets(nodeId, query, limit, offset(c, limit))
	dangerous(err)

	renderData(c, gin.H{
		"total": total,
		"list":  list,
	}, nil)
}

type taskTplForm struct {
	Title     string   `json:"title"`
	Batch     int      `json:"batch"`
	Tolerance int      `json:"tolerance"`
	Timeout   int      `json:"timeout"`
	Pause     string   `json:"pause"`
	Script    string   `json:"script"`
	Args      string   `json:"args"`
	Tags      string   `json:"tags"`
	Account   string   `json:"account"`
	Hosts     []string `json:"hosts"`
}

func taskTplPost(c *gin.Context) {
	user := loginUser(c)
	node := Node(queryInt64(c, "nid"))

	user.CheckPermByNode(node, "job_tpl_create")

	var f taskTplForm
	bind(c, &f)

	tpl := &models.TaskTpl{
		NodeId:    node.Id,
		Title:     f.Title,
		Batch:     f.Batch,
		Tolerance: f.Tolerance,
		Timeout:   f.Timeout,
		Pause:     f.Pause,
		Script:    f.Script,
		Args:      f.Args,
		Tags:      f.Tags,
		Account:   f.Account,
		Creator:   user.Username,
	}

	renderMessage(c, tpl.Save(f.Hosts))
}

func taskTplPut(c *gin.Context) {
	tpl := TaskTpl(urlParamInt64(c, "id"))

	user := loginUser(c)
	node := Node(tpl.NodeId)

	user.CheckPermByNode(node, "job_tpl_modify")

	var f taskTplForm
	bind(c, &f)

	tpl.Title = f.Title
	tpl.Batch = f.Batch
	tpl.Tolerance = f.Tolerance
	tpl.Timeout = f.Timeout
	tpl.Pause = f.Pause
	tpl.Script = f.Script
	tpl.Args = f.Args
	tpl.Tags = f.Tags
	tpl.Account = f.Account

	renderMessage(c, tpl.Update(f.Hosts))
}

func taskTplGet(c *gin.Context) {
	tpl := TaskTpl(urlParamInt64(c, "id"))

	user := loginUser(c)
	node := Node(tpl.NodeId)

	user.CheckPermByNode(node, "job_tpl_view")
	hosts, err := tpl.Hosts()
	renderData(c, gin.H{
		"tpl":   tpl,
		"hosts": hosts,
	}, err)
}

func taskTplDel(c *gin.Context) {
	tpl := TaskTpl(urlParamInt64(c, "id"))

	user := loginUser(c)
	node := Node(tpl.NodeId)

	user.CheckPermByNode(node, "job_tpl_delete")

	renderMessage(c, tpl.Del())
}

type taskTplTagsForm struct {
	Ids    []int64  `json:"ids"`
	Tags   string   `json:"tags"`
	TagArr []string `json:"-"`
	Act    string   `json:"act"`
}

func (f *taskTplTagsForm) Validate() {
	if f.Ids == nil || len(f.Ids) == 0 {
		bomb("arg[ids] empty")
	}

	if str.Dangerous(f.Tags) {
		bomb("arg[tags] dangerous")
	}

	if f.Act != "bind" && f.Act != "unbind" {
		bomb("arg[act] should be 'bind' or 'unbind'")
	}

	if f.Tags == "" {
		f.TagArr = []string{}
	} else {
		f.TagArr = strings.Split(f.Tags, ",")
	}

	cnt := len(f.TagArr)
	arr := make([]string, 0, cnt)
	for i := 0; i < cnt; i++ {
		if f.TagArr[i] == "" {
			continue
		}
		arr = append(arr, f.TagArr[i])
	}

	f.TagArr = arr

	if len(f.TagArr) == 0 {
		bomb("tags empty")
	}
}

func taskTplTagsPut(c *gin.Context) {
	user := loginUser(c)

	var f taskTplTagsForm
	bind(c, &f)
	f.Validate()

	cnt := len(f.Ids)
	for i := 0; i < cnt; i++ {
		tpl := TaskTpl(f.Ids[i])
		node := Node(tpl.NodeId)

		user.CheckPermByNode(node, "job_tpl_modify")

		if f.Act == "bind" {
			dangerous(tpl.BindTags(f.TagArr))
		} else {
			dangerous(tpl.UnbindTags(f.TagArr))
		}
	}

	renderMessage(c, nil)
}

type taskTplNodeForm struct {
	Ids    []int64 `json:"ids"`
	NodeId int64   `json:"node_id"`
}

// 批量修改任务模板所属节点
func taskTplNodePut(c *gin.Context) {
	var f taskTplNodeForm
	bind(c, &f)

	user := loginUser(c)
	dstNode := Node(f.NodeId)
	user.CheckPermByNode(dstNode, "job_tpl_modify")

	cnt := len(f.Ids)
	for i := 0; i < cnt; i++ {
		tpl := TaskTpl(f.Ids[i])
		node := Node(tpl.NodeId)
		user.CheckPermByNode(node, "job_tpl_modify")
		dangerous(tpl.UpdateGroup(f.NodeId))
	}

	renderMessage(c, nil)
}

type apiTaskForm struct {
	Action       string   `json:"action"`
	Title        string   `json:"title"`
	Account      string   `json:"account"`
	BatchStr     string   `json:"batch"`
	Batch        int      `json:"-"`
	ToleranceStr string   `json:"tolerance"`
	Tolerance    int      `json:"-"`
	TimeoutStr   string   `json:"timeout"`
	Timeout      int      `json:"-"`
	Pause        string   `json:"pause"`
	Script       string   `json:"script"`
	Args         string   `json:"args"`
	Hosts        []string `json:"hosts"`
}

func (f *apiTaskForm) Overwrite(tpl *models.TaskTpl) {
	if f.Title == "" {
		f.Title = tpl.Title
	}

	if f.Account == "" {
		f.Account = tpl.Account
	}

	if f.BatchStr == "" {
		f.Batch = tpl.Batch
	} else {
		b, e := strconv.ParseInt(f.BatchStr, 10, 64)
		dangerous(e)

		f.Batch = int(b)
	}

	if f.ToleranceStr == "" {
		f.Tolerance = tpl.Tolerance
	} else {
		t, e := strconv.ParseInt(f.ToleranceStr, 10, 64)
		dangerous(e)

		f.Tolerance = int(t)
	}

	if f.TimeoutStr == "" {
		f.Timeout = tpl.Timeout
	} else {
		t, e := strconv.ParseInt(f.TimeoutStr, 10, 64)
		dangerous(e)

		f.Timeout = int(t)
	}

	if f.Pause == "" {
		f.Pause = tpl.Pause
	}

	if f.Script == "" {
		f.Script = tpl.Script
	}

	if f.Args == "" {
		f.Args = tpl.Args
	}

	if f.Hosts == nil || len(f.Hosts) == 0 {
		hosts, err := tpl.Hosts()
		dangerous(err)

		f.Hosts = hosts
	}

	if !(f.Action == "start" || f.Action == "pause") {
		bomb("arg[action] invalid")
	}
}

// 用户拿着自己的token调用API触发任务执行
func taskTplRun(c *gin.Context) {
	var f apiTaskForm
	bind(c, &f)

	user := loginUser(c)
	tpl := TaskTpl(urlParamInt64(c, "id"))

	f.Overwrite(tpl)
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

func accountToOP(account string) string {
	operation := "task_run_use_root_account"
	if account != "root" {
		operation = "task_run_use_gene_account"
	}
	return operation
}
