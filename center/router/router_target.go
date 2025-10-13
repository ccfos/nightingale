package router

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/strx"
	"github.com/ccfos/nightingale/v6/pushgw/idents"
	"github.com/ccfos/nightingale/v6/storage"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

type TargetQuery struct {
	Filters []models.HostQuery `json:"queries"`
	P       int                `json:"p"`
	Limit   int                `json:"limit"`
}

func (rt *Router) targetGetsByHostFilter(c *gin.Context) {
	var f TargetQuery
	ginx.BindJSON(c, &f)

	query := models.GetHostsQuery(f.Filters)

	hosts, err := models.TargetGetsByFilter(rt.Ctx, query, f.Limit, (f.P-1)*f.Limit)
	ginx.Dangerous(err)

	total, err := models.TargetCountByFilter(rt.Ctx, query)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(gin.H{
		"list":  hosts,
		"total": total,
	}, nil)
}

func (rt *Router) targetGets(c *gin.Context) {
	bgids := strx.IdsInt64ForAPI(ginx.QueryStr(c, "gids", ""), ",")
	query := ginx.QueryStr(c, "query", "")
	limit := ginx.QueryInt(c, "limit", 30)
	downtime := ginx.QueryInt64(c, "downtime", 0)
	dsIds := queryDatasourceIds(c)

	order := ginx.QueryStr(c, "order", "ident")
	desc := ginx.QueryBool(c, "desc", false)

	hosts := queryStrListField(c, "hosts", ",", " ", "\n")

	bgids = rt.resolveTargetBgids(c, bgids)

	options := []models.BuildTargetWhereOption{
		models.BuildTargetWhereWithBgids(bgids),
		models.BuildTargetWhereWithDsIds(dsIds),
		models.BuildTargetWhereWithQuery(query),
		models.BuildTargetWhereWithDowntime(downtime),
		models.BuildTargetWhereWithHosts(hosts),
	}
	total, err := models.TargetTotal(rt.Ctx, options...)
	ginx.Dangerous(err)

	list, err := models.TargetGets(rt.Ctx, limit,
		ginx.Offset(c, limit), order, desc, options...)
	ginx.Dangerous(err)

	ginx.Dangerous(rt.populateTargetDetails(list))

	ginx.NewRender(c).Data(gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

func (rt *Router) targetExport(c *gin.Context) {
	bgids := strx.IdsInt64ForAPI(ginx.QueryStr(c, "gids", ""), ",")
	query := ginx.QueryStr(c, "query", "")
	downtime := ginx.QueryInt64(c, "downtime", 0)
	dsIds := queryDatasourceIds(c)
	order := ginx.QueryStr(c, "order", "ident")
	desc := ginx.QueryBool(c, "desc", false)
	hosts := queryStrListField(c, "hosts", ",", " ", "\n")

	bgids = rt.resolveTargetBgids(c, bgids)

	options := []models.BuildTargetWhereOption{
		models.BuildTargetWhereWithBgids(bgids),
		models.BuildTargetWhereWithDsIds(dsIds),
		models.BuildTargetWhereWithQuery(query),
		models.BuildTargetWhereWithDowntime(downtime),
		models.BuildTargetWhereWithHosts(hosts),
	}

	list, err := models.TargetGetsAllByOptions(rt.Ctx, order, desc, options...)
	ginx.Dangerous(err)

	ginx.Dangerous(rt.populateTargetDetails(list))

	filename := fmt.Sprintf("targets_%s.csv", time.Now().Format("20060102150405"))
	c.Writer.Header().Set("Content-Type", "text/csv; charset=utf-8")
	c.Writer.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	c.Writer.WriteHeaderNow()

	if _, err := c.Writer.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		logger.Warningf("write utf-8 bom failed: %v", err)
	}

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	header := []string{
		"ident", "host_ip", "note", "business_groups", "tags", "host_tags",
		"engine_name", "agent_version", "os", "cpu_num", "cpu_util", "mem_util",
		"remote_addr", "update_at",
	}
	if err := writer.Write(header); err != nil {
		logger.Warningf("write csv header failed: %v", err)
		return
	}

	const timeLayout = "2006-01-02 15:04:05"

	for _, target := range list {
		updateAt := ""
		if target.UpdateAt > 0 {
			updateAt = time.Unix(target.UpdateAt, 0).Format(timeLayout)
		}

		cpuNum := ""
		if target.CpuNum >= 0 {
			cpuNum = strconv.Itoa(target.CpuNum)
		}

		cpuUtil := ""
		if target.CpuUtil != 0 {
			cpuUtil = fmt.Sprintf("%.2f", target.CpuUtil)
		}

		memUtil := ""
		if target.MemUtil != 0 {
			memUtil = fmt.Sprintf("%.2f", target.MemUtil)
		}

		row := []string{
			target.Ident,
			target.HostIp,
			target.Note,
			strings.Join(target.GroupNames, ";"),
			strings.Join(target.TagsJSON, " "),
			strings.Join(target.HostTags, " "),
			target.EngineName,
			target.AgentVersion,
			target.OS,
			cpuNum,
			cpuUtil,
			memUtil,
			target.RemoteAddr,
			updateAt,
		}

		if err := writer.Write(row); err != nil {
			logger.Warningf("write csv row failed: %v", err)
			return
		}
	}

	if err := writer.Error(); err != nil {
		logger.Warningf("flush csv writer failed: %v", err)
	}
}

func (rt *Router) targetExtendInfoByIdent(c *gin.Context) {
	ident := ginx.QueryStr(c, "ident", "")
	key := models.WrapExtendIdent(ident)
	vals := storage.MGet(context.Background(), rt.Redis, []string{key})
	if len(vals) > 0 {
		extInfo := string(vals[0])
		if extInfo == "null" {
			extInfo = ""
		}
		ginx.NewRender(c).Data(gin.H{
			"extend_info": extInfo,
			"ident":       ident,
		}, nil)
		return
	}
	ginx.NewRender(c).Data(gin.H{
		"extend_info": "",
		"ident":       ident,
	}, nil)
}

func (rt *Router) targetGetsByService(c *gin.Context) {
	lst, err := models.TargetGetsAll(rt.Ctx)
	ginx.NewRender(c).Data(lst, err)
}

func (rt *Router) targetGetTags(c *gin.Context) {
	idents := ginx.QueryStr(c, "idents", "")
	idents = strings.ReplaceAll(idents, ",", " ")
	ignoreHostTag := ginx.QueryBool(c, "ignore_host_tag", false)
	lst, err := models.TargetGetTags(rt.Ctx, strings.Fields(idents), ignoreHostTag, "")
	ginx.NewRender(c).Data(lst, err)
}

type targetTagsForm struct {
	Idents  []string `json:"idents" binding:"required_without=HostIps"`
	HostIps []string `json:"host_ips" binding:"required_without=Idents"`
	Tags    []string `json:"tags" binding:"required"`
}

func (rt *Router) targetBindTagsByFE(c *gin.Context) {
	var f targetTagsForm
	var err error
	var failedResults = make(map[string]string)
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 && len(f.HostIps) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents or host_ips must be provided")
	}
	// Acquire idents by idents and hostIps
	failedResults, f.Idents, err = models.TargetsGetIdentsByIdentsAndHostIps(rt.Ctx, f.Idents, f.HostIps)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	rt.checkTargetPerm(c, f.Idents)

	ginx.NewRender(c).Data(rt.targetBindTags(f, failedResults))
}

func (rt *Router) targetBindTagsByService(c *gin.Context) {
	var f targetTagsForm
	var err error
	var failedResults = make(map[string]string)
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 && len(f.HostIps) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents or host_ips must be provided")
	}
	// Acquire idents by idents and hostIps
	failedResults, f.Idents, err = models.TargetsGetIdentsByIdentsAndHostIps(rt.Ctx, f.Idents, f.HostIps)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	ginx.NewRender(c).Data(rt.targetBindTags(f, failedResults))
}

func (rt *Router) targetBindTags(f targetTagsForm, failedIdents map[string]string) (map[string]string, error) {
	// 1. Check tags
	if err := rt.validateTags(f.Tags); err != nil {
		return nil, err
	}

	// 2. Acquire targets by idents
	targets, err := models.TargetsGetByIdents(rt.Ctx, f.Idents)
	if err != nil {
		return nil, err
	}

	// 3. Add tags to targets
	for _, target := range targets {
		if err = rt.addTagsToTarget(target, f.Tags); err != nil {
			failedIdents[target.Ident] = err.Error()
		}
	}

	return failedIdents, nil
}

func (rt *Router) populateTargetDetails(list []*models.Target) error {
	if len(list) == 0 {
		return nil
	}

	tgs, err := models.TargetBusiGroupsGetAll(rt.Ctx)
	if err != nil {
		return err
	}

	now := time.Now()
	cache := make(map[int64]*models.BusiGroup)
	keys := make([]string, 0, len(list))

	for _, target := range list {
		target.GroupIds = tgs[target.Ident]

		if now.Unix()-target.UpdateAt < 60 {
			target.TargetUp = 2
		} else if now.Unix()-target.UpdateAt < 180 {
			target.TargetUp = 1
		}

		if err := target.FillGroup(rt.Ctx, cache); err != nil {
			return err
		}

		if len(target.GroupObjs) > 0 {
			names := make([]string, 0, len(target.GroupObjs))
			for _, group := range target.GroupObjs {
				if group != nil {
					names = append(names, group.Name)
				}
			}
			target.GroupNames = names
		} else {
			target.GroupNames = nil
		}

		keys = append(keys, models.WrapIdent(target.Ident))
	}

	if len(keys) == 0 {
		return nil
	}

	metaMap := make(map[string]*models.HostMeta)
	vals := storage.MGet(context.Background(), rt.Redis, keys)
	for _, value := range vals {
		if value == nil {
			continue
		}
		var meta models.HostMeta
		if err := json.Unmarshal(value, &meta); err != nil {
			logger.Warningf("unmarshal %v host meta failed: %v", value, err)
			continue
		}
		metaMap[meta.Hostname] = &meta
	}

	for _, target := range list {
		if meta, ok := metaMap[target.Ident]; ok {
			target.FillMeta(meta)
		} else {
			// 未上报过元数据的主机，cpuNum默认为-1, 用于前端展示 unknown
			target.CpuNum = -1
		}
	}

	return nil
}

func (rt *Router) resolveTargetBgids(c *gin.Context, bgids []int64) []int64 {
	if len(bgids) > 0 {
		// 如果用户当前查看的是未归组机器，会传入 bgids = [0]，此时是不需要校验的，故而排除这种情况
		if !(len(bgids) == 1 && bgids[0] == 0) {
			for _, gid := range bgids {
				rt.bgroCheck(c, gid)
			}
		}
		return bgids
	}

	user := c.MustGet("user").(*models.User)
	if !user.IsAdmin() {
		ids, err := models.MyBusiGroupIds(rt.Ctx, user.Id)
		ginx.Dangerous(err)
		bgids = append(ids, 0)
	}

	return bgids
}

func (rt *Router) validateTags(tags []string) error {
	for _, tag := range tags {
		arr := strings.Split(tag, "=")
		if len(arr) != 2 {
			return fmt.Errorf("invalid tag format: %s (expected format: key=value)", tag)
		}

		key, value := strings.TrimSpace(arr[0]), strings.TrimSpace(arr[1])
		if key == "" {
			return fmt.Errorf("invalid tag: key is empty in tag %s", tag)
		}
		if value == "" {
			return fmt.Errorf("invalid tag: value is empty in tag %s", tag)
		}

		if strings.Contains(key, ".") {
			return fmt.Errorf("invalid tag key: %s (key cannot contain '.')", key)
		}

		if strings.Contains(key, "-") {
			return fmt.Errorf("invalid tag key: %s (key cannot contain '-')", key)
		}

		if !model.LabelNameRE.MatchString(key) {
			return fmt.Errorf("invalid tag key: %s "+
				"(key must start with a letter or underscore, followed by letters, digits, or underscores)", key)
		}
	}

	return nil
}

func (rt *Router) addTagsToTarget(target *models.Target, tags []string) error {
	for _, tag := range tags {
		tagKey := strings.Split(tag, "=")[0]
		if _, exist := target.TagsMap[tagKey]; exist {
			return fmt.Errorf("duplicate tagkey(%s)", tagKey)
		}
	}

	return target.AddTags(rt.Ctx, tags)
}

func (rt *Router) targetUnbindTagsByFE(c *gin.Context) {
	var f targetTagsForm
	var err error
	var failedResults = make(map[string]string)
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 && len(f.HostIps) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents or host_ips must be provided")
	}
	// Acquire idents by idents and hostIps
	failedResults, f.Idents, err = models.TargetsGetIdentsByIdentsAndHostIps(rt.Ctx, f.Idents, f.HostIps)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	rt.checkTargetPerm(c, f.Idents)

	ginx.NewRender(c).Data(rt.targetUnbindTags(f, failedResults))
}

func (rt *Router) targetUnbindTagsByService(c *gin.Context) {
	var f targetTagsForm
	var err error
	var failedResults = make(map[string]string)
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 && len(f.HostIps) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents or host_ips must be provided")
	}
	// Acquire idents by idents and hostIps
	failedResults, f.Idents, err = models.TargetsGetIdentsByIdentsAndHostIps(rt.Ctx, f.Idents, f.HostIps)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	ginx.NewRender(c).Data(rt.targetUnbindTags(f, failedResults))
}

func (rt *Router) targetUnbindTags(f targetTagsForm, failedIdents map[string]string) (map[string]string, error) {
	// 1. Acquire targets by idents
	targets, err := models.TargetsGetByIdents(rt.Ctx, f.Idents)
	if err != nil {
		return nil, err
	}

	// 2. Remove tags from targets
	for _, target := range targets {
		err = target.DelTags(rt.Ctx, f.Tags)
		if err != nil {
			failedIdents[target.Ident] = err.Error()
			continue
		}
	}

	return failedIdents, nil
}

type targetNoteForm struct {
	Idents  []string `json:"idents" binding:"required_without=HostIps"`
	HostIps []string `json:"host_ips" binding:"required_without=Idents"`
	Note    string   `json:"note"`
}

func (rt *Router) targetUpdateNote(c *gin.Context) {
	var f targetNoteForm
	var err error
	var failedResults = make(map[string]string)
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 && len(f.HostIps) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents or host_ips must be provided")
	}

	// Acquire idents by idents and hostIps
	failedResults, f.Idents, err = models.TargetsGetIdentsByIdentsAndHostIps(rt.Ctx, f.Idents, f.HostIps)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	rt.checkTargetPerm(c, f.Idents)

	ginx.NewRender(c).Data(failedResults, models.TargetUpdateNote(rt.Ctx, f.Idents, f.Note))
}

func (rt *Router) targetUpdateNoteByService(c *gin.Context) {
	var f targetNoteForm
	var err error
	var failedResults = make(map[string]string)
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 && len(f.HostIps) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents or host_ips must be provided")
	}

	// Acquire idents by idents and hostIps
	failedResults, f.Idents, err = models.TargetsGetIdentsByIdentsAndHostIps(rt.Ctx, f.Idents, f.HostIps)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	ginx.NewRender(c).Data(failedResults, models.TargetUpdateNote(rt.Ctx, f.Idents, f.Note))
}

type targetBgidForm struct {
	Idents  []string `json:"idents" binding:"required_without=HostIps"`
	HostIps []string `json:"host_ips" binding:"required_without=Idents"`
	Bgid    int64    `json:"bgid"`
}

type targetBgidsForm struct {
	Idents  []string `json:"idents" binding:"required_without=HostIps"`
	HostIps []string `json:"host_ips" binding:"required_without=Idents"`
	Bgids   []int64  `json:"bgids"`
	Tags    []string `json:"tags"`
	Action  string   `json:"action"` // add del reset
}

func haveNeverGroupedIdent(ctx *ctx.Context, idents []string) (bool, error) {
	for _, ident := range idents {
		bgids, err := models.TargetGroupIdsGetByIdent(ctx, ident)
		if err != nil {
			return false, err
		}

		if len(bgids) <= 0 {
			return true, nil
		}
	}

	return false, nil
}

func (rt *Router) targetBindBgids(c *gin.Context) {
	var f targetBgidsForm
	var err error
	var failedResults = make(map[string]string)
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 && len(f.HostIps) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents or host_ips must be provided")
	}

	// Acquire idents by idents and hostIps
	failedResults, f.Idents, err = models.TargetsGetIdentsByIdentsAndHostIps(rt.Ctx, f.Idents, f.HostIps)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	user := c.MustGet("user").(*models.User)
	if !user.IsAdmin() {
		// 普通用户，检查用户是否有权限操作所有请求的业务组
		existing, _, err := models.SeparateTargetIdents(rt.Ctx, f.Idents)
		ginx.Dangerous(err)
		rt.checkTargetPerm(c, existing)

		var groupIds []int64
		if f.Action == "reset" {
			// 如果是复写，则需要检查用户是否有权限操作机器之前的业务组
			bgids, err := models.TargetGroupIdsGetByIdents(rt.Ctx, f.Idents)
			ginx.Dangerous(err)

			groupIds = append(groupIds, bgids...)
		}
		groupIds = append(groupIds, f.Bgids...)

		for _, bgid := range groupIds {
			bg := BusiGroup(rt.Ctx, bgid)
			can, err := user.CanDoBusiGroup(rt.Ctx, bg, "rw")
			ginx.Dangerous(err)

			if !can {
				ginx.Bomb(http.StatusForbidden, "forbidden")
			}
		}
		isNeverGrouped, checkErr := haveNeverGroupedIdent(rt.Ctx, f.Idents)
		ginx.Dangerous(checkErr)

		if isNeverGrouped {
			can, err := user.CheckPerm(rt.Ctx, "/targets/bind")
			ginx.Dangerous(err)
			if !can {
				ginx.Bomb(http.StatusForbidden, "forbidden")
			}
		}
	}

	switch f.Action {
	case "add":
		ginx.NewRender(c).Data(failedResults, models.TargetBindBgids(rt.Ctx, f.Idents, f.Bgids, f.Tags))
	case "del":
		ginx.NewRender(c).Data(failedResults, models.TargetUnbindBgids(rt.Ctx, f.Idents, f.Bgids))
	case "reset":
		ginx.NewRender(c).Data(failedResults, models.TargetOverrideBgids(rt.Ctx, f.Idents, f.Bgids, f.Tags))
	default:
		ginx.Bomb(http.StatusBadRequest, "invalid action")
	}
}

func (rt *Router) targetUpdateBgidByService(c *gin.Context) {
	var f targetBgidForm
	var err error
	var failedResults = make(map[string]string)
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 && len(f.HostIps) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents or host_ips must be provided")
	}

	// Acquire idents by idents and hostIps
	failedResults, f.Idents, err = models.TargetsGetIdentsByIdentsAndHostIps(rt.Ctx, f.Idents, f.HostIps)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	ginx.NewRender(c).Data(failedResults, models.TargetOverrideBgids(rt.Ctx, f.Idents, []int64{f.Bgid}, nil))
}

type identsForm struct {
	Idents  []string `json:"idents" binding:"required_without=HostIps"`
	HostIps []string `json:"host_ips" binding:"required_without=Idents"`
}

func (rt *Router) targetDel(c *gin.Context) {
	var f identsForm
	var err error
	var failedResults = make(map[string]string)
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 && len(f.HostIps) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents or host_ips must be provided")
	}

	// Acquire idents by idents and hostIps
	failedResults, f.Idents, err = models.TargetsGetIdentsByIdentsAndHostIps(rt.Ctx, f.Idents, f.HostIps)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	ginx.NewRender(c).Data(failedResults, models.TargetDel(rt.Ctx, f.Idents, rt.TargetDeleteHook))
}

func (rt *Router) targetDelByService(c *gin.Context) {
	var f identsForm
	var err error
	var failedResults = make(map[string]string)
	ginx.BindJSON(c, &f)

	if len(f.Idents) == 0 && len(f.HostIps) == 0 {
		ginx.Bomb(http.StatusBadRequest, "idents or host_ips must be provided")
	}

	// Acquire idents by idents and hostIps
	failedResults, f.Idents, err = models.TargetsGetIdentsByIdentsAndHostIps(rt.Ctx, f.Idents, f.HostIps)
	if err != nil {
		ginx.Bomb(http.StatusBadRequest, err.Error())
	}

	ginx.NewRender(c).Data(failedResults, models.TargetDel(rt.Ctx, f.Idents, rt.TargetDeleteHook))
}

func (rt *Router) checkTargetPerm(c *gin.Context, idents []string) {
	user := c.MustGet("user").(*models.User)
	nopri, err := user.NopriIdents(rt.Ctx, idents)
	ginx.Dangerous(err)

	if len(nopri) > 0 {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}
}

func (rt *Router) targetsOfAlertRule(c *gin.Context) {
	engineName := ginx.QueryStr(c, "engine_name", "")
	m, err := models.GetTargetsOfHostAlertRule(rt.Ctx, engineName)
	ret := make(map[string]map[int64][]string)
	for en, v := range m {
		if en != engineName {
			continue
		}

		ret[en] = make(map[int64][]string)
		for rid, idents := range v {
			ret[en][rid] = idents
		}
	}

	ginx.NewRender(c).Data(ret, err)
}

func (rt *Router) checkTargetsExistByIndent(idents []string) {
	notExists, err := models.TargetNoExistIdents(rt.Ctx, idents)
	ginx.Dangerous(err)

	if len(notExists) > 0 {
		ginx.Bomb(http.StatusBadRequest, "targets not exist: %s", strings.Join(notExists, ", "))
	}
}

func (rt *Router) targetsOfHostQuery(c *gin.Context) {
	var queries []models.HostQuery
	ginx.BindJSON(c, &queries)

	hostsQuery := models.GetHostsQuery(queries)
	session := models.TargetFilterQueryBuild(rt.Ctx, hostsQuery, 0, 0)
	var lst []*models.Target
	err := session.Find(&lst).Error
	if err != nil {
		ginx.Bomb(http.StatusInternalServerError, err.Error())
	}

	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) targetUpdate(c *gin.Context) {
	var f idents.TargetUpdate
	ginx.BindJSON(c, &f)

	ginx.NewRender(c).Message(rt.IdentSet.UpdateTargets(f.Lst, f.Now))
}
