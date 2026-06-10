package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/toolkits/pkg/logger"
)

type alertMuteResult struct {
	Id       int64  `json:"id"`
	GroupId  int64  `json:"group_id"`
	Cause    string `json:"cause,omitempty"`
	Disabled int    `json:"disabled"`
	Btime    string `json:"btime"`
	Etime    string `json:"etime"`
	CreateBy string `json:"create_by,omitempty"`
}

type alertMuteDetailResult struct {
	Id       int64       `json:"id"`
	GroupId  int64       `json:"group_id"`
	Note     string      `json:"note,omitempty"`
	Cause    string      `json:"cause,omitempty"`
	Cate     string      `json:"cate,omitempty"`
	Tags     interface{} `json:"tags,omitempty"`
	Disabled int         `json:"disabled"`
	Btime    string      `json:"btime"`
	Etime    string      `json:"etime"`
	CreateBy string      `json:"create_by,omitempty"`
	UpdateBy string      `json:"update_by,omitempty"`
}

func init() {
	register(defs.ListAlertMutes, listAlertMutes)
	register(defs.GetAlertMuteDetail, getAlertMuteDetail)
	register(defs.CreateAlertMute, createAlertMute)
}

// createAlertMute 落库一条屏蔽规则。入参 config 是与前端/HTTP API 同构的 AlertMute JSON
// （n9e-alert-mute-copilot skill 文档化了字段形状），直接反序列化进 models.AlertMute，由
// AlertMute.Add 内部做 Verify(etime>btime、标签解析) + FE2DB(datasource/periodic/severities
// 序列化) + 落库。业务组缺参门同 create_dashboard：config 未带 group_id 时回退表单注入的
// busi_group_id，仍缺则弹业务组选择表单。
func createAlertMute(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertMutesAdd); err != nil {
		return "", err
	}

	configJSON := getArgString(args, "config")
	if configJSON == "" {
		return "", fmt.Errorf("config is required: a JSON object describing the mute (cause, tags, btime, etime, ...); load the n9e-alert-mute-copilot skill for the exact shape")
	}

	var mute models.AlertMute
	if err := json.Unmarshal([]byte(configJSON), &mute); err != nil {
		return "", fmt.Errorf("invalid config JSON: %v", err)
	}

	// 业务组缺参门：config 没带 group_id 就回退表单/页面注入的 busi_group_id，仍缺则弹表单。
	groupId := mute.GroupId
	if groupId == 0 {
		groupId = resolveCreationGroupID(args, params)
	}
	if groupId == 0 {
		return "", creationFormInterrupt(deps, user, "n9e-alert-mute-copilot", []string{"busi_group_id"})
	}
	mute.GroupId = groupId

	bg, err := models.BusiGroupGetById(deps.DBCtx, groupId)
	if err != nil {
		return "", fmt.Errorf("failed to get busi group: %v", err)
	}
	if bg == nil {
		return "", fmt.Errorf("busi group not found: id=%d", groupId)
	}
	if err := checkBgRW(deps, user, bg); err != nil {
		return "", err
	}

	// datasource_ids 缺省为 [0]：空数组在引擎里已等价"全部"（MatchMute 见 DatasourceIdsJson
	// 为空即跳过数据源过滤），但 [0] 才是 FE 表示"全部数据源"的标准哨兵——规范化成它，落库后
	// 前端能正确回显"全部"，也避免模型照搬 skill 示例里的 [] 时存成空串。
	if len(mute.DatasourceIdsJson) == 0 {
		mute.DatasourceIdsJson = []int64{0}
	}

	// 时间兜底：让 LLM 不必自己算 Unix 时间戳（btime=now、duration→etime、periodic 缺省一年）。
	if err := fillMuteTime(&mute, args); err != nil {
		return "", err
	}

	// tags / periodic 归一化：接受人话/数组，转成引擎要的 wire 形式，并对非法 func 早报错
	// （引擎对非法 func 不报错却永不匹配，是静默坑）。
	if err := normalizeMuteTags(&mute); err != nil {
		return "", err
	}
	normalizeMutePeriodic(&mute)

	mute.Id = 0 // 防止模型把 id 塞进 config 导致主键冲突
	mute.CreateBy = user.Username
	mute.UpdateBy = user.Username

	if err := mute.Add(deps.DBCtx); err != nil {
		return "", fmt.Errorf("failed to create alert mute: %v", err)
	}

	logger.Infof("create_alert_mute: user=%s, group_id=%d, cause=%s, id=%d", user.Username, groupId, mute.Cause, mute.Id)

	result := map[string]interface{}{
		"id":         mute.Id,
		"group_id":   mute.GroupId,
		"group_name": bg.Name,
		"cause":      mute.Cause,
		"btime":      formatUnixTime(mute.Btime),
		"etime":      formatUnixTime(mute.Etime),
	}
	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}

// fillMuteTime 给屏蔽规则补时间，把"算 Unix 时间戳"这件 LLM 容易错的活搬到服务端：
//   - btime 缺省(0) → 当前时间；
//   - 传了 duration 参数(如 "2h"/"7d"/"1d12h") → etime = btime + duration（覆盖 config 里的 etime）；
//   - 周期屏蔽(type=1)缺 etime → 默认一年：btime/etime 仅用于过 Verify(etime>btime)，
//     周期匹配 IsWithinPeriodicMute 根本不看这俩，给足够大的区间即可；
//   - 固定屏蔽(type=0)既无 duration 又无 etime → 明确报错，而不是落到 Verify 那句晦涩的 etime<=btime。
func fillMuteTime(mute *models.AlertMute, args map[string]interface{}) error {
	if mute.Btime == 0 {
		mute.Btime = time.Now().Unix()
	}

	if durStr := getArgString(args, "duration"); durStr != "" {
		secs, err := parseDurationSeconds(durStr)
		if err != nil {
			return err
		}
		mute.Etime = mute.Btime + secs
	} else if mute.MuteTimeType == models.Periodic && mute.Etime == 0 {
		mute.Etime = mute.Btime + 365*24*3600
	}

	if mute.Etime == 0 {
		return fmt.Errorf("固定时间屏蔽需要结束时间：请用 duration 参数（如 \"2h\"、\"7d\"）或在 config 里给 etime（Unix 秒）")
	}
	return nil
}

var durationSegRe = regexp.MustCompile(`(\d+)(w|d|h|m|s)`)

// parseDurationSeconds 解析 "2h"/"7d"/"30m"/"1d12h"/"1w" 这类时长为秒。
// 在 Go time.ParseDuration(只认到 h)之上补了 d(天)/w(周)；用全量覆盖校验拒绝 "2x" 这类残渣。
func parseDurationSeconds(s string) (int64, error) {
	compact := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if compact == "" {
		return 0, fmt.Errorf("duration 不能为空")
	}
	if leftover := durationSegRe.ReplaceAllString(compact, ""); leftover != "" {
		return 0, fmt.Errorf("无法解析时长 %q（残留 %q）：支持 s/m/h/d/w，如 2h、7d、1d12h", s, leftover)
	}
	unitSec := map[string]int64{"s": 1, "m": 60, "h": 3600, "d": 86400, "w": 604800}
	var total int64
	for _, m := range durationSegRe.FindAllStringSubmatch(compact, -1) {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		total += n * unitSec[m[2]]
	}
	if total <= 0 {
		return 0, fmt.Errorf("时长必须大于 0：%q", s)
	}
	return total, nil
}

var validTagFuncs = map[string]bool{"==": true, "!=": true, "=~": true, "!~": true, "in": true, "not in": true}

// normalizeMuteTags 校验并归一化 tags：func 缺省回退 op；非法 func 早报错；
// in/not in 的 value 接受数组或逗号分隔，统一成空格分隔字符串（FE/引擎约定）。
func normalizeMuteTags(mute *models.AlertMute) error {
	if len([]byte(mute.Tags)) == 0 {
		return nil
	}
	var filters []models.TagFilter
	if err := json.Unmarshal(mute.Tags, &filters); err != nil {
		return fmt.Errorf("invalid tags: %v", err)
	}
	for i := range filters {
		f := &filters[i]
		if f.Func == "" {
			f.Func = f.Op
		}
		if f.Key == "" {
			return fmt.Errorf("tags[%d]: key 不能为空", i)
		}
		if !validTagFuncs[f.Func] {
			return fmt.Errorf("tags[%d]: func %q 非法，必须是 == / != / =~ / !~ / in / not in 之一", i, f.Func)
		}
		f.Op = f.Func // 保持 op 与 func 一致
		if f.Func == "in" || f.Func == "not in" {
			f.Value = normalizeInValue(f.Value)
		}
	}
	b, err := json.Marshal(filters)
	if err != nil {
		return fmt.Errorf("failed to normalize tags: %v", err)
	}
	mute.Tags = ormx.JSONArr(b)
	return nil
}

// normalizeInValue 把 in/not in 的 value 统一成空格分隔字符串：数组→空格拼接；含逗号的串→逗号转空格。
func normalizeInValue(v interface{}) interface{} {
	switch val := v.(type) {
	case []interface{}:
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, fmt.Sprintf("%v", item))
		}
		return strings.Join(parts, " ")
	case string:
		if strings.ContainsRune(val, ',') {
			fields := strings.FieldsFunc(val, func(r rune) bool { return r == ',' || r == ' ' })
			return strings.Join(fields, " ")
		}
		return val
	default:
		return v
	}
}

// weekdayWords 把常见的中英文星期说法映射到引擎要的"空格分隔数字串"(0=周日…6=周六)。
var weekdayWords = map[string]string{
	"每天": "0 1 2 3 4 5 6", "everyday": "0 1 2 3 4 5 6", "daily": "0 1 2 3 4 5 6",
	"工作日": "1 2 3 4 5", "weekday": "1 2 3 4 5", "weekdays": "1 2 3 4 5",
	"周末": "0 6", "weekend": "0 6", "weekends": "0 6",
}

// normalizeMutePeriodic 归一化周期屏蔽的人话写法：
//   - enable_days_of_week 接受 工作日/每天/周末 → 数字串；逗号 → 空格；
//   - enable_stime/etime 写 "全天"/"allday" → 00:00 ~ 23:59。
func normalizeMutePeriodic(mute *models.AlertMute) {
	for i := range mute.PeriodicMutesJson {
		p := &mute.PeriodicMutesJson[i]

		dow := strings.TrimSpace(p.EnableDaysOfWeek)
		if mapped, ok := weekdayWords[strings.ToLower(dow)]; ok {
			p.EnableDaysOfWeek = mapped
		} else if strings.ContainsRune(dow, ',') {
			fields := strings.FieldsFunc(dow, func(r rune) bool { return r == ',' || r == ' ' })
			p.EnableDaysOfWeek = strings.Join(fields, " ")
		}

		if isAllDayWord(p.EnableStime) || isAllDayWord(p.EnableEtime) {
			p.EnableStime = "00:00"
			p.EnableEtime = "23:59"
		}
	}
}

func isAllDayWord(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "全天", "allday", "all day", "24h":
		return true
	}
	return false
}

func listAlertMutes(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertMutes); err != nil {
		return "", err
	}

	bgids, isAdmin, err := getUserBgids(deps, user)
	if err != nil {
		return "", err
	}

	query := getArgString(args, "query")
	limit := getArgInt(args, "limit", 50)
	if limit > 200 {
		limit = 200
	}

	var mutes []models.AlertMute
	if isAdmin {
		mutes, err = models.AlertMuteGetsByBGIds(deps.DBCtx, nil)
	} else {
		if len(bgids) == 0 {
			return marshalList(0, []alertMuteResult{}), nil
		}
		mutes, err = models.AlertMuteGetsByBGIds(deps.DBCtx, bgids)
	}
	if err != nil {
		return "", fmt.Errorf("failed to query alert mutes: %v", err)
	}

	results := make([]alertMuteResult, 0)
	for _, m := range mutes {
		if query != "" && !containsIgnoreCase(m.Cause, query) {
			continue
		}
		results = append(results, alertMuteResult{
			Id:       m.Id,
			GroupId:  m.GroupId,
			Cause:    m.Cause,
			Disabled: m.Disabled,
			Btime:    formatUnixTime(m.Btime),
			Etime:    formatUnixTime(m.Etime),
			CreateBy: m.CreateBy,
		})
		if len(results) >= limit {
			break
		}
	}

	logger.Debugf("list_alert_mutes: user_id=%d, found %d mutes", user.Id, len(results))
	return marshalList(len(results), results), nil
}

func getAlertMuteDetail(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermAlertMutes); err != nil {
		return "", err
	}

	id := getArgInt64(args, "id")
	if id == 0 {
		return "", fmt.Errorf("id is required")
	}

	mute, err := models.AlertMuteGetById(deps.DBCtx, id)
	if err != nil {
		return "", fmt.Errorf("failed to get alert mute: %v", err)
	}
	if mute == nil {
		return fmt.Sprintf(`{"error":"alert mute not found: id=%d"}`, id), nil
	}

	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
		if err != nil {
			return "", err
		}
		if !int64SliceContains(bgids, mute.GroupId) {
			return "", fmt.Errorf("forbidden: no access to this alert mute")
		}
	}

	result := alertMuteDetailResult{
		Id:       mute.Id,
		GroupId:  mute.GroupId,
		Note:     mute.Note,
		Cause:    mute.Cause,
		Cate:     mute.Cate,
		Tags:     mute.Tags,
		Disabled: mute.Disabled,
		Btime:    formatUnixTime(mute.Btime),
		Etime:    formatUnixTime(mute.Etime),
		CreateBy: mute.CreateBy,
		UpdateBy: mute.UpdateBy,
	}

	bytes, _ := json.Marshal(result)
	return string(bytes), nil
}
