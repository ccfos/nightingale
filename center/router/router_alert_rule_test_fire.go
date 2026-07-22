package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/alert/dispatch"
	"github.com/ccfos/nightingale/v6/alert/mute"
	"github.com/ccfos/nightingale/v6/alert/pipeline/engine"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/toolkits/pkg/logger"
)

// 告警规则「模拟触发」：在告警条件未满足时，从规则配置合成一条测试事件，
// 同步走完 生效检查 → 事件处理 pipeline → 屏蔽匹配 → 通知匹配与真实发送，
// 返回逐段链路报告。测试事件不入库、不进真实消费队列、不触发回调/自愈/全局 webhook。
// skip_send=true 时为干跑：不执行任何有外部副作用的环节（pipeline 节点、通知发送）。

type TestFireSample struct {
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
	Query  string            `json:"query"`
}

type AlertRuleTestFireForm struct {
	EventType string           `json:"event_type"` // trigger | recover
	Severity  int              `json:"severity"`   // 1 | 2 | 3
	Sample    *TestFireSample  `json:"sample"`     // 可选：前端从查询预览选中的真实序列
	SkipSend  bool             `json:"skip_send"`  // true 时全链路干跑，不执行任何有外部副作用的环节
	Config    models.AlertRule `json:"config" binding:"required"`
}

const (
	testFireStagePass = "pass"
	testFireStageWarn = "warn"
	testFireStageFail = "fail"
	testFireStageSkip = "skip"
)

type testFireStage struct {
	Stage  string `json:"stage"`
	Status string `json:"status"` // pass | warn | fail | skip
	Data   gin.H  `json:"data"`
}

// 同一 用户 + 业务组 的触发间隔下限，防止误连点骚扰真实接收人。
// key 只用可信来源（me.Id + URL 上的 bgid），不含请求体字段，避免被换 config.id 绕过。
const testFireMinInterval = 10 * time.Second

var (
	testFireMu      sync.Mutex
	testFireLastRun = map[string]time.Time{}
)

// testFireAllow 原子判定是否允许本次触发，并顺带清理过期条目（键有界，无需后台 GC）
func testFireAllow(key string) bool {
	testFireMu.Lock()
	defer testFireMu.Unlock()
	now := time.Now()
	for k, ts := range testFireLastRun {
		if now.Sub(ts) >= testFireMinInterval {
			delete(testFireLastRun, k)
		}
	}
	if ts, ok := testFireLastRun[key]; ok && now.Sub(ts) < testFireMinInterval {
		return false
	}
	testFireLastRun[key] = now
	return true
}

func (rt *Router) alertRuleTestFire(c *gin.Context) {
	var f AlertRuleTestFireForm
	ginx.BindJSON(c, &f)

	bgid := ginx.UrlParamInt64(c, "id")
	me := c.MustGet("user").(*models.User)
	username := c.MustGet("username").(string)

	if f.EventType == "" {
		f.EventType = "trigger"
	}
	if f.EventType != "trigger" && f.EventType != "recover" {
		ginx.Bomb(http.StatusBadRequest, "event_type must be trigger or recover")
	}
	if f.Severity == 0 {
		f.Severity = 2
	}
	if f.Severity < 1 || f.Severity > 3 {
		ginx.Bomb(http.StatusBadRequest, "severity must be 1, 2 or 3")
	}

	if !testFireAllow(fmt.Sprintf("%d:%d", me.Id, bgid)) {
		ginx.Bomb(http.StatusTooManyRequests, "test fire too frequently, please retry later")
	}

	// 前端 payload 的 rule_config / callbacks / append_tags / annotations 绑在 gorm:"-" 影子字段，
	// FE2DB 之后才能像 DB 加载的规则一样使用
	ginx.Dangerous(f.Config.FE2DB())
	cfg := &f.Config
	cfg.GroupId = bgid

	// 去重并限量：请求体里重复的 notify_rule_id / pipeline_id 会在一次已通过限流的请求里
	// 对同一批接收人放大发送、重复执行同一条流水线，这里保序去重并设上限
	cfg.NotifyRuleIds = dedupInt64(cfg.NotifyRuleIds)
	cfg.PipelineConfigs = dedupPipelineConfigs(cfg.PipelineConfigs)
	if len(cfg.NotifyRuleIds) > testFireMaxNotifyRules {
		ginx.Bomb(http.StatusBadRequest, "too many notify rules, at most %d", testFireMaxNotifyRules)
	}
	if len(cfg.PipelineConfigs) > testFireMaxPipelines {
		ginx.Bomb(http.StatusBadRequest, "too many pipelines, at most %d", testFireMaxPipelines)
	}

	// 越权校验前置：cfg.NotifyRuleIds / PipelineConfigs 完全来自请求体，可指向任意团队的资源。
	// 未授权直接 403，绝不进入合成/执行/发送环节，避免向他人接收人发消息或触发他人流水线。
	isAdmin := me.IsAdmin()
	var myUserGroupIds map[int64]struct{}
	if !isAdmin && len(cfg.NotifyRuleIds) > 0 {
		ids, err := models.MyGroupIdsMap(rt.Ctx, me.Id)
		ginx.Dangerous(err)
		myUserGroupIds = ids
	}
	notifyRuleMap := rt.authorizeNotifyRules(cfg, myUserGroupIds, isAdmin)
	pipelineMap := rt.authorizePipelines(cfg, bgid, isAdmin, me)

	stages := make([]testFireStage, 0, 6)

	// ---- Stage 1: 合成事件 ----
	event, synthStage := rt.synthesizeTestEvent(cfg, &f, bgid)
	stages = append(stages, synthStage)

	// ---- Stage 2: 生效检查 ----
	disabled := cfg.Disabled == 1
	inTimeSpan := !mute.TimeSpanMuteStrategy(cfg, event)
	bgMatch := !mute.BgNotMatchMuteStrategy(cfg, event, rt.TargetCache)
	effectiveStatus := testFireStagePass
	if disabled || !inTimeSpan || !bgMatch {
		effectiveStatus = testFireStageWarn
	}
	stages = append(stages, testFireStage{
		Stage:  "effective",
		Status: effectiveStatus,
		Data: gin.H{
			"disabled":     disabled,
			"in_time_span": inTimeSpan,
			"bg_match":     bgMatch,
		},
	})

	// ---- Stage 3: 事件处理 pipeline ----
	pipelineStage, processedEvent, eventDropped := rt.runTestFirePipelines(cfg, event, username, f.SkipSend, pipelineMap)
	stages = append(stages, pipelineStage)
	if processedEvent != nil {
		event = processedEvent
	}

	if eventDropped {
		// 真实链路中被 pipeline 丢弃的事件不会再走屏蔽/通知
		stages = append(stages,
			testFireStage{Stage: "mute", Status: testFireStageSkip, Data: gin.H{"reason": "event_dropped"}},
			testFireStage{Stage: "notify", Status: testFireStageSkip, Data: gin.H{"reason": "event_dropped"}},
		)
	} else {
		// ---- Stage 4: 屏蔽匹配（命中只警示不拦截，用户的目的是测通知）----
		stages = append(stages, rt.runTestFireMuteCheck(bgid, event))

		// ---- Stage 5: 通知匹配与发送 ----
		stages = append(stages, rt.runTestFireNotify(cfg, event, f.SkipSend, username, notifyRuleMap))
	}

	// ---- Stage 6: 测试模式跳过的副作用 ----
	stages = append(stages, testFireSideEffectsStage(cfg))

	ginx.NewRender(c).Data(gin.H{
		"event":  event,
		"stages": stages,
	}, nil)
}

func hasEnabledPipeline(pcs []models.PipelineConfig) bool {
	for _, pc := range pcs {
		if pc.Enable && pc.PipelineId > 0 {
			return true
		}
	}
	return false
}

// 单请求引用的通知规则 / 流水线数量上限，超限视为异常请求
const (
	testFireMaxNotifyRules = 20
	testFireMaxPipelines   = 20
)

// authorizeNotifyRules 加载 cfg 引用的通知规则并做归属校验，未授权直接 403。
// 不存在的 ID 不返回（通知阶段按「未找到」处理，不泄露其他团队信息）。
// 通知规则按用户组归属（UserGroupIds），故这里用调用者的用户组集合比对。
func (rt *Router) authorizeNotifyRules(cfg *models.AlertRule, myUserGroupIds map[int64]struct{}, isAdmin bool) map[int64]*models.NotifyRule {
	out := make(map[int64]*models.NotifyRule)
	if cfg.NotifyVersion != 1 || len(cfg.NotifyRuleIds) == 0 {
		return out
	}
	for _, nrid := range cfg.NotifyRuleIds {
		nr, err := models.GetNotifyRule(rt.Ctx, nrid)
		if err != nil || nr == nil {
			continue
		}
		if !isAdmin && !hasGroupIntersection(myUserGroupIds, nr.UserGroupIds) {
			ginx.Bomb(http.StatusForbidden, "forbidden notify rule: %d", nrid)
		}
		out[nrid] = nr
	}
	return out
}

// authorizePipelines 加载 cfg 引用的事件处理流水线并做归属校验，未授权直接 403。
// 流水线按业务组归属（EventPipeline.GroupId 是业务组 id，与用户组 id 不同一空间）：
// GroupId==本规则业务组 → 请求已过 bgrw，放行；其他业务组 → 走 CanDoBusiGroup；
// GroupId==0（团队级）→ 走 TeamIds 的用户组权限。语义与 checkEventPipelinePermission 对齐。
func (rt *Router) authorizePipelines(cfg *models.AlertRule, bgid int64, isAdmin bool, me *models.User) map[int64]*models.EventPipeline {
	out := make(map[int64]*models.EventPipeline)
	ids := make([]int64, 0)
	for _, pc := range cfg.PipelineConfigs {
		if pc.Enable && pc.PipelineId > 0 {
			ids = append(ids, pc.PipelineId)
		}
	}
	if len(ids) == 0 {
		return out
	}
	pipelines, err := models.GetEventPipelinesByIds(rt.Ctx, ids)
	ginx.Dangerous(err)
	for _, p := range pipelines {
		authorized := isAdmin
		if !authorized {
			if p.GroupId > 0 {
				if p.GroupId == bgid {
					authorized = true
				} else if bg := BusiGroup(rt.Ctx, p.GroupId); bg != nil {
					can, err := me.CanDoBusiGroup(rt.Ctx, bg)
					ginx.Dangerous(err)
					authorized = can
				}
			} else {
				authorized = me.CheckGroupPermission(rt.Ctx, p.TeamIds) == nil
			}
		}
		if !authorized {
			ginx.Bomb(http.StatusForbidden, "forbidden event pipeline: %d", p.ID)
		}
		out[p.ID] = p
	}
	return out
}

func hasGroupIntersection(myUserGroupIds map[int64]struct{}, groupIds []int64) bool {
	for _, gid := range groupIds {
		if _, ok := myUserGroupIds[gid]; ok {
			return true
		}
	}
	return false
}

func dedupInt64(in []int64) []int64 {
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// dedupPipelineConfigs 按 pipeline_id 保序去重；id==0 的项原样保留（不构成放大）
func dedupPipelineConfigs(in []models.PipelineConfig) []models.PipelineConfig {
	seen := make(map[int64]struct{}, len(in))
	out := make([]models.PipelineConfig, 0, len(in))
	for _, pc := range in {
		if pc.PipelineId != 0 {
			if _, ok := seen[pc.PipelineId]; ok {
				continue
			}
			seen[pc.PipelineId] = struct{}{}
		}
		out = append(out, pc)
	}
	return out
}

func (rt *Router) synthesizeTestEvent(cfg *models.AlertRule, f *AlertRuleTestFireForm, bgid int64) (*models.AlertCurEvent, testFireStage) {
	now := time.Now().Unix()
	event := cfg.GenerateNewEvent(rt.Ctx)

	sampleSource := "mock"
	labels := map[string]string{
		"ident":  "mock-host-01",
		"source": "alert-rule-test-fire",
	}
	triggerValue := "81.5"
	promql := ""

	if f.Sample != nil && len(f.Sample.Labels) > 0 {
		sampleSource = "real"
		labels = map[string]string{}
		for k, v := range f.Sample.Labels {
			// __name__ 等内部 label 不进事件标签，与真实评估产出的事件保持一致
			if strings.HasPrefix(k, "__") || k == "" {
				continue
			}
			labels[k] = v
		}
		triggerValue = readableFloat(f.Sample.Value)
		promql = f.Sample.Query
	}
	if promql == "" {
		promql = firstQueryFromRuleConfig(cfg.RuleConfig)
	}

	// 标签：series labels + 规则附加标签 + rulename，rulename 用原始规则名（[TEST] 前缀只加在通知标题上）
	labels["rulename"] = cfg.Name
	for _, pair := range cfg.AppendTagsJSON {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 && kv[0] != "" {
			labels[kv[0]] = kv[1]
		}
	}
	tagKeys := make([]string, 0, len(labels))
	for k := range labels {
		tagKeys = append(tagKeys, k)
	}
	sort.Strings(tagKeys)
	tagsJSON := make([]string, 0, len(labels))
	for _, k := range tagKeys {
		tagsJSON = append(tagsJSON, k+"="+labels[k])
	}

	event.Id = 0
	event.GroupId = bgid
	event.Hash = "test-fire-" + uuid.NewString()
	event.Severity = f.Severity
	event.IsRecovered = f.EventType == "recover"
	event.TriggerTime = now
	event.FirstTriggerTime = now
	event.LastEvalTime = now
	event.NotifyCurNumber = 1
	event.TriggerValue = triggerValue
	event.TriggerValues = triggerValue
	event.PromQl = promql
	event.PromEvalInterval = cfg.PromEvalInterval
	event.NotifyVersion = cfg.NotifyVersion
	event.NotifyRuleIds = cfg.NotifyRuleIds
	event.Tags = strings.Join(tagsJSON, ",,")
	event.TagsJSON = tagsJSON
	event.SetTagsMap()

	if bg, err := models.BusiGroupGetById(rt.Ctx, bgid); err == nil && bg != nil {
		event.GroupName = bg.Name
	}

	// DatasourceCache 单测环境下为空，做 nil 保护
	var dsId int64
	if len(cfg.DatasourceIdsJson) > 0 && cfg.DatasourceIdsJson[0] > 0 {
		dsId = cfg.DatasourceIdsJson[0]
	} else if len(cfg.DatasourceQueries) > 0 && rt.DatasourceCache != nil {
		if ids := rt.DatasourceCache.GetIDsByDsCateAndQueries(cfg.Cate, cfg.DatasourceQueries); len(ids) > 0 {
			dsId = ids[0]
		}
	}
	event.DatasourceId = dsId
	if rt.DatasourceCache != nil {
		if ds := rt.DatasourceCache.GetById(dsId); ds != nil {
			event.Cluster = ds.Name
		}
	}

	event.Annotations = cfg.Annotations
	if event.Annotations == "" {
		event.Annotations = "{}"
	}

	// 渲染模板变量（{{$value}} / {{$labels.xxx}}）。顺序不可变：rule_name/rule_note 依赖 $annotations
	renderErrors := []string{}
	for _, field := range []string{"annotations", "rule_name", "rule_note"} {
		if err := event.ParseRule(field); err != nil {
			renderErrors = append(renderErrors, fmt.Sprintf("%s: %v", field, err))
		}
	}
	// 渲染完成后再打 [TEST] 标记，模板里引用的规则名保持原值
	event.RuleName = "[TEST] " + event.RuleName

	status := testFireStagePass
	if len(renderErrors) > 0 {
		status = testFireStageWarn
	}
	return event, testFireStage{
		Stage:  "synthesize",
		Status: status,
		Data: gin.H{
			"sample_source":    sampleSource,
			"labels":           labels,
			"value":            triggerValue,
			"render_errors":    renderErrors,
			"notify_recovered": cfg.NotifyRecovered,
		},
	}
}

func (rt *Router) runTestFirePipelines(cfg *models.AlertRule, event *models.AlertCurEvent, username string, skipSend bool, pipelineMap map[int64]*models.EventPipeline) (testFireStage, *models.AlertCurEvent, bool) {
	if !hasEnabledPipeline(cfg.PipelineConfigs) {
		return testFireStage{Stage: "pipeline", Status: testFireStagePass, Data: gin.H{"pipelines": []gin.H{}}}, nil, false
	}

	status := testFireStagePass
	results := make([]gin.H, 0)
	current := event
	dropped := false

	// 与真实链路一致：按 PipelineConfigs 的配置顺序遍历，跳过禁用配置/禁用流水线/过滤不命中的流水线
	for _, pc := range cfg.PipelineConfigs {
		if !pc.Enable {
			continue
		}
		p := pipelineMap[pc.PipelineId]
		if p == nil {
			results = append(results, gin.H{"id": pc.PipelineId, "status": "not_found"})
			continue
		}
		item := gin.H{"id": p.ID, "name": p.Name}
		if p.Disabled {
			item["status"] = "disabled_skipped"
			results = append(results, item)
			continue
		}
		if !dispatch.PipelineApplicable(p, current) {
			item["status"] = "not_applicable"
			results = append(results, item)
			continue
		}
		if skipSend {
			// 干跑：流水线节点（callback/aisummary 等）有真实外部副作用，不执行
			item["status"] = "dry_run_skipped"
			results = append(results, item)
			continue
		}
		resultEvent, execItem, d := rt.executeOnePipeline(p, current, username)
		results = append(results, execItem)
		if execItem["status"] == "failed" {
			status = testFireStageFail
		}
		if d {
			dropped = true
			if status == testFireStagePass {
				status = testFireStageWarn
			}
			break
		}
		current = resultEvent
	}

	return testFireStage{Stage: "pipeline", Status: status, Data: gin.H{"pipelines": results, "skip_send": skipSend}}, current, dropped
}

func (rt *Router) executeOnePipeline(p *models.EventPipeline, event *models.AlertCurEvent, username string) (*models.AlertCurEvent, gin.H, bool) {
	item := gin.H{"id": p.ID, "name": p.Name}
	workflowEngine := engine.NewWorkflowEngine(rt.Ctx)
	triggerCtx := &models.WorkflowTriggerContext{
		Mode:      models.TriggerModeAPI,
		TriggerBy: username,
		RequestID: uuid.NewString(),
	}
	resultEvent, result, err := workflowEngine.Execute(p, event, triggerCtx)
	if err != nil {
		item["status"] = "failed"
		item["error"] = redactSensitive(err.Error())
		return event, item, false
	}
	item["status"] = result.Status
	item["node_results"] = sanitizeNodeResults(result.NodeResults)
	if resultEvent == nil {
		item["dropped"] = true
		return nil, item, true
	}
	return resultEvent, item, false
}

func (rt *Router) runTestFireMuteCheck(bgid int64, event *models.AlertCurEvent) testFireStage {
	mutes, err := models.AlertMuteGetsByBG(rt.Ctx, bgid)
	if err != nil {
		return testFireStage{Stage: "mute", Status: testFireStageWarn, Data: gin.H{"error": err.Error()}}
	}

	matched := make([]gin.H, 0)
	for i := range mutes {
		m := mutes[i]
		if m.Disabled == 1 {
			continue
		}
		if err := m.Verify(); err != nil {
			logger.Warningf("test-fire: verify mute %d failed: %v", m.Id, err)
			continue
		}
		if ok, _ := mute.MatchMute(event, &m); ok {
			matched = append(matched, gin.H{"id": m.Id, "note": m.Note})
		}
	}

	status := testFireStagePass
	if len(matched) > 0 {
		status = testFireStageWarn
	}
	return testFireStage{Stage: "mute", Status: status, Data: gin.H{"matched_mutes": matched}}
}

func (rt *Router) runTestFireNotify(cfg *models.AlertRule, event *models.AlertCurEvent, skipSend bool, username string, notifyRuleMap map[int64]*models.NotifyRule) testFireStage {
	// 真实链路中，未开启「恢复时通知」的规则不会发送恢复通知（RecoverSingle 会重判），
	// 测试保持同样行为：跳过发送并在报告中说明，避免测试结果与真实行为不一致
	if event.IsRecovered && cfg.NotifyRecovered != 1 {
		return testFireStage{
			Stage:  "notify",
			Status: testFireStageWarn,
			Data:   gin.H{"recover_notify_disabled": true, "results": []gin.H{}},
		}
	}

	if cfg.NotifyVersion != 1 {
		missing := rt.legacyNotifyMissingTokens(cfg)
		status := testFireStagePass
		if len(missing) > 0 {
			status = testFireStageWarn
		}
		return testFireStage{
			Stage:  "notify",
			Status: status,
			Data:   gin.H{"legacy": true, "missing_channels": missing},
		}
	}

	if len(cfg.NotifyRuleIds) == 0 {
		// 新人最常见的坑：规则配好了但没有任何通知目标
		return testFireStage{Stage: "notify", Status: testFireStageWarn, Data: gin.H{"no_targets": true, "results": []gin.H{}}}
	}

	channelNames := map[int64]string{}
	channelName := func(id int64) string {
		if name, ok := channelNames[id]; ok {
			return name
		}
		name := fmt.Sprintf("channel-%d", id)
		if lst, err := models.NotifyChannelGets(rt.Ctx, id, "", "", -1); err == nil && len(lst) > 0 {
			name = lst[0].Name
		}
		channelNames[id] = name
		return name
	}

	results := make([]gin.H, 0)
	anyFail := false
	for _, nrid := range cfg.NotifyRuleIds {
		nr := notifyRuleMap[nrid]
		if nr == nil {
			results = append(results, gin.H{"notify_rule_id": nrid, "matched": false, "match_error": "notify rule not found"})
			anyFail = true
			continue
		}
		if !nr.Enable {
			results = append(results, gin.H{"notify_rule_id": nr.ID, "notify_rule_name": nr.Name, "matched": false, "match_error": "notify rule disabled"})
			continue
		}

		// 真实链路每条通知规则内部会先跑自身的流水线，返回 nil 则不发送。
		// 干跑不执行流水线（避免其 callback 等副作用），只在真实发送路径复现该判定。
		sendEvent := event
		if !skipSend {
			processed, dropped := rt.runNotifyOwnPipelines(nr.PipelineConfigs, event, username)
			if dropped {
				results = append(results, gin.H{
					"notify_rule_id":             nr.ID,
					"notify_rule_name":           nr.Name,
					"matched":                    false,
					"dropped_by_notify_pipeline": true,
				})
				continue
			}
			sendEvent = processed
		}

		for i := range nr.NotifyConfigs {
			nc := nr.NotifyConfigs[i]
			item := gin.H{
				"notify_rule_id":   nr.ID,
				"notify_rule_name": nr.Name,
				"channel_id":       nc.ChannelID,
				"channel_name":     channelName(nc.ChannelID),
			}
			// 现有 notify-tryrun 漏掉了匹配检查直接发送，这里必须做，否则测试结果与真实链路不一致
			if err := dispatch.NotifyRuleMatchCheck(&nc, sendEvent); err != nil {
				item["matched"] = false
				item["match_error"] = err.Error()
				results = append(results, item)
				continue
			}
			item["matched"] = true
			if skipSend {
				item["sent"] = false
				item["skipped"] = true
				results = append(results, item)
				continue
			}
			resp, err := SendNotifyChannelMessage(rt.Ctx, rt.UserCache, rt.UserGroupCache, nc, []*models.AlertCurEvent{sendEvent})
			if err != nil {
				item["sent"] = false
				item["error"] = redactSensitive(err.Error())
				anyFail = true
			} else {
				item["sent"] = true
				item["response"] = redactSensitive(resp)
			}
			results = append(results, item)
		}
	}

	status := testFireStagePass
	if anyFail {
		status = testFireStageFail
	}
	return testFireStage{Stage: "notify", Status: status, Data: gin.H{"results": results, "skip_send": skipSend}}
}

// runNotifyOwnPipelines 跑通知规则自身挂的流水线，返回处理后事件与是否被丢弃。
// 事件用深拷贝，避免污染主链路后续阶段依赖的事件。
func (rt *Router) runNotifyOwnPipelines(pcs []models.PipelineConfig, event *models.AlertCurEvent, username string) (*models.AlertCurEvent, bool) {
	if !hasEnabledPipeline(pcs) {
		return event, false
	}
	ids := make([]int64, 0)
	for _, pc := range pcs {
		if pc.Enable && pc.PipelineId > 0 {
			ids = append(ids, pc.PipelineId)
		}
	}
	pipelines, err := models.GetEventPipelinesByIds(rt.Ctx, ids)
	if err != nil {
		logger.Warningf("test-fire: load notify rule pipelines failed: %v", err)
		return event, false
	}
	pipelineMap := make(map[int64]*models.EventPipeline, len(pipelines))
	for _, p := range pipelines {
		pipelineMap[p.ID] = p
	}

	current := event.DeepCopy()
	for _, pc := range pcs {
		if !pc.Enable {
			continue
		}
		p := pipelineMap[pc.PipelineId]
		if p == nil || p.Disabled {
			continue
		}
		if !dispatch.PipelineApplicable(p, current) {
			continue
		}
		resultEvent, _, d := rt.executeOnePipeline(p, current, username)
		if d {
			return nil, true
		}
		current = resultEvent
	}
	return current, false
}

// legacyNotifyMissingTokens 校验旧版通知（notify_version=0）各内置渠道是否至少有一个接收人配置了 token，
// 逻辑与 alertRuleNotifyTryRun 的旧版分支一致，只报告不发送
func (rt *Router) legacyNotifyMissingTokens(cfg *models.AlertRule) []string {
	missing := make([]string, 0)
	if len(cfg.NotifyChannelsJSON) == 0 || len(cfg.NotifyGroupsJSON) == 0 {
		return missing
	}

	ngids := make([]int64, 0, len(cfg.NotifyGroupsJSON))
	for _, gidStr := range cfg.NotifyGroupsJSON {
		var gid int64
		if _, err := fmt.Sscanf(gidStr, "%d", &gid); err == nil {
			ngids = append(ngids, gid)
		}
	}
	userGroups := rt.UserGroupCache.GetByUserGroupIds(ngids)
	uids := make([]int64, 0)
	for i := range userGroups {
		uids = append(uids, userGroups[i].UserIds...)
	}
	users := rt.UserCache.GetByUserIds(uids)

	for _, channel := range cfg.NotifyChannelsJSON {
		switch channel {
		case models.Dingtalk, models.Wecom, models.Feishu, models.Mm,
			models.Telegram, models.Email, models.FeishuCard:
		default:
			continue
		}
		found := false
		for ui := range users {
			if _, ok := users[ui].ExtractToken(channel); ok {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, channel)
		}
	}
	return missing
}

func testFireSideEffectsStage(cfg *models.AlertRule) testFireStage {
	taskTpls := 0
	if cfg.RuleConfig != "" {
		var rc struct {
			TaskTpls []struct {
				TplId int64 `json:"tpl_id"`
			} `json:"task_tpls"`
		}
		if err := json.Unmarshal([]byte(cfg.RuleConfig), &rc); err == nil {
			for _, tpl := range rc.TaskTpls {
				if tpl.TplId > 0 {
					taskTpls++
				}
			}
		}
	}
	return testFireStage{
		Stage:  "side_effects",
		Status: testFireStageSkip,
		Data: gin.H{
			"callbacks": len(cfg.CallbacksJSON),
			"task_tpls": taskTpls,
		},
	}
}

func firstQueryFromRuleConfig(ruleConfig string) string {
	if ruleConfig == "" {
		return ""
	}
	var rc struct {
		Queries []struct {
			PromQl string `json:"prom_ql"`
			Query  string `json:"query"`
		} `json:"queries"`
	}
	if err := json.Unmarshal([]byte(ruleConfig), &rc); err != nil {
		return ""
	}
	for _, q := range rc.Queries {
		if q.PromQl != "" {
			return q.PromQl
		}
		if q.Query != "" {
			return q.Query
		}
	}
	return ""
}

func readableFloat(v float64) string {
	ret := fmt.Sprintf("%.5f", v)
	ret = strings.TrimRight(ret, "0")
	return strings.TrimRight(ret, ".")
}

// sanitizeNodeResults 只回传节点的安全字段，并对错误做脱敏，
// 避免 callback 等处理器把整个 config（含 AuthPassword/Headers 里的 token）格式化进 error 后泄露给调用者
func sanitizeNodeResults(nodes []*models.NodeExecutionResult) []gin.H {
	out := make([]gin.H, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		item := gin.H{
			"node_id":     n.NodeID,
			"node_name":   n.NodeName,
			"node_type":   n.NodeType,
			"status":      n.Status,
			"duration_ms": n.DurationMs,
		}
		if n.Error != "" {
			item["error"] = redactSensitive(n.Error)
		}
		out = append(out, item)
	}
	return out
}

var testFireSensitiveMarkers = []string{
	"password", "passwd", "secret", "token", "authorization",
	"api_key", "apikey", "authusername", "authpassword", "header",
}

// redactSensitive 保守脱敏：字符串中出现任何可能承载凭据的字段名标记时，整体替换为通用提示；
// 否则仅做长度截断。用于对外返回的错误/响应文本，防止流水线节点凭据经 error 泄露
func redactSensitive(s string) string {
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	for _, marker := range testFireSensitiveMarkers {
		if strings.Contains(lower, marker) {
			return "(details hidden for security)"
		}
	}
	const max = 300
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}
