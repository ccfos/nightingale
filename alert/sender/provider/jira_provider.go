package provider

// Jira 工单媒介：告警触发时建单，恢复时评论并流转到「完成」类别。
//
// 去重与恢复语义（JQL 写法、重开 / 忽略解决结果的状态机）移植自 Prometheus Alertmanager
// notify/jira/jira.go（Apache License 2.0），在其基础上加了：进程内缓存补 Jira Cloud 搜索的
// 最终一致窗口、恢复时评论（Zabbix 的做法）、自动识别流转到「完成」的动作、乱序到达的防护。
// 设计见方案文档「n9e-Jira通知媒介-方案.md」。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	gocache "github.com/patrickmn/go-cache"
	"github.com/toolkits/pkg/logger"
)

const (
	jiraMaxSummaryRunes     = 255
	jiraMaxDescriptionRunes = 32000 // Jira 上限 32767，留出余量
	jiraMaxLabelRunes       = 255
	jiraMaxTagLabels        = 20
	jiraDedupLabelPrefix    = "eventHash="

	jiraOnResolveClose   = "close"
	jiraOnResolveComment = "comment"
	jiraOnResolveNone    = "none"
	jiraOnRepeatComment  = "comment"

	// TestNonceParam 是测试接口写进通知参数的一次性随机串：只有 Jira 用它拼去重键，
	// 让每次测试都建一张新单，且「同时测试恢复」的两次发送能对上同一张单。
	TestNonceParam = "__test_nonce"
)

// jiraIssueCache 缓存 媒介 id + 项目 + 去重键 -> 工单，补 Jira Cloud 搜索的最终一致窗口：
// 刚建的单立刻再搜可能搜不到，命中缓存时改用按 key 取实时状态。
var jiraIssueCache = gocache.New(24*time.Hour, time.Hour)

// jiraRecoverTombstones 记录「恢复时没找到工单」的事件：之后才到达的、更早的触发视为乱序，不再建单
var jiraRecoverTombstones = gocache.New(time.Hour, 10*time.Minute)

// jiraLocks 按去重键分段加锁，保证同一告警的建单与恢复不并发
var jiraLocks [256]sync.Mutex

func jiraLock(key string) *sync.Mutex {
	h := fnv.New32a()
	h.Write([]byte(key))
	return &jiraLocks[h.Sum32()%uint32(len(jiraLocks))]
}

type cachedJiraIssue struct {
	Key  string
	Done bool
}

type JiraProvider struct{}

func (p *JiraProvider) Ident() string { return models.RequestTypeJira }

func (p *JiraProvider) Check(config *models.NotifyChannelConfig) error {
	if config.RequestType != models.RequestTypeJira {
		return errors.New("jira provider requires request_type: jira")
	}
	return config.ValidateJiraRequestConfig()
}

// jiraParams 是通知规则里 Jira 这条通知配置的参数，复杂值以 JSON 字符串保存
type jiraParams struct {
	ProjectKey        string
	IssueType         string
	OnResolve         string
	ResolveTransition string
	OnRepeat          string
	PriorityMap       map[string]string
	Labels            []string
	TagsAsLabels      bool
	Fields            map[string]string
	TestNonce         string
}

func parseJiraParams(p map[string]string) (*jiraParams, error) {
	jp := &jiraParams{
		ProjectKey:        strings.TrimSpace(p["project_key"]),
		IssueType:         strings.TrimSpace(p["issue_type"]),
		OnResolve:         strings.TrimSpace(p["on_resolve"]),
		ResolveTransition: strings.TrimSpace(p["resolve_transition"]),
		OnRepeat:          strings.TrimSpace(p["on_repeat"]),
		TagsAsLabels:      p["tags_as_labels"] == "true",
		TestNonce:         p[TestNonceParam],
	}
	if jp.ProjectKey == "" {
		return nil, errors.New("jira project_key is required in the notify rule")
	}
	if jp.IssueType == "" {
		return nil, errors.New("jira issue_type is required in the notify rule")
	}
	switch jp.OnResolve {
	case "":
		jp.OnResolve = jiraOnResolveClose
	case jiraOnResolveClose, jiraOnResolveComment, jiraOnResolveNone:
	default:
		return nil, fmt.Errorf("invalid on_resolve %q, must be close, comment or none", jp.OnResolve)
	}

	if err := unmarshalParam(p, "priority_map", &jp.PriorityMap); err != nil {
		return nil, err
	}
	if err := unmarshalParam(p, "labels", &jp.Labels); err != nil {
		return nil, err
	}
	if err := unmarshalParam(p, "fields", &jp.Fields); err != nil {
		return nil, err
	}
	return jp, nil
}

// unmarshalParam 解析以 JSON 字符串保存的参数，空串视为未设置
func unmarshalParam(p map[string]string, key string, out interface{}) error {
	v := strings.TrimSpace(p[key])
	if v == "" || v == "null" {
		return nil
	}
	if err := json.Unmarshal([]byte(v), out); err != nil {
		return fmt.Errorf("invalid %s: %v", key, err)
	}
	return nil
}

func (p *JiraProvider) Notify(ctx context.Context, req *NotifyRequest) *NotifyResult {
	params, err := parseJiraParams(req.CustomParams)
	if err != nil {
		return &NotifyResult{Err: err}
	}
	client, err := newJiraClient(ctx, req.Config, req.HttpClient)
	if err != nil {
		return &NotifyResult{Target: params.ProjectKey, Err: err}
	}

	var targets, responses []string
	var errs []error
	for _, event := range req.Events {
		if event == nil {
			continue
		}
		key, resp, err := p.handleEvent(ctx, client, req, params, event)
		if key != "" {
			targets = append(targets, key)
		}
		if resp != "" {
			responses = append(responses, resp)
		}
		if err != nil {
			errs = append(errs, err)
			logger.Warningf("jira_notify: channel=%s project=%s event=%s err=%v", req.Config.Name, params.ProjectKey, event.Hash, err)
		}
	}

	res := &NotifyResult{Target: strings.Join(targets, ","), Response: strings.Join(responses, "; ")}
	if res.Target == "" {
		res.Target = params.ProjectKey
	}
	// 单个错误原样返回（保留 HintError，测试接口才能翻译提示），多个用 errors.Join 合并
	if len(errs) == 1 {
		res.Err = errs[0]
	} else if len(errs) > 1 {
		res.Err = errors.Join(errs...)
	}
	return res
}

// handleEvent 处理单个事件，返回 issue key、给通知记录看的动作说明、错误。
// 「已存在」「没找到工单」「已关闭」等正常结局返回 nil 错误。
func (p *JiraProvider) handleEvent(ctx context.Context, c *jiraClient, req *NotifyRequest, jp *jiraParams, event *models.AlertCurEvent) (string, string, error) {
	dedupKey := event.Hash
	if jp.TestNonce != "" {
		dedupKey += "-" + jp.TestNonce
	}
	var channelID int64
	if req.Config != nil {
		channelID = req.Config.ID
	}
	cacheKey := fmt.Sprintf("%d|%s|%s|%s", channelID, c.siteURL, jp.ProjectKey, dedupKey)

	mu := jiraLock(cacheKey)
	mu.Lock()
	defer mu.Unlock()

	if event.IsRecovered {
		return p.handleRecovered(ctx, c, req, jp, event, cacheKey, dedupKey)
	}
	return p.handleFiring(ctx, c, req, jp, event, cacheKey, dedupKey)
}

func (p *JiraProvider) handleFiring(ctx context.Context, c *jiraClient, req *NotifyRequest, jp *jiraParams, event *models.AlertCurEvent, cacheKey, dedupKey string) (string, string, error) {
	issue, err := p.lookup(ctx, c, jp, cacheKey, dedupKey)
	if err != nil {
		return "", "", err
	}

	if issue == nil {
		// 恢复先于触发被处理（事件消费乱序）时，这条触发已经过期，建出来的单再也不会被关
		if v, ok := jiraRecoverTombstones.Get(cacheKey); ok && event.TriggerTime > 0 && event.TriggerTime <= v.(int64) {
			return "", "skipped: the alert had already recovered before this trigger was processed", nil
		}
		return p.create(ctx, c, req, jp, event, cacheKey, dedupKey)
	}

	if !issue.done() {
		if jp.OnRepeat == jiraOnRepeatComment {
			if err := c.addComment(ctx, issue.Key, contentOf(req.TplContent)); err != nil {
				return issue.Key, "", err
			}
			return issue.Key, fmt.Sprintf("exists, commented %s %s", issue.Key, c.browseURL(issue.Key)), nil
		}
		return issue.Key, fmt.Sprintf("exists, not created again %s %s", issue.Key, c.browseURL(issue.Key)), nil
	}

	// 缓存里记着的单已经关闭（恢复时关的，或被人手动关了）：同一告警再次触发就建新单。
	// 不做「时间窗内重开旧单」：那需要跨工单生命周期记关闭时间和解决结果，状态和分支都多一层
	return p.create(ctx, c, req, jp, event, cacheKey, dedupKey)
}

func (p *JiraProvider) handleRecovered(ctx context.Context, c *jiraClient, req *NotifyRequest, jp *jiraParams, event *models.AlertCurEvent, cacheKey, dedupKey string) (string, string, error) {
	issue, err := p.lookup(ctx, c, jp, cacheKey, dedupKey)
	if err != nil {
		return "", "", err
	}
	if issue == nil {
		recoveredAt := event.LastEvalTime
		if recoveredAt == 0 {
			recoveredAt = time.Now().Unix()
		}
		jiraRecoverTombstones.Set(cacheKey, recoveredAt, gocache.DefaultExpiration)
		return "", "no matching open issue found", nil
	}
	if issue.done() {
		return issue.Key, fmt.Sprintf("issue already closed %s %s", issue.Key, c.browseURL(issue.Key)), nil
	}
	if jp.OnResolve == jiraOnResolveNone {
		return issue.Key, fmt.Sprintf("recovered, left untouched %s %s", issue.Key, c.browseURL(issue.Key)), nil
	}

	if err := c.addComment(ctx, issue.Key, contentOf(req.TplContent)); err != nil {
		return issue.Key, "", err
	}
	if jp.OnResolve == jiraOnResolveComment {
		return issue.Key, fmt.Sprintf("commented %s %s", issue.Key, c.browseURL(issue.Key)), nil
	}

	// 关单失败只记录原因、不算发送失败：评论已经写上了
	name, err := p.closeIssue(ctx, c, issue.Key, jp.ResolveTransition)
	if err != nil {
		return issue.Key, fmt.Sprintf("commented %s %s, but failed to close: %s", issue.Key, c.browseURL(issue.Key), err.Error()), nil
	}
	jiraIssueCache.Set(cacheKey, cachedJiraIssue{Key: issue.Key, Done: true}, gocache.DefaultExpiration)
	return issue.Key, fmt.Sprintf("commented and closed via %q %s %s", name, issue.Key, c.browseURL(issue.Key)), nil
}

// lookup 找到该告警对应的工单：先查缓存（命中时按 key 取实时状态，不依赖搜索；可能是已关闭的单），
// 再按 JQL 搜索未关闭的单。
func (p *JiraProvider) lookup(ctx context.Context, c *jiraClient, jp *jiraParams, cacheKey, dedupKey string) (*jiraIssue, error) {
	if v, ok := jiraIssueCache.Get(cacheKey); ok {
		cached := v.(cachedJiraIssue)
		issue, err := c.getIssue(ctx, cached.Key)
		if err != nil {
			return nil, err
		}
		if issue != nil {
			// 已关闭的单也原样返回：触发时由 handleFiring 判断重开还是建新单，恢复时记为「已关闭」
			return issue, nil
		}
		// 工单已被删除，缓存作废，退回搜索
		jiraIssueCache.Delete(cacheKey)
	}

	issue, err := c.searchIssue(ctx, buildJiraJQL(jp, dedupKey))
	if err != nil {
		return nil, err
	}
	if issue != nil {
		jiraIssueCache.Set(cacheKey, cachedJiraIssue{Key: issue.Key, Done: issue.done()}, gocache.DefaultExpiration)
	}
	return issue, nil
}

// buildJiraJQL 按去重标签找该告警未关闭的单，取自 Alertmanager notify/jira/jira.go 的 searchExistingIssue
func buildJiraJQL(jp *jiraParams, dedupKey string) string {
	return strings.Join([]string{
		"statusCategory != Done",
		"project = " + jqlQuote(jp.ProjectKey),
		"labels = " + jqlQuote(jiraDedupLabelPrefix+dedupKey),
	}, " AND ") + " ORDER BY created DESC"
}

func jqlQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func (p *JiraProvider) create(ctx context.Context, c *jiraClient, req *NotifyRequest, jp *jiraParams, event *models.AlertCurEvent, cacheKey, dedupKey string) (string, string, error) {
	fields := map[string]interface{}{
		"project":     map[string]string{"key": jp.ProjectKey},
		"issuetype":   map[string]string{"name": jp.IssueType},
		"summary":     jiraSummary(req.TplContent, event),
		"description": jiraADF(contentOf(req.TplContent), jiraMaxDescriptionRunes),
		"labels":      jiraLabels(jp, event, dedupKey),
	}
	if name := jp.PriorityMap[strconv.Itoa(event.Severity)]; name != "" {
		fields["priority"] = map[string]string{"name": name}
	}
	for k, v := range jp.Fields {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		// 值是合法 JSON 时按 JSON 提交（下拉、多选等字段），否则按文本提交
		if json.Valid([]byte(v)) {
			fields[k] = json.RawMessage(v)
		} else {
			fields[k] = v
		}
	}

	key, err := c.createIssue(ctx, fields)
	if err != nil {
		return "", "", err
	}
	jiraIssueCache.Set(cacheKey, cachedJiraIssue{Key: key}, gocache.DefaultExpiration)
	jiraRecoverTombstones.Delete(cacheKey)
	return key, fmt.Sprintf("created %s %s", key, c.browseURL(key)), nil
}

// closeIssue 执行流转到「完成」类别的动作，返回实际使用的动作名。
// name 为空时自动选择：候选是目标状态类别为 done 的动作，按常见名称偏好排序。
func (p *JiraProvider) closeIssue(ctx context.Context, c *jiraClient, key, name string) (string, error) {
	return p.transitionByName(ctx, c, key, name, func(t jiraTransition) bool { return t.To.StatusCategory.Key == "done" })
}

var jiraDonePreference = []string{"done", "resolve issue", "resolve", "close issue", "close", "完成", "已完成", "解决", "关闭"}

// transitionByName 按名称（不区分大小写，也可以是数字 ID）执行流转；name 为空时用 auto 从候选里挑。
// 流转要求填写解决结果时，带 {"resolution":{"name":"Done"}} 重试一次。
func (p *JiraProvider) transitionByName(ctx context.Context, c *jiraClient, key, name string, auto func(jiraTransition) bool) (string, error) {
	ts, err := c.listTransitions(ctx, key)
	if err != nil {
		return "", err
	}

	var picked *jiraTransition
	if name != "" {
		for i := range ts {
			if strings.EqualFold(ts[i].Name, name) || ts[i].ID == name {
				picked = &ts[i]
				break
			}
		}
		if picked == nil {
			return "", withHint(fmt.Errorf("transition %q not found, available: %s", name, transitionNames(ts)),
				"Fill in one of the available transition names, or leave it empty to pick the Done transition automatically")
		}
	} else if auto != nil {
		var cands []jiraTransition
		for _, t := range ts {
			if auto(t) {
				cands = append(cands, t)
			}
		}
		if len(cands) == 0 {
			return "", withHint(fmt.Errorf("no transition leads to a Done status, available: %s", transitionNames(ts)),
				"Check the workflow of this issue type, or fill in the close transition name in the notify rule")
		}
		picked = &cands[0]
	pick:
		for _, pref := range jiraDonePreference {
			for i := range cands {
				if strings.EqualFold(cands[i].Name, pref) {
					picked = &cands[i]
					break pick
				}
			}
		}
	}
	if picked == nil {
		return "", errors.New("no transition specified")
	}

	resp, err := c.doTransition(ctx, key, picked.ID, nil)
	if err != nil && resp != nil && resp.StatusCode == http.StatusBadRequest && strings.Contains(strings.ToLower(string(resp.Body)), "resolution") {
		_, err = c.doTransition(ctx, key, picked.ID, map[string]interface{}{"resolution": map[string]string{"name": "Done"}})
		if err != nil {
			err = withHint(err, "The close transition requires a resolution; pick another transition in the notify rule, or only comment on recovery")
		}
	}
	if err != nil {
		return picked.Name, err
	}
	return picked.Name, nil
}

func transitionNames(ts []jiraTransition) string {
	names := make([]string, 0, len(ts))
	for _, t := range ts {
		names = append(names, t.Name)
	}
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

func contentOf(tpl map[string]interface{}) string {
	if tpl == nil {
		return ""
	}
	if v, ok := tpl["content"]; ok && v != nil {
		return fmt.Sprint(v)
	}
	return ""
}

// jiraSummary 取模板的 title；模板没有 title（用户自建的老模板）时按默认格式生成
func jiraSummary(tpl map[string]interface{}, event *models.AlertCurEvent) string {
	s := ""
	if v, ok := tpl["title"]; ok && v != nil {
		s = fmt.Sprint(v)
	}
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		s = fmt.Sprintf("[S%d] %s", event.Severity, event.RuleName)
		if event.TargetIdent != "" {
			s += " · " + event.TargetIdent
		}
	}
	s, _ = truncateInRunes(s, jiraMaxSummaryRunes)
	return s
}

var jiraLabelSpaces = regexp.MustCompile(`\s+`)

func jiraLabel(s string) string {
	s = jiraLabelSpaces.ReplaceAllString(strings.TrimSpace(s), "_")
	s, _ = truncateInRunes(s, jiraMaxLabelRunes)
	return s
}

// jiraLabels 组装标签：去重标签 + 规则里的固定标签 +（可选）告警标签
func jiraLabels(jp *jiraParams, event *models.AlertCurEvent, dedupKey string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(l string) {
		l = jiraLabel(l)
		if l == "" || seen[l] {
			return
		}
		seen[l] = true
		out = append(out, l)
	}
	add(jiraDedupLabelPrefix + dedupKey)
	for _, l := range jp.Labels {
		add(l)
	}
	if jp.TagsAsLabels {
		tags := append([]string{}, event.TagsJSON...)
		sort.Strings(tags)
		n := 0
		for _, t := range tags {
			if n >= jiraMaxTagLabels {
				break
			}
			add(t)
			n++
		}
	}
	return out
}

var jiraURLRe = regexp.MustCompile(`https?://[^\s<>"]+`)

// jiraADF 把纯文本包成 Atlassian Document Format：按行拆段落，行里的链接加 link 标记。
// 做法参考 Grafana Alerting 的 simpleAdfDocument，先按字符截断再包装。
func jiraADF(text string, maxRunes int) map[string]interface{} {
	text, _ = truncateInRunes(text, maxRunes)
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	content := make([]interface{}, 0, len(lines))
	for _, line := range lines {
		para := map[string]interface{}{"type": "paragraph"}
		if nodes := adfTextNodes(line); len(nodes) > 0 {
			para["content"] = nodes
		}
		content = append(content, para)
	}
	return map[string]interface{}{"type": "doc", "version": 1, "content": content}
}

func adfTextNodes(line string) []interface{} {
	var nodes []interface{}
	last := 0
	for _, loc := range jiraURLRe.FindAllStringIndex(line, -1) {
		if loc[0] > last {
			nodes = append(nodes, map[string]interface{}{"type": "text", "text": line[last:loc[0]]})
		}
		u := line[loc[0]:loc[1]]
		nodes = append(nodes, map[string]interface{}{
			"type": "text", "text": u,
			"marks": []interface{}{map[string]interface{}{"type": "link", "attrs": map[string]string{"href": u}}},
		})
		last = loc[1]
	}
	if last < len(line) {
		nodes = append(nodes, map[string]interface{}{"type": "text", "text": line[last:]})
	}
	return nodes
}

// ---- 校验凭证 ----

// CheckCredential 按方案 4.7 逐项检查：Cloud ID、凭证、账号、部署类型、权限。
// 凭证不通时自动换另一种令牌类型的地址再试一次，能通就提示用户改选令牌类型。
func (p *JiraProvider) CheckCredential(ctx context.Context, config *models.NotifyChannelConfig, httpClient *http.Client) []CheckItem {
	var items []CheckItem
	jc := config.RequestConfig.JiraRequestConfig

	tokenType := jc.TokenType
	if tokenType == "" {
		tokenType = models.JiraTokenScoped
	}
	if tokenType == models.JiraTokenScoped && strings.TrimSpace(jc.CloudID) == "" {
		site, err := expandUserVars(strings.TrimRight(strings.TrimSpace(jc.SiteURL), "/"))
		if err == nil {
			var cloudID string
			cloudID, err = resolveJiraCloudID(ctx, httpClient, site)
			if err == nil {
				items = append(items, CheckItem{Name: "Cloud ID", OK: true, Required: true, Message: cloudID})
			}
		}
		if err != nil {
			items = append(items, CheckItem{Name: "Cloud ID", Required: true, err: err, Message: err.Error()})
			return append(items, skippedJiraItems("Credentials", "Account", "Deployment type", "Browse projects", "Create issues", "Add comments", "Transition issues")...)
		}
	}

	c, err := newJiraClient(ctx, config, httpClient)
	if err != nil {
		items = append(items, CheckItem{Name: "Credentials", Required: true, err: err, Message: err.Error()})
		return append(items, skippedJiraItems("Account", "Deployment type", "Browse projects", "Create issues", "Add comments", "Transition issues")...)
	}

	resp, err := c.do(ctx, http.MethodGet, "/project/search", url.Values{"maxResults": {"1"}}, nil, nil)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusUnauthorized {
			if other := p.tryOtherTokenType(ctx, config, httpClient, tokenType); other != "" {
				err = withHint(err, "The token type does not match the token: choose "+other)
			}
		}
		items = append(items, CheckItem{Name: "Credentials", Required: true, err: err, Message: err.Error()})
		return append(items, skippedJiraItems("Account", "Deployment type", "Browse projects", "Create issues", "Add comments", "Transition issues")...)
	}
	items = append(items, CheckItem{Name: "Credentials", OK: true, Required: true})

	var me struct {
		DisplayName  string `json:"displayName"`
		EmailAddress string `json:"emailAddress"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/myself", nil, nil, &me); err != nil {
		items = append(items, CheckItem{Name: "Account", Skipped: true, Message: "Skipped: the token has no permission to read the account (read:jira-user)"})
	} else {
		items = append(items, CheckItem{Name: "Account", OK: true, Message: strings.TrimSpace(me.DisplayName + " " + me.EmailAddress)})
	}

	var info struct {
		DeploymentType string `json:"deploymentType"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/serverInfo", nil, nil, &info); err != nil || info.DeploymentType == "" {
		items = append(items, CheckItem{Name: "Deployment type", Skipped: true, Message: "Skipped: server info is not available to this token"})
	} else if !strings.EqualFold(info.DeploymentType, "Cloud") {
		items = append(items, CheckItem{Name: "Deployment type", Required: true, Message: "This is a Jira Data Center / Server site, choose Jira Data Center"})
	} else {
		items = append(items, CheckItem{Name: "Deployment type", OK: true, Required: true, Message: "Jira Cloud"})
	}

	perms, err := c.myPermissions(ctx, "")
	if err != nil {
		for _, n := range []string{"Browse projects", "Create issues", "Add comments", "Transition issues"} {
			items = append(items, CheckItem{Name: n, Skipped: true, Required: n == "Browse projects" || n == "Create issues", err: err, Message: err.Error()})
		}
		return items
	}
	for _, pm := range []struct {
		Name     string
		Key      string
		Required bool
	}{
		{"Browse projects", "BROWSE_PROJECTS", true},
		{"Create issues", "CREATE_ISSUES", true},
		{"Add comments", "ADD_COMMENTS", false},
		{"Transition issues", "TRANSITION_ISSUES", false},
	} {
		items = append(items, CheckItem{Name: pm.Name, OK: perms[pm.Key], Required: pm.Required})
	}
	return items
}

// tryOtherTokenType 用另一种令牌类型的地址试一次，能通时返回应改选的类型名
func (p *JiraProvider) tryOtherTokenType(ctx context.Context, config *models.NotifyChannelConfig, httpClient *http.Client, current string) string {
	alt := *config
	altRC := *config.RequestConfig
	altJC := *config.RequestConfig.JiraRequestConfig
	name := "Scoped API token"
	if current == models.JiraTokenScoped {
		altJC.TokenType = models.JiraTokenClassic
		name = "Classic API token"
	} else {
		altJC.TokenType = models.JiraTokenScoped
	}
	altJC.RetryTimes = 0
	altRC.JiraRequestConfig = &altJC
	alt.RequestConfig = &altRC

	c, err := newJiraClient(ctx, &alt, httpClient)
	if err != nil {
		return ""
	}
	c.retryTimes = 0
	if _, err := c.do(ctx, http.MethodGet, "/project/search", url.Values{"maxResults": {"1"}}, nil, nil); err != nil {
		return ""
	}
	return name
}

func skippedJiraItems(names ...string) []CheckItem {
	out := make([]CheckItem, 0, len(names))
	for _, n := range names {
		required := n == "Credentials" || n == "Deployment type" || n == "Browse projects" || n == "Create issues"
		out = append(out, CheckItem{Name: n, Skipped: true, Required: required})
	}
	return out
}

// ---- 供列表接口调用 ----

// NewJiraClientForChannel 为已保存的 Jira 媒介构造客户端，供项目 / 工作类型 / 优先级下拉使用
func NewJiraClientForChannel(ctx context.Context, channel *models.NotifyChannelConfig) (*JiraAPI, error) {
	if channel == nil || channel.RequestType != models.RequestTypeJira {
		return nil, errors.New("notify channel is not a jira channel")
	}
	httpClient, err := models.GetHTTPClient(channel)
	if err != nil {
		return nil, err
	}
	c, err := newJiraClient(ctx, channel, httpClient)
	if err != nil {
		return nil, err
	}
	return &JiraAPI{c: c}, nil
}

// JiraAPI 暴露给路由层的只读查询
type JiraAPI struct{ c *jiraClient }

func (a *JiraAPI) Projects(ctx context.Context) ([]JiraProject, error) { return a.c.listProjects(ctx) }

func (a *JiraAPI) IssueTypes(ctx context.Context, projectKey string) ([]JiraIssueType, error) {
	return a.c.listIssueTypes(ctx, projectKey)
}

func (a *JiraAPI) Priorities(ctx context.Context) ([]JiraPriority, error) {
	return a.c.listPriorities(ctx)
}

// JiraIssueTypeCheck 是选定项目和工作类型后的检查结果
type JiraIssueTypeCheck struct {
	MissingPermissions []string    `json:"missing_permissions"`
	RequiredFields     []JiraField `json:"required_fields"`
}

// IssueTypeCheck 返回账号在该项目缺少的权限，以及该工作类型需要用户补的必填字段
func (a *JiraAPI) IssueTypeCheck(ctx context.Context, projectKey, issueType string) (*JiraIssueTypeCheck, error) {
	res := &JiraIssueTypeCheck{MissingPermissions: []string{}, RequiredFields: []JiraField{}}
	perms, err := a.c.myPermissions(ctx, projectKey)
	if err != nil {
		return nil, err
	}
	for _, k := range jiraPermissionKeys {
		if !perms[k] {
			res.MissingPermissions = append(res.MissingPermissions, k)
		}
	}
	types, err := a.c.listIssueTypes(ctx, projectKey)
	if err != nil {
		return nil, err
	}
	for _, t := range types {
		if strings.EqualFold(t.Name, issueType) || t.ID == issueType {
			fields, err := a.c.listRequiredFields(ctx, projectKey, t.ID)
			if err != nil {
				return nil, err
			}
			res.RequiredFields = fields
			break
		}
	}
	return res, nil
}
