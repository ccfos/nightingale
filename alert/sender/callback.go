package sender

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/alert/aconf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ibex"
	"github.com/ccfos/nightingale/v6/pkg/poster"

	"github.com/toolkits/pkg/logger"
)

func SendCallbacks(ctx *ctx.Context, urls []string, event *models.AlertCurEvent, targetCache *memsto.TargetCacheType, userCache *memsto.UserCacheType, ibexConf aconf.Ibex) {
	for _, url := range urls {
		if url == "" {
			continue
		}

		if strings.HasPrefix(url, "${ibex}") {
			if !event.IsRecovered {
				handleIbex(ctx, url, event, targetCache, userCache, ibexConf)
			}
			continue
		}

		if !(strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://")) {
			url = "http://" + url
		}

		resp, code, err := poster.PostJSON(url, 5*time.Second, event, 3)
		if err != nil {
			logger.Errorf("event_callback_fail(rule_id=%d url=%s), resp: %s, err: %v, code: %d", event.RuleId, url, string(resp), err, code)
		} else {
			logger.Infof("event_callback_succ(rule_id=%d url=%s), resp: %s, code: %d", event.RuleId, url, string(resp), code)
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
	Stdin     string   `json:"stdin"`
	Action    string   `json:"action"`
	Creator   string   `json:"creator"`
	Hosts     []string `json:"hosts"`
}

type TaskCreateReply struct {
	Err string `json:"err"`
	Dat int64  `json:"dat"` // task.id
}

func handleIbex(ctx *ctx.Context, url string, event *models.AlertCurEvent, targetCache *memsto.TargetCacheType, userCache *memsto.UserCacheType, ibexConf aconf.Ibex) {
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

	tpl, err := models.TaskTplGet(ctx, "id = ?", id)
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
	can, err := canDoIbex(ctx, tpl.UpdateBy, tpl, host, targetCache, userCache)
	if err != nil {
		logger.Errorf("event_callback_ibex: check perm fail: %v", err)
		return
	}

	if !can {
		logger.Errorf("event_callback_ibex: user(%s) no permission", tpl.UpdateBy)
		return
	}

	tagsMap := make(map[string]string)
	for i := 0; i < len(event.TagsJSON); i++ {
		pair := strings.TrimSpace(event.TagsJSON[i])
		if pair == "" {
			continue
		}

		arr := strings.Split(pair, "=")
		if len(arr) != 2 {
			continue
		}

		tagsMap[arr[0]] = arr[1]
	}
	// 附加告警级别  告警触发值标签
	tagsMap["alert_severity"] = strconv.Itoa(event.Severity)
	tagsMap["alert_trigger_value"] = event.TriggerValue

	tags, err := json.Marshal(tagsMap)
	if err != nil {
		logger.Errorf("event_callback_ibex: failed to marshal tags to json: %v", tagsMap)
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
		Stdin:     string(tags),
		Action:    "start",
		Creator:   tpl.UpdateBy,
		Hosts:     []string{host},
	}

	var res TaskCreateReply
	err = ibex.New(
		ibexConf.Address,
		ibexConf.BasicAuthUser,
		ibexConf.BasicAuthPass,
		ibexConf.Timeout,
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
		EventId:      event.Id,
		GroupId:      tpl.GroupId,
		IbexAddress:  ibexConf.Address,
		IbexAuthUser: ibexConf.BasicAuthUser,
		IbexAuthPass: ibexConf.BasicAuthPass,
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

	if err = record.Add(ctx); err != nil {
		logger.Errorf("event_callback_ibex: persist task_record fail: %v", err)
	}
}

func canDoIbex(ctx *ctx.Context, username string, tpl *models.TaskTpl, host string, targetCache *memsto.TargetCacheType, userCache *memsto.UserCacheType) (bool, error) {
	user := userCache.GetByUsername(username)
	if user != nil && user.IsAdmin() {
		return true, nil
	}

	target, has := targetCache.Get(host)
	if !has {
		return false, nil
	}

	return target.GroupId == tpl.GroupId, nil
}
