package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	gocache "github.com/patrickmn/go-cache"
)

// jiraGatewayURL 是带权限范围的令牌（含服务账号的令牌）必须经过的 Atlassian 网关，单测里替换成本地地址
var jiraGatewayURL = "https://api.atlassian.com/ex/jira"

// jiraCloudIDCache 缓存 站点地址 -> Cloud ID，Cloud ID 在站点生命周期内不变
var jiraCloudIDCache = gocache.New(24*time.Hour, time.Hour)

// jiraClient 封装一次通知（或一次列表 / 校验请求）用到的 Jira REST 调用。
// 地址与认证方式由媒介配置决定，见 newJiraClient。
type jiraClient struct {
	cfg        models.JiraRequestConfig // 变量引用已展开的副本
	siteURL    string                   // 去掉结尾 / 的站点地址，拼浏览链接用
	baseURL    string                   // .../rest/api/3
	authHeader string
	http       *http.Client
	retryTimes int
	retrySleep time.Duration
}

// newJiraClient 根据媒介配置构造客户端：带权限范围的令牌走网关（需要 Cloud ID），
// 普通令牌直接访问站点地址。Cloud ID 留空时按站点地址自动获取。
func newJiraClient(ctx context.Context, channel *models.NotifyChannelConfig, httpClient *http.Client) (*jiraClient, error) {
	if channel == nil || channel.RequestConfig == nil || channel.RequestConfig.JiraRequestConfig == nil {
		return nil, errors.New("jira request config not found")
	}
	if httpClient == nil {
		return nil, errors.New("http client not found")
	}
	cfg := *channel.RequestConfig.JiraRequestConfig

	var err error
	for _, f := range []*string{&cfg.SiteURL, &cfg.Email, &cfg.APIToken, &cfg.CloudID} {
		if *f, err = expandUserVars(strings.TrimSpace(*f)); err != nil {
			return nil, err
		}
	}
	if cfg.DeploymentType != "" && cfg.DeploymentType != models.JiraDeploymentCloud {
		return nil, fmt.Errorf("jira deployment type %q is not supported yet", cfg.DeploymentType)
	}
	if cfg.TokenType == "" {
		cfg.TokenType = models.JiraTokenScoped
	}

	c := &jiraClient{
		cfg:        cfg,
		siteURL:    strings.TrimRight(cfg.SiteURL, "/"),
		authHeader: "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.Email+":"+cfg.APIToken)),
		http:       httpClient,
	}
	c.retryTimes, c.retrySleep = nativeRetrySettings(&cfg.NativeNetworkConfig)

	if err := c.resolveBaseURL(ctx, cfg.TokenType); err != nil {
		return nil, err
	}
	return c, nil
}

// resolveBaseURL 按令牌类型确定 REST 地址
func (c *jiraClient) resolveBaseURL(ctx context.Context, tokenType string) error {
	if tokenType == models.JiraTokenClassic {
		c.baseURL = c.siteURL + "/rest/api/3"
		return nil
	}
	cloudID := c.cfg.CloudID
	if cloudID == "" {
		var err error
		if cloudID, err = resolveJiraCloudID(ctx, c.http, c.siteURL); err != nil {
			return err
		}
	}
	c.baseURL = strings.TrimRight(jiraGatewayURL, "/") + "/" + url.PathEscape(cloudID) + "/rest/api/3"
	return nil
}

// resolveJiraCloudID 通过站点的 /_edge/tenant_info（免认证）取 Cloud ID，按站点地址缓存。
func resolveJiraCloudID(ctx context.Context, client *http.Client, siteURL string) (string, error) {
	if v, ok := jiraCloudIDCache.Get(siteURL); ok {
		return v.(string), nil
	}
	resp, err := doWithRetry(ctx, client, 1, time.Second,
		func(ctx context.Context) (*http.Request, error) {
			return http.NewRequestWithContext(ctx, http.MethodGet, siteURL+"/_edge/tenant_info", nil)
		},
		func(r *nativeResponse) (bool, error) { return checkStatus(r, nil) })
	if err != nil {
		return "", withHint(fmt.Errorf("failed to get cloud id from %s/_edge/tenant_info: %v", siteURL, err),
			"Check the site URL, or fill in the Cloud ID manually")
	}
	var info struct {
		CloudID string `json:"cloudId"`
	}
	if err := json.Unmarshal(resp.Body, &info); err != nil || info.CloudID == "" {
		return "", withHint(fmt.Errorf("failed to get cloud id from %s/_edge/tenant_info: unexpected response %s", siteURL, truncateBody(resp.Body)),
			"Check the site URL, or fill in the Cloud ID manually")
	}
	jiraCloudIDCache.Set(siteURL, info.CloudID, gocache.DefaultExpiration)
	return info.CloudID, nil
}

// browseURL 返回工单的浏览链接（走网关时也用站点地址，网关地址在浏览器里打不开）
func (c *jiraClient) browseURL(key string) string {
	return c.siteURL + "/browse/" + key
}

// jiraAPIError 解析 Jira 的错误响应：{"errorMessages":[...],"errors":{"field":"msg"}}
func jiraErrorDetail(r *nativeResponse) string {
	var e struct {
		ErrorMessages []string          `json:"errorMessages"`
		Errors        map[string]string `json:"errors"`
		Message       string            `json:"message"`
	}
	if err := json.Unmarshal(r.Body, &e); err != nil {
		return truncateBody(r.Body)
	}
	parts := append([]string{}, e.ErrorMessages...)
	fields := make([]string, 0, len(e.Errors))
	for k := range e.Errors {
		fields = append(fields, k)
	}
	sort.Strings(fields)
	for _, k := range fields {
		parts = append(parts, k+": "+e.Errors[k])
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	}
	if len(parts) == 0 {
		return truncateBody(r.Body)
	}
	return strings.Join(parts, "; ")
}

// jiraHint 把常见状态码翻译成「可能原因 + 怎么办」
func jiraHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "Jira credentials are invalid: check the email, the API token (expires in at most 1 year) and whether the token type matches the token"
	case http.StatusForbidden:
		return "The Jira account lacks permission: it needs Browse projects, Create issues, Add comments and Transition issues in the project"
	case http.StatusNotFound:
		return "The project or issue does not exist, or the Jira account cannot browse it"
	case http.StatusTooManyRequests:
		return "Rate limited by Jira, retried according to Retry-After"
	}
	return ""
}

// do 发一次 Jira REST 请求，body 为 nil 时不带请求体，out 非 nil 时解析响应。
// 返回最终的响应（可能为 nil）以便调用方按状态码分支。
func (c *jiraClient) do(ctx context.Context, method, path string, query url.Values, body, out interface{}) (*nativeResponse, error) {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return nil, err
		}
	}
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	resp, err := doWithRetry(ctx, c.http, c.retryTimes, c.retrySleep,
		func(ctx context.Context) (*http.Request, error) {
			var rd *bytes.Reader
			if payload != nil {
				rd = bytes.NewReader(payload)
			}
			var req *http.Request
			var err error
			if rd != nil {
				req, err = http.NewRequestWithContext(ctx, method, u, rd)
			} else {
				req, err = http.NewRequestWithContext(ctx, method, u, nil)
			}
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", c.authHeader)
			req.Header.Set("Accept", "application/json")
			// 固定英文：状态类别等字段取值、报错原文都以英文为准，JQL 里的 Done 也依赖它
			req.Header.Set("Accept-Language", "en")
			if payload != nil {
				req.Header.Set("Content-Type", "application/json")
			}
			return req, nil
		},
		func(r *nativeResponse) (bool, error) { return checkStatus(r, jiraErrorDetail) })
	if err != nil {
		var se *HTTPStatusError
		if errors.As(err, &se) {
			return resp, withHint(fmt.Errorf("%s %s: %w", method, path, err), jiraHint(se.StatusCode))
		}
		return resp, fmt.Errorf("%s %s: %w", method, path, err)
	}
	if out != nil && len(resp.Body) > 0 {
		if err := json.Unmarshal(resp.Body, out); err != nil {
			return resp, withHint(fmt.Errorf("%s %s: response is not JSON: %s", method, path, truncateBody(resp.Body)),
				"Check the site URL; the request may have been redirected to a login page")
		}
	}
	return resp, nil
}

// jiraIssue 是去重与恢复判断需要的工单字段
type jiraIssue struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	Fields struct {
		Status *struct {
			Name           string `json:"name"`
			StatusCategory struct {
				Key string `json:"key"`
			} `json:"statusCategory"`
		} `json:"status"`
		Resolution *struct {
			Name string `json:"name"`
		} `json:"resolution"`
		ResolutionDate string `json:"resolutiondate"`
	} `json:"fields"`
}

func (i *jiraIssue) done() bool {
	return i != nil && i.Fields.Status != nil && i.Fields.Status.StatusCategory.Key == "done"
}

func (i *jiraIssue) resolution() string {
	if i == nil || i.Fields.Resolution == nil {
		return ""
	}
	return i.Fields.Resolution.Name
}

// resolvedAt 解析 resolutiondate（Jira 的格式形如 2025-12-09T20:29:52.900+0800）
func (i *jiraIssue) resolvedAt() (time.Time, bool) {
	if i == nil || i.Fields.ResolutionDate == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02T15:04:05.000-0700", time.RFC3339Nano, "2006-01-02T15:04:05-0700"} {
		if t, err := time.Parse(layout, i.Fields.ResolutionDate); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

var jiraIssueFields = []string{"status", "resolution", "resolutiondate"}

// searchIssue 按 JQL 查第一条（Cloud 的 /search/jql，最多取 2 条）
func (c *jiraClient) searchIssue(ctx context.Context, jql string) (*jiraIssue, error) {
	var out struct {
		Issues []jiraIssue `json:"issues"`
	}
	body := map[string]interface{}{"jql": jql, "maxResults": 2, "fields": jiraIssueFields}
	if _, err := c.do(ctx, http.MethodPost, "/search/jql", nil, body, &out); err != nil {
		return nil, err
	}
	if len(out.Issues) == 0 {
		return nil, nil
	}
	return &out.Issues[0], nil
}

// getIssue 按 key 取工单实时状态；工单已被删除时返回 nil, nil
func (c *jiraClient) getIssue(ctx context.Context, key string) (*jiraIssue, error) {
	var out jiraIssue
	q := url.Values{"fields": {strings.Join(jiraIssueFields, ",")}}
	resp, err := c.do(ctx, http.MethodGet, "/issue/"+url.PathEscape(key), q, nil, &out)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

// createIssue 建单，返回 issue key
func (c *jiraClient) createIssue(ctx context.Context, fields map[string]interface{}) (string, error) {
	var out struct {
		Key string `json:"key"`
	}
	if _, err := c.do(ctx, http.MethodPost, "/issue", nil, map[string]interface{}{"fields": fields}, &out); err != nil {
		return "", err
	}
	if out.Key == "" {
		return "", errors.New("jira create issue: empty issue key in response")
	}
	return out.Key, nil
}

func (c *jiraClient) addComment(ctx context.Context, key, text string) error {
	_, err := c.do(ctx, http.MethodPost, "/issue/"+url.PathEscape(key)+"/comment", nil,
		map[string]interface{}{"body": jiraADF(text, jiraMaxDescriptionRunes)}, nil)
	return err
}

type jiraTransition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   struct {
		Name           string `json:"name"`
		StatusCategory struct {
			Key string `json:"key"`
		} `json:"statusCategory"`
	} `json:"to"`
}

func (c *jiraClient) listTransitions(ctx context.Context, key string) ([]jiraTransition, error) {
	var out struct {
		Transitions []jiraTransition `json:"transitions"`
	}
	if _, err := c.do(ctx, http.MethodGet, "/issue/"+url.PathEscape(key)+"/transitions", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Transitions, nil
}

func (c *jiraClient) doTransition(ctx context.Context, key, transitionID string, fields map[string]interface{}) (*nativeResponse, error) {
	body := map[string]interface{}{"transition": map[string]string{"id": transitionID}}
	if len(fields) > 0 {
		body["fields"] = fields
	}
	return c.do(ctx, http.MethodPost, "/issue/"+url.PathEscape(key)+"/transitions", nil, body, nil)
}

// JiraProject 是项目下拉的一项
type JiraProject struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

func (c *jiraClient) listProjects(ctx context.Context) ([]JiraProject, error) {
	var all []JiraProject
	for startAt := 0; ; {
		var page struct {
			Values []JiraProject `json:"values"`
			IsLast bool          `json:"isLast"`
			Total  int           `json:"total"`
		}
		q := url.Values{"startAt": {fmt.Sprint(startAt)}, "maxResults": {"50"}, "orderBy": {"name"}}
		if _, err := c.do(ctx, http.MethodGet, "/project/search", q, nil, &page); err != nil {
			return nil, err
		}
		all = append(all, page.Values...)
		startAt += len(page.Values)
		if page.IsLast || len(page.Values) == 0 || startAt >= 5000 {
			return all, nil
		}
	}
}

// JiraIssueType 是工作类型下拉的一项
type JiraIssueType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subtask bool   `json:"subtask"`
}

// listIssueTypes 取项目可建单的工作类型（createmeta）。Cloud 返回 issueTypes，Data Center 返回 values，这里都认。
func (c *jiraClient) listIssueTypes(ctx context.Context, projectKey string) ([]JiraIssueType, error) {
	var all []JiraIssueType
	for startAt := 0; ; {
		var page struct {
			IssueTypes []JiraIssueType `json:"issueTypes"`
			Values     []JiraIssueType `json:"values"`
			Total      int             `json:"total"`
			IsLast     *bool           `json:"isLast"`
		}
		q := url.Values{"startAt": {fmt.Sprint(startAt)}, "maxResults": {"50"}}
		if _, err := c.do(ctx, http.MethodGet, "/issue/createmeta/"+url.PathEscape(projectKey)+"/issuetypes", q, nil, &page); err != nil {
			return nil, err
		}
		items := page.IssueTypes
		if len(items) == 0 {
			items = page.Values
		}
		for _, it := range items {
			if !it.Subtask {
				all = append(all, it)
			}
		}
		startAt += len(items)
		if len(items) == 0 || (page.IsLast != nil && *page.IsLast) || (page.Total > 0 && startAt >= page.Total) || startAt >= 1000 {
			return all, nil
		}
	}
}

// JiraField 是工作类型的一个字段（createmeta）
type JiraField struct {
	FieldID         string `json:"fieldId"`
	Name            string `json:"name"`
	Required        bool   `json:"required"`
	HasDefaultValue bool   `json:"hasDefaultValue"`
}

// listRequiredFields 返回该工作类型建单时必填、且夜莺不会自动填的字段
func (c *jiraClient) listRequiredFields(ctx context.Context, projectKey, issueTypeID string) ([]JiraField, error) {
	var out []JiraField
	for startAt := 0; ; {
		var page struct {
			Fields []JiraField `json:"fields"`
			Values []JiraField `json:"values"`
			Total  int         `json:"total"`
			IsLast *bool       `json:"isLast"`
		}
		q := url.Values{"startAt": {fmt.Sprint(startAt)}, "maxResults": {"100"}}
		path := "/issue/createmeta/" + url.PathEscape(projectKey) + "/issuetypes/" + url.PathEscape(issueTypeID)
		if _, err := c.do(ctx, http.MethodGet, path, q, nil, &page); err != nil {
			return nil, err
		}
		items := page.Fields
		if len(items) == 0 {
			items = page.Values
		}
		for _, f := range items {
			if f.Required && !f.HasDefaultValue && !jiraAutoFilledFields[f.FieldID] {
				out = append(out, f)
			}
		}
		startAt += len(items)
		if len(items) == 0 || (page.IsLast != nil && *page.IsLast) || (page.Total > 0 && startAt >= page.Total) || startAt >= 1000 {
			return out, nil
		}
	}
}

// jiraAutoFilledFields 是建单时夜莺自己会填的字段，不算「需要用户补」的必填项
var jiraAutoFilledFields = map[string]bool{
	"project": true, "issuetype": true, "summary": true, "description": true, "labels": true, "priority": true, "reporter": true,
}

// JiraPriority 是优先级下拉的一项
type JiraPriority struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *jiraClient) listPriorities(ctx context.Context) ([]JiraPriority, error) {
	var out []JiraPriority
	if _, err := c.do(ctx, http.MethodGet, "/priority", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// jiraPermissionKeys 是通知链路用到的项目权限
var jiraPermissionKeys = []string{"BROWSE_PROJECTS", "CREATE_ISSUES", "ADD_COMMENTS", "TRANSITION_ISSUES"}

// myPermissions 返回当前账号的权限（projectKey 为空时查全局口径）
func (c *jiraClient) myPermissions(ctx context.Context, projectKey string) (map[string]bool, error) {
	var out struct {
		Permissions map[string]struct {
			HavePermission bool `json:"havePermission"`
		} `json:"permissions"`
	}
	q := url.Values{"permissions": {strings.Join(jiraPermissionKeys, ",")}}
	if projectKey != "" {
		q.Set("projectKey", projectKey)
	}
	if _, err := c.do(ctx, http.MethodGet, "/mypermissions", q, nil, &out); err != nil {
		return nil, err
	}
	res := make(map[string]bool, len(out.Permissions))
	for k, v := range out.Permissions {
		res[k] = v.HavePermission
	}
	return res, nil
}
