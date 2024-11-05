// @Author: Ciusyan 6/5/24

package sender

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	imodels "github.com/flashcatcloud/ibex/src/models"
	"github.com/flashcatcloud/ibex/src/storage"

	"github.com/toolkits/pkg/logger"
)

var (
	_ CallBacker = (*IbexCallBacker)(nil)
)

type IbexCallBacker struct {
	targetCache  *memsto.TargetCacheType
	userCache    *memsto.UserCacheType
	taskTplCache *memsto.TaskTplCache
}

func (c *IbexCallBacker) CallBack(ctx CallBackContext) {
	if len(ctx.CallBackURL) == 0 || len(ctx.Events) == 0 {
		return
	}

	event := ctx.Events[0]

	if event.IsRecovered {
		return
	}

	c.handleIbex(ctx.Ctx, ctx.CallBackURL, event)
}

func (c *IbexCallBacker) handleIbex(ctx *ctx.Context, url string, event *models.AlertCurEvent) {
	if imodels.DB() == nil && ctx.IsCenter {
		logger.Warning("event_callback_ibex: db is nil")
		return
	}

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

	CallIbex(ctx, id, host, c.taskTplCache, c.targetCache, c.userCache, event)
}

func CallIbex(ctx *ctx.Context, id int64, host string,
	taskTplCache *memsto.TaskTplCache, targetCache *memsto.TargetCacheType,
	userCache *memsto.UserCacheType, event *models.AlertCurEvent) {
	tpl := taskTplCache.Get(id)
	if tpl == nil {
		logger.Errorf("event_callback_ibex: no such tpl(%d)", id)
		return
	}
	// check perm
	// tpl.GroupId - host - account 三元组校验权限
	can, err := canDoIbex(tpl.UpdateBy, tpl, host, targetCache, userCache)
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

		arr := strings.SplitN(pair, "=", 2)
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
	in := models.TaskForm{
		Title:          tpl.Title + " FH: " + host,
		Account:        tpl.Account,
		Batch:          tpl.Batch,
		Tolerance:      tpl.Tolerance,
		Timeout:        tpl.Timeout,
		Pause:          tpl.Pause,
		Script:         tpl.Script,
		Args:           tpl.Args,
		Stdin:          string(tags),
		Action:         "start",
		Creator:        tpl.UpdateBy,
		Hosts:          []string{host},
		AlertTriggered: true,
	}

	id, err = TaskAdd(in, tpl.UpdateBy, ctx.IsCenter)
	if err != nil {
		logger.Errorf("event_callback_ibex: call ibex fail: %v", err)
		return
	}

	// write db
	record := models.TaskRecord{
		Id:        id,
		EventId:   event.Id,
		GroupId:   tpl.GroupId,
		Title:     in.Title,
		Account:   in.Account,
		Batch:     in.Batch,
		Tolerance: in.Tolerance,
		Timeout:   in.Timeout,
		Pause:     in.Pause,
		Script:    in.Script,
		Args:      in.Args,
		CreateAt:  time.Now().Unix(),
		CreateBy:  in.Creator,
	}

	if err = record.Add(ctx); err != nil {
		logger.Errorf("event_callback_ibex: persist task_record fail: %v", err)
	}
}

func canDoIbex(username string, tpl *models.TaskTpl, host string, targetCache *memsto.TargetCacheType, userCache *memsto.UserCacheType) (bool, error) {
	user := userCache.GetByUsername(username)
	if user != nil && user.IsAdmin() {
		return true, nil
	}

	target, has := targetCache.Get(host)
	if !has {
		return false, nil
	}

	return target.MatchGroupId(tpl.GroupId), nil
}

func TaskAdd(f models.TaskForm, authUser string, isCenter bool) (int64, error) {
	hosts := cleanHosts(f.Hosts)
	if len(hosts) == 0 {
		return 0, fmt.Errorf("arg(hosts) empty")
	}

	taskMeta := &imodels.TaskMeta{
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
	if err != nil {
		return 0, err
	}

	taskMeta.HandleFH(hosts[0])

	// 任务类型分为"告警规则触发"和"n9e center用户下发"两种；
	// 边缘机房"告警规则触发"的任务不需要规划，并且它可能是失联的，无法使用db资源，所以放入redis缓存中，直接下发给agentd执行
	if !isCenter && f.AlertTriggered {
		if err := taskMeta.Create(); err != nil {
			// 当网络不连通时，生成唯一的id，防止边缘机房中不同任务的id相同；
			// 方法是，redis自增id去防止同一个机房的不同n9e edge生成的id相同；
			// 但没法防止不同边缘机房生成同样的id，所以，生成id的数据不会上报存入数据库，只用于闭环执行。
			taskMeta.Id, err = storage.IdGet()
			if err != nil {
				return 0, err
			}
		}

		taskHost := imodels.TaskHost{
			Id:     taskMeta.Id,
			Host:   hosts[0],
			Status: "running",
		}
		if err = taskHost.Create(); err != nil {
			logger.Warningf("task_add_fail: authUser=%s title=%s err=%s", authUser, taskMeta.Title, err.Error())
		}

		// 缓存任务元信息和待下发的任务
		err = taskMeta.Cache(hosts[0])
		if err != nil {
			return 0, err
		}

	} else {
		// 如果是中心机房，还是保持之前的逻辑
		err = taskMeta.Save(hosts, f.Action)
		if err != nil {
			return 0, err
		}
	}

	logger.Infof("task_add_succ: authUser=%s title=%s", authUser, taskMeta.Title)
	return taskMeta.Id, nil
}

func cleanHosts(formHosts []string) []string {
	cnt := len(formHosts)
	arr := make([]string, 0, cnt)
	for i := 0; i < cnt; i++ {
		item := strings.TrimSpace(formHosts[i])
		if item == "" {
			continue
		}

		if strings.HasPrefix(item, "#") {
			continue
		}

		arr = append(arr, item)
	}

	return arr
}
