package sender

import (
	"strconv"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/pkg/ibex"
	"github.com/didi/nightingale/v5/src/pkg/poster"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
)

func SendCallbacks(urls []string, event *models.AlertCurEvent) {
	for _, url := range urls {
		if url == "" {
			continue
		}

		if strings.HasPrefix(url, "${ibex}") {
			if !event.IsRecovered {
				handleIbex(url, event)
			}
			continue
		}

		if !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
			url = "http://" + url
		}

		resp, code, err := poster.PostJSON(url, 5*time.Second, event, 3)
		if err != nil {
			logger.Errorf("event_callback(rule_id=%d url=%s) fail, resp: %s, err: %v, code: %d", event.RuleId, url, string(resp), err, code)
		} else {
			logger.Infof("event_callback(rule_id=%d url=%s) succ, resp: %s, code: %d", event.RuleId, url, string(resp), code)
		}
	}
}

type TaskForm struct {
	Title     string   `json:"title"`
	Account   string   `json:"account"`
	Batch     int      `json:"batch"`
	Tolerance int      `json:"tolerance"`
	Timeout   int      `json:"timeout"`
	Pause     string   `json:"pause"`
	Script    string   `json:"script"`
	Args      string   `json:"args"`
	Action    string   `json:"action"`
	Creator   string   `json:"creator"`
	Hosts     []string `json:"hosts"`
}

type TaskCreateReply struct {
	Err string `json:"err"`
	Dat int64  `json:"dat"` // task.id
}

func handleIbex(url string, event *models.AlertCurEvent) {
	arr := strings.Split(url, "/")

	var idstr string
	var host string

	if len(arr) > 1 {
		idstr = arr[1]
	}

	if len(arr) > 2 {
		host = arr[2]
	}

	id, err := strconv.ParseInt(idstr, 10, 64)
	if err != nil {
		logger.Errorf("event_callback_ibex: failed to parse url: %s", url)
		return
	}

	if host == "" {
		// 用户在callback url中没有传入host，就从event中解析
		host = event.TargetIdent
	}

	if host == "" {
		logger.Error("event_callback_ibex: failed to get host")
		return
	}

	tpl, err := models.TaskTplGet("id = ?", id)
	if err != nil {
		logger.Errorf("event_callback_ibex: failed to get tpl: %v", err)
		return
	}

	if tpl == nil {
		logger.Errorf("event_callback_ibex: no such tpl(%d)", id)
		return
	}

	// check perm
	// tpl.GroupId - host - account 三元组校验权限
	can, err := canDoIbex(tpl.UpdateBy, tpl, host)
	if err != nil {
		logger.Errorf("event_callback_ibex: check perm fail: %v", err)
		return
	}

	if !can {
		logger.Errorf("event_callback_ibex: user(%s) no permission", tpl.UpdateBy)
		return
	}

	// call ibex
	in := TaskForm{
		Title:     tpl.Title + " FH: " + host,
		Account:   tpl.Account,
		Batch:     tpl.Batch,
		Tolerance: tpl.Tolerance,
		Timeout:   tpl.Timeout,
		Pause:     tpl.Pause,
		Script:    tpl.Script,
		Args:      tpl.Args,
		Action:    "start",
		Creator:   tpl.UpdateBy,
		Hosts:     []string{host},
	}

	var res TaskCreateReply
	err = ibex.New(
		config.C.Ibex.Address,
		config.C.Ibex.BasicAuthUser,
		config.C.Ibex.BasicAuthPass,
		config.C.Ibex.Timeout,
	).
		Path("/ibex/v1/tasks").
		In(in).
		Out(&res).
		POST()

	if err != nil {
		logger.Errorf("event_callback_ibex: call ibex fail: %v", err)
		return
	}

	if res.Err != "" {
		logger.Errorf("event_callback_ibex: call ibex response error: %v", res.Err)
		return
	}

	// write db
	record := models.TaskRecord{
		Id:           res.Dat,
		GroupId:      tpl.GroupId,
		IbexAddress:  config.C.Ibex.Address,
		IbexAuthUser: config.C.Ibex.BasicAuthUser,
		IbexAuthPass: config.C.Ibex.BasicAuthPass,
		Title:        in.Title,
		Account:      in.Account,
		Batch:        in.Batch,
		Tolerance:    in.Tolerance,
		Timeout:      in.Timeout,
		Pause:        in.Pause,
		Script:       in.Script,
		Args:         in.Args,
		CreateAt:     time.Now().Unix(),
		CreateBy:     in.Creator,
	}

	if err = record.Add(); err != nil {
		logger.Errorf("event_callback_ibex: persist task_record fail: %v", err)
	}
}

func canDoIbex(username string, tpl *models.TaskTpl, host string) (bool, error) {
	user, err := models.UserGetByUsername(username)
	if err != nil {
		return false, err
	}

	if user != nil && user.IsAdmin() {
		return true, nil
	}

	target, has := memsto.TargetCache.Get(host)
	if !has {
		return false, nil
	}

	return target.GroupId == tpl.GroupId, nil
}
