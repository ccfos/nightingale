package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

// fakeJira 是一个最小的 Jira Cloud：记录工单的标签、状态、评论，支持搜索、建单、评论、流转。
type fakeJira struct {
	t  *testing.T
	mu sync.Mutex

	cloudID     string
	issues      map[string]*fakeIssue
	order       []string
	seq         int
	transitions []fakeTransition

	searchDisabled  bool // 模拟 Cloud 搜索的最终一致：刚建的单搜不到
	failStatus      int  // 非 0 时所有 REST 请求返回该状态码
	tooManyOnce     bool // 第一次 REST 请求返回 429
	resolutionError bool // 流转时要求 resolution

	createCount   int
	searchCount   int
	getCount      int
	lastAuth      string
	lastPaths     []string
	lastCreate    map[string]interface{}
	transitionLog []string
}

type fakeIssue struct {
	Key            string
	Labels         []string
	Category       string // new / indeterminate / done
	Resolution     string
	ResolutionDate string
	Comments       []string
}

type fakeTransition struct {
	ID, Name, Category string
}

func newFakeJira(t *testing.T) (*fakeJira, *httptest.Server) {
	fj := &fakeJira{
		t:       t,
		cloudID: "cloud-123",
		issues:  map[string]*fakeIssue{},
		transitions: []fakeTransition{
			{"11", "Start Progress", "indeterminate"},
			{"21", "Close", "done"},
			{"31", "Done", "done"},
			{"41", "Reopen", "new"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(fj.serve))
	t.Cleanup(srv.Close)
	return fj, srv
}

var (
	reLabel = regexp.MustCompile(`labels = "([^"]+)"`)
	reIssue = regexp.MustCompile(`/issue/([A-Z]+-\d+)(/.*)?$`)
)

func (fj *fakeJira) serve(w http.ResponseWriter, r *http.Request) {
	fj.mu.Lock()
	defer fj.mu.Unlock()

	if r.URL.Path == "/_edge/tenant_info" {
		json.NewEncoder(w).Encode(map[string]string{"cloudId": fj.cloudID})
		return
	}
	fj.lastAuth = r.Header.Get("Authorization")
	fj.lastPaths = append(fj.lastPaths, r.Method+" "+r.URL.Path)

	idx := strings.Index(r.URL.Path, "/rest/api/3")
	if idx < 0 {
		http.NotFound(w, r)
		return
	}
	path := r.URL.Path[idx+len("/rest/api/3"):]

	if fj.tooManyOnce {
		fj.tooManyOnce = false
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		return
	}
	if fj.failStatus != 0 {
		w.WriteHeader(fj.failStatus)
		json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"boom"}})
		return
	}

	body, _ := io.ReadAll(r.Body)
	switch {
	case r.Method == http.MethodPost && path == "/search/jql":
		fj.searchCount++
		var req struct {
			JQL string `json:"jql"`
		}
		json.Unmarshal(body, &req)
		issues := []interface{}{}
		if !fj.searchDisabled {
			m := reLabel.FindStringSubmatch(req.JQL)
			openOnly := strings.Contains(req.JQL, "statusCategory != Done")
			for i := len(fj.order) - 1; i >= 0; i-- {
				is := fj.issues[fj.order[i]]
				if m == nil || !contains(is.Labels, m[1]) {
					continue
				}
				if openOnly && is.Category == "done" {
					continue
				}
				issues = append(issues, fj.issueJSON(is))
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"issues": issues})
	case r.Method == http.MethodPost && path == "/issue":
		fj.createCount++
		var req struct {
			Fields map[string]interface{} `json:"fields"`
		}
		json.Unmarshal(body, &req)
		fj.lastCreate = req.Fields
		fj.seq++
		key := fmt.Sprintf("OPS-%d", fj.seq)
		var labels []string
		for _, l := range req.Fields["labels"].([]interface{}) {
			labels = append(labels, l.(string))
		}
		fj.issues[key] = &fakeIssue{Key: key, Labels: labels, Category: "new"}
		fj.order = append(fj.order, key)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"key": key, "id": "1000"})
	default:
		m := reIssue.FindStringSubmatch(path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		is := fj.issues[m[1]]
		if is == nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{"errorMessages": []string{"Issue does not exist"}})
			return
		}
		switch {
		case r.Method == http.MethodGet && m[2] == "":
			fj.getCount++
			json.NewEncoder(w).Encode(fj.issueJSON(is))
		case r.Method == http.MethodPost && m[2] == "/comment":
			is.Comments = append(is.Comments, string(body))
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":"1"}`))
		case r.Method == http.MethodGet && m[2] == "/transitions":
			var ts []interface{}
			for _, t := range fj.transitions {
				ts = append(ts, map[string]interface{}{"id": t.ID, "name": t.Name,
					"to": map[string]interface{}{"name": t.Name, "statusCategory": map[string]string{"key": t.Category}}})
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"transitions": ts})
		case r.Method == http.MethodPost && m[2] == "/transitions":
			var req struct {
				Transition struct {
					ID string `json:"id"`
				} `json:"transition"`
				Fields map[string]interface{} `json:"fields"`
			}
			json.Unmarshal(body, &req)
			if fj.resolutionError && req.Fields["resolution"] == nil {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(`{"errorMessages":[],"errors":{"resolution":"Resolution is required."}}`))
				return
			}
			for _, t := range fj.transitions {
				if t.ID == req.Transition.ID {
					fj.transitionLog = append(fj.transitionLog, t.Name)
					is.Category = t.Category
					if t.Category == "done" {
						is.Resolution = "Done"
						is.ResolutionDate = time.Now().Format("2006-01-02T15:04:05.000-0700")
					} else {
						is.Resolution, is.ResolutionDate = "", ""
					}
				}
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}
}

func (fj *fakeJira) issueJSON(is *fakeIssue) map[string]interface{} {
	fields := map[string]interface{}{
		"status": map[string]interface{}{"name": is.Category, "statusCategory": map[string]string{"key": is.Category}},
	}
	if is.Resolution != "" {
		fields["resolution"] = map[string]string{"name": is.Resolution}
		fields["resolutiondate"] = is.ResolutionDate
	}
	return map[string]interface{}{"id": "1", "key": is.Key, "fields": fields}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

func resetJiraCaches() {
	jiraIssueCache.Flush()
	jiraRecoverTombstones.Flush()
	jiraCloudIDCache.Flush()
}

func jiraTestChannel(id int64, site, tokenType string) *models.NotifyChannelConfig {
	return &models.NotifyChannelConfig{
		ID: id, Name: "jira", Ident: "jira", RequestType: models.RequestTypeJira,
		RequestConfig: &models.RequestConfig{JiraRequestConfig: &models.JiraRequestConfig{
			SiteURL: site, TokenType: tokenType, Email: "bot@example.com", APIToken: "tok",
			NativeNetworkConfig: models.NativeNetworkConfig{RetryTimes: 2, RetrySleep: 1},
		}},
	}
}

func jiraReq(ch *models.NotifyChannelConfig, params map[string]string, ev *models.AlertCurEvent) *NotifyRequest {
	return &NotifyRequest{
		Config:       ch,
		Events:       []*models.AlertCurEvent{ev},
		TplContent:   map[string]interface{}{"title": "[S2] cpu high", "content": "line1\n\nsee https://n9e.example.com/e/1 now"},
		CustomParams: params,
		HttpClient:   &http.Client{Timeout: 5 * time.Second},
	}
}

func firingEvent(hash string, triggerTime int64) *models.AlertCurEvent {
	return &models.AlertCurEvent{Hash: hash, RuleName: "cpu high", Severity: 2, TriggerTime: triggerTime, LastEvalTime: triggerTime,
		TagsJSON: []string{"ident=host 01", "service=api"}}
}

func recoveredEvent(hash string, at int64) *models.AlertCurEvent {
	e := firingEvent(hash, at-60)
	e.IsRecovered = true
	e.LastEvalTime = at
	return e
}

func setupJira(t *testing.T) (*fakeJira, *models.NotifyChannelConfig) {
	t.Helper()
	resetJiraCaches()
	fj, srv := newFakeJira(t)
	old := jiraGatewayURL
	jiraGatewayURL = srv.URL + "/ex/jira"
	t.Cleanup(func() { jiraGatewayURL = old })
	return fj, jiraTestChannel(time.Now().UnixNano(), srv.URL, models.JiraTokenScoped)
}

var baseParams = map[string]string{"project_key": "OPS", "issue_type": "Bug"}

func withParams(extra map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range baseParams {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func TestJiraProviderCheck(t *testing.T) {
	p := &JiraProvider{}
	ch := jiraTestChannel(1, "https://x.atlassian.net", "")
	if err := p.Check(ch); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	legacy := &models.NotifyChannelConfig{Ident: "jira", RequestType: "http",
		RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: "https://x", Method: "POST"}}}
	if err := p.Check(legacy); err == nil {
		t.Fatal("legacy http jira record must not pass the native check")
	}
	// 旧版 request_type=http 的 jira 记录兜底到 callback，发送行为不变
	got, ok := DefaultRegistry.Resolve(legacy)
	if !ok || got.Ident() != "callback" {
		t.Fatalf("legacy jira record should fall back to callback, got %v %v", got, ok)
	}
	if got, ok := DefaultRegistry.Resolve(ch); !ok || got.Ident() != "jira" {
		t.Fatalf("native jira record should resolve to jira provider, got %v", got)
	}
	bad := jiraTestChannel(1, "not a url", "")
	if err := p.Check(bad); err == nil {
		t.Fatal("invalid site url should be rejected")
	}
	varRef := jiraTestChannel(1, "{{.jira_site}}", "")
	if err := p.Check(varRef); err != nil {
		t.Fatalf("variable reference should skip format check: %v", err)
	}
}

func TestJiraScopedTokenGoesThroughGateway(t *testing.T) {
	fj, ch := setupJira(t)
	res := (&JiraProvider{}).Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 100)))
	if res.Err != nil {
		t.Fatalf("notify: %v", res.Err)
	}
	if fj.createCount != 1 {
		t.Fatalf("want 1 create, got %d", fj.createCount)
	}
	want := "/ex/jira/cloud-123/rest/api/3/issue"
	if !contains(fj.lastPaths, "POST "+want) {
		t.Fatalf("scoped token should use gateway path %s, got %v", want, fj.lastPaths)
	}
	if fj.lastAuth != "Basic "+base64.StdEncoding.EncodeToString([]byte("bot@example.com:tok")) {
		t.Fatalf("unexpected auth header %q", fj.lastAuth)
	}
	if res.Target != "OPS-1" || !strings.Contains(res.Response, "created OPS-1") || !strings.Contains(res.Response, "/browse/OPS-1") {
		t.Fatalf("unexpected result %+v", res)
	}
	// 浏览链接用站点地址，不用网关地址
	if strings.Contains(res.Response, "/ex/jira/") {
		t.Fatalf("browse url must use the site url: %s", res.Response)
	}
}

func TestJiraClassicTokenUsesSiteURL(t *testing.T) {
	fj, ch := setupJira(t)
	ch.RequestConfig.JiraRequestConfig.TokenType = models.JiraTokenClassic
	res := (&JiraProvider{}).Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 100)))
	if res.Err != nil {
		t.Fatalf("notify: %v", res.Err)
	}
	if !contains(fj.lastPaths, "POST /rest/api/3/issue") {
		t.Fatalf("classic token should hit site url directly, got %v", fj.lastPaths)
	}
}

func TestJiraCreatePayload(t *testing.T) {
	fj, ch := setupJira(t)
	params := withParams(map[string]string{
		"priority_map":   `{"2":"High"}`,
		"labels":         `["n9e","team a"]`,
		"tags_as_labels": "true",
		"fields":         `{"customfield_1":"{\"value\":\"prod\"}","customfield_2":"plain text"}`,
	})
	res := (&JiraProvider{}).Notify(context.Background(), jiraReq(ch, params, firingEvent("h1", 100)))
	if res.Err != nil {
		t.Fatalf("notify: %v", res.Err)
	}
	f := fj.lastCreate
	if f["summary"] != "[S2] cpu high" {
		t.Fatalf("summary: %v", f["summary"])
	}
	if f["priority"].(map[string]interface{})["name"] != "High" {
		t.Fatalf("priority: %v", f["priority"])
	}
	labels := fmt.Sprint(f["labels"])
	for _, want := range []string{"eventHash=h1", "n9e", "team_a", "ident=host_01", "service=api"} {
		if !strings.Contains(labels, want) {
			t.Fatalf("labels %s missing %s", labels, want)
		}
	}
	if _, ok := f["customfield_1"].(map[string]interface{}); !ok {
		t.Fatalf("json custom field should be sent as json, got %T", f["customfield_1"])
	}
	if f["customfield_2"] != "plain text" {
		t.Fatalf("text custom field: %v", f["customfield_2"])
	}
	desc, _ := json.Marshal(f["description"])
	if !strings.Contains(string(desc), `"type":"doc"`) || !strings.Contains(string(desc), `"href":"https://n9e.example.com/e/1"`) {
		t.Fatalf("description should be ADF with link marks: %s", desc)
	}
}

func TestJiraRepeatFiringDoesNotCreateAgain(t *testing.T) {
	fj, ch := setupJira(t)
	p := &JiraProvider{}
	p.Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 100)))
	res := p.Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 100)))
	if res.Err != nil || fj.createCount != 1 {
		t.Fatalf("repeat firing should not create again: err=%v creates=%d", res.Err, fj.createCount)
	}
	if !strings.Contains(res.Response, "exists") || res.Target != "OPS-1" {
		t.Fatalf("unexpected response %+v", res)
	}
	if n := len(fj.issues["OPS-1"].Comments); n != 0 {
		t.Fatalf("default on_repeat must not comment, got %d comments", n)
	}
	// on_repeat=comment 时追加评论
	res = p.Notify(context.Background(), jiraReq(ch, withParams(map[string]string{"on_repeat": "comment"}), firingEvent("h1", 100)))
	if res.Err != nil || len(fj.issues["OPS-1"].Comments) != 1 {
		t.Fatalf("on_repeat=comment should comment once: %v %d", res.Err, len(fj.issues["OPS-1"].Comments))
	}
}

func TestJiraCacheCoversSearchLag(t *testing.T) {
	fj, ch := setupJira(t)
	p := &JiraProvider{}
	fj.searchDisabled = true // Cloud 搜索最终一致：刚建的单搜不到
	p.Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 100)))
	searches := fj.searchCount
	res := p.Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 100)))
	if res.Err != nil || fj.createCount != 1 {
		t.Fatalf("cache hit should prevent duplicate create: err=%v creates=%d", res.Err, fj.createCount)
	}
	if fj.searchCount != searches || fj.getCount == 0 {
		t.Fatalf("cache hit should read the issue by key instead of searching: searches %d->%d gets %d", searches, fj.searchCount, fj.getCount)
	}
}

func TestJiraRecoveryCommentsAndClosesAutomatically(t *testing.T) {
	fj, ch := setupJira(t)
	p := &JiraProvider{}
	p.Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 100)))
	res := p.Notify(context.Background(), jiraReq(ch, baseParams, recoveredEvent("h1", 200)))
	if res.Err != nil {
		t.Fatalf("recovery: %v", res.Err)
	}
	is := fj.issues["OPS-1"]
	if len(is.Comments) != 1 || is.Category != "done" {
		t.Fatalf("recovery should comment and close: comments=%d category=%s", len(is.Comments), is.Category)
	}
	// 多个能到 done 的动作时按偏好选 Done
	if fj.transitionLog[len(fj.transitionLog)-1] != "Done" {
		t.Fatalf("should prefer the Done transition, got %v", fj.transitionLog)
	}
	if !strings.Contains(res.Response, "closed") {
		t.Fatalf("unexpected response %s", res.Response)
	}
	// 再来一次恢复：工单已关闭，正常结局
	res = p.Notify(context.Background(), jiraReq(ch, baseParams, recoveredEvent("h1", 300)))
	if res.Err != nil {
		t.Fatalf("second recovery should not fail: %v", res.Err)
	}
}

func TestJiraRecoveryOnlyComment(t *testing.T) {
	fj, ch := setupJira(t)
	p := &JiraProvider{}
	params := withParams(map[string]string{"on_resolve": "comment"})
	p.Notify(context.Background(), jiraReq(ch, params, firingEvent("h1", 100)))
	res := p.Notify(context.Background(), jiraReq(ch, params, recoveredEvent("h1", 200)))
	is := fj.issues["OPS-1"]
	if res.Err != nil || len(is.Comments) != 1 || is.Category == "done" {
		t.Fatalf("on_resolve=comment should only comment: err=%v comments=%d category=%s", res.Err, len(is.Comments), is.Category)
	}
}

func TestJiraCloseFailureIsNotSendFailure(t *testing.T) {
	fj, ch := setupJira(t)
	p := &JiraProvider{}
	params := withParams(map[string]string{"resolve_transition": "No Such Transition"})
	p.Notify(context.Background(), jiraReq(ch, params, firingEvent("h1", 100)))
	res := p.Notify(context.Background(), jiraReq(ch, params, recoveredEvent("h1", 200)))
	if res.Err != nil {
		t.Fatalf("close failure must not fail the notification: %v", res.Err)
	}
	if len(fj.issues["OPS-1"].Comments) != 1 || !strings.Contains(res.Response, "failed to close") || !strings.Contains(res.Response, "available: Start Progress, Close, Done, Reopen") {
		t.Fatalf("response should explain the close failure: %s", res.Response)
	}
}

func TestJiraResolutionRequiredRetry(t *testing.T) {
	fj, ch := setupJira(t)
	fj.resolutionError = true
	p := &JiraProvider{}
	p.Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 100)))
	res := p.Notify(context.Background(), jiraReq(ch, baseParams, recoveredEvent("h1", 200)))
	if res.Err != nil || fj.issues["OPS-1"].Category != "done" {
		t.Fatalf("should retry the transition with a resolution: err=%v category=%s", res.Err, fj.issues["OPS-1"].Category)
	}
}

func TestJiraRecoveryWithoutIssueIsNotAFailure(t *testing.T) {
	fj, ch := setupJira(t)
	p := &JiraProvider{}
	res := p.Notify(context.Background(), jiraReq(ch, baseParams, recoveredEvent("h1", 200)))
	if res.Err != nil || !strings.Contains(res.Response, "no matching open issue") {
		t.Fatalf("recovery without issue should be a normal outcome: %+v", res)
	}
	// 乱序：更早的触发晚到，不再建出一张永远不会被关的单
	res = p.Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 150)))
	if res.Err != nil || fj.createCount != 0 || !strings.Contains(res.Response, "skipped") {
		t.Fatalf("stale trigger after recovery should be skipped: %+v creates=%d", res, fj.createCount)
	}
	// 恢复之后的新一轮触发正常建单
	res = p.Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 250)))
	if res.Err != nil || fj.createCount != 1 {
		t.Fatalf("new trigger after recovery should create: %+v creates=%d", res, fj.createCount)
	}
}

func TestJiraReopenWithinWindow(t *testing.T) {
	fj, ch := setupJira(t)
	p := &JiraProvider{}
	params := withParams(map[string]string{"reopen_transition": "reopen", "reopen_duration": "60"})
	p.Notify(context.Background(), jiraReq(ch, params, firingEvent("h1", 100)))
	p.Notify(context.Background(), jiraReq(ch, params, recoveredEvent("h1", 200)))
	res := p.Notify(context.Background(), jiraReq(ch, params, firingEvent("h1", 300)))
	if res.Err != nil || fj.createCount != 1 || fj.issues["OPS-1"].Category != "new" || !strings.Contains(res.Response, "reopened") {
		t.Fatalf("firing within the reopen window should reopen: %+v creates=%d category=%s", res, fj.createCount, fj.issues["OPS-1"].Category)
	}
}

func TestJiraClosedWithoutReopenCreatesNew(t *testing.T) {
	fj, ch := setupJira(t)
	p := &JiraProvider{}
	p.Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 100)))
	p.Notify(context.Background(), jiraReq(ch, baseParams, recoveredEvent("h1", 200)))
	res := p.Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 300)))
	if res.Err != nil || fj.createCount != 2 || res.Target != "OPS-2" {
		t.Fatalf("firing after close without reopen should create a new issue: %+v creates=%d", res, fj.createCount)
	}
}

func TestJiraWontFixIsNotReopened(t *testing.T) {
	fj, ch := setupJira(t)
	p := &JiraProvider{}
	params := withParams(map[string]string{"reopen_transition": "Reopen", "wont_fix_resolution": "Done"})
	p.Notify(context.Background(), jiraReq(ch, params, firingEvent("h1", 100)))
	p.Notify(context.Background(), jiraReq(ch, params, recoveredEvent("h1", 200)))
	res := p.Notify(context.Background(), jiraReq(ch, params, firingEvent("h1", 300)))
	if res.Err != nil || fj.createCount != 2 {
		t.Fatalf("issue resolved as the ignored resolution should not be reopened: %+v creates=%d", res, fj.createCount)
	}
}

func TestJiraTestNonceIsolatesTestSends(t *testing.T) {
	fj, ch := setupJira(t)
	p := &JiraProvider{}
	n1 := withParams(map[string]string{TestNonceParam: "a1"})
	n2 := withParams(map[string]string{TestNonceParam: "b2"})
	p.Notify(context.Background(), jiraReq(ch, n1, firingEvent("mock", 100)))
	p.Notify(context.Background(), jiraReq(ch, n2, firingEvent("mock", 100)))
	if fj.createCount != 2 {
		t.Fatalf("each test send should create its own issue, got %d", fj.createCount)
	}
	// 同一个 nonce 的「测试恢复」关掉的是本次测试建的那张单
	res := p.Notify(context.Background(), jiraReq(ch, n2, recoveredEvent("mock", 200)))
	if res.Err != nil || fj.issues["OPS-2"].Category != "done" || fj.issues["OPS-1"].Category == "done" {
		t.Fatalf("recovery with the same nonce should close only its own issue: %+v", res)
	}
	if !contains(fj.issues["OPS-2"].Labels, "eventHash=mock-b2") {
		t.Fatalf("dedup label should carry the nonce: %v", fj.issues["OPS-2"].Labels)
	}
}

func TestJiraRetriesOn429(t *testing.T) {
	fj, ch := setupJira(t)
	fj.tooManyOnce = true
	res := (&JiraProvider{}).Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 100)))
	if res.Err != nil || fj.createCount != 1 {
		t.Fatalf("429 should be retried: err=%v creates=%d", res.Err, fj.createCount)
	}
}

func TestJiraAuthErrorCarriesLocalizableHint(t *testing.T) {
	fj, ch := setupJira(t)
	fj.failStatus = http.StatusUnauthorized
	res := (&JiraProvider{}).Notify(context.Background(), jiraReq(ch, baseParams, firingEvent("h1", 100)))
	if res.Err == nil {
		t.Fatal("401 should fail")
	}
	var he *HintError
	if !errors.As(res.Err, &he) || !strings.Contains(he.Hint, "credentials are invalid") {
		t.Fatalf("401 should carry a hint, got %v", res.Err)
	}
	msg := LocalizeError(res.Err, func(k string) string { return "TRANSLATED" })
	if !strings.Contains(msg, "(TRANSLATED)") || !strings.Contains(msg, "boom") {
		t.Fatalf("localized message should keep the raw error and translate the hint: %s", msg)
	}
}

func TestJiraMissingParams(t *testing.T) {
	_, ch := setupJira(t)
	res := (&JiraProvider{}).Notify(context.Background(), jiraReq(ch, map[string]string{"project_key": "OPS"}, firingEvent("h1", 100)))
	if res.Err == nil || !strings.Contains(res.Err.Error(), "issue_type") {
		t.Fatalf("missing issue_type should fail clearly, got %v", res.Err)
	}
}

func TestJiraSummaryFallbackAndTruncate(t *testing.T) {
	ev := firingEvent("h", 1)
	ev.TargetIdent = "host-1"
	if got := jiraSummary(map[string]interface{}{"content": "x"}, ev); got != "[S2] cpu high · host-1" {
		t.Fatalf("fallback summary: %q", got)
	}
	long := strings.Repeat("长", 300)
	if got := jiraSummary(map[string]interface{}{"title": long + "\nline2"}, ev); len([]rune(got)) != jiraMaxSummaryRunes || strings.Contains(got, "\n") {
		t.Fatalf("summary must be single-line and truncated to %d runes, got %d", jiraMaxSummaryRunes, len([]rune(got)))
	}
}

func TestJiraJQL(t *testing.T) {
	jp := &jiraParams{ProjectKey: "OPS", ReopenDuration: time.Hour}
	if got := buildJiraJQL(jp, "h1", true); got != `statusCategory != Done AND project = "OPS" AND labels = "eventHash=h1" ORDER BY status ASC, resolutiondate DESC` {
		t.Fatalf("firing jql: %s", got)
	}
	jp.ReopenTransition, jp.WontFixResolution = "Reopen", `Won't "Fix"`
	got := buildJiraJQL(jp, "h1", true)
	if !strings.HasPrefix(got, `(resolution is EMPTY OR resolution != "Won't \"Fix\"") AND (resolutiondate is EMPTY OR resolutiondate >= -60m)`) {
		t.Fatalf("reopen jql: %s", got)
	}
	if got := buildJiraJQL(jp, "h1", false); !strings.Contains(got, "statusCategory != Done") {
		t.Fatalf("recovery jql must look for open issues only: %s", got)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if d, ok := parseRetryAfter("3", now); !ok || d != 3*time.Second {
		t.Fatalf("seconds: %v %v", d, ok)
	}
	if d, ok := parseRetryAfter(now.Add(5*time.Second).Format(http.TimeFormat), now); !ok || d != 5*time.Second {
		t.Fatalf("http date: %v %v", d, ok)
	}
	if _, ok := parseRetryAfter("soon", now); ok {
		t.Fatal("garbage should not parse")
	}
}

func TestExpandUserVars(t *testing.T) {
	old := UserVariableGetter
	UserVariableGetter = func() map[string]string { return map[string]string{"jira_token": "s3cr3t+/="} }
	t.Cleanup(func() { UserVariableGetter = old })
	if v, err := expandUserVars("{{.jira_token}}"); err != nil || v != "s3cr3t+/=" {
		t.Fatalf("expand: %q %v", v, err)
	}
	if _, err := expandUserVars("{{.missing}}"); err == nil {
		t.Fatal("undefined variable should fail instead of sending an empty credential")
	}
	if v, _ := expandUserVars("plain"); v != "plain" {
		t.Fatalf("plain value changed: %q", v)
	}
}

func TestJiraCredentialCheckSuggestsTokenType(t *testing.T) {
	resetJiraCaches()
	var gatewayCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/_edge/tenant_info":
			w.Write([]byte(`{"cloudId":"c1"}`))
		case strings.HasPrefix(r.URL.Path, "/ex/jira/"):
			gatewayCalls++
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/rest/api/3/project/search":
			w.Write([]byte(`{"values":[],"isLast":true}`))
		case r.URL.Path == "/rest/api/3/myself":
			w.Write([]byte(`{"displayName":"Bot","emailAddress":"bot@example.com"}`))
		case r.URL.Path == "/rest/api/3/serverInfo":
			w.Write([]byte(`{"deploymentType":"Cloud"}`))
		case r.URL.Path == "/rest/api/3/mypermissions":
			w.Write([]byte(`{"permissions":{"BROWSE_PROJECTS":{"havePermission":true},"CREATE_ISSUES":{"havePermission":true},"ADD_COMMENTS":{"havePermission":false},"TRANSITION_ISSUES":{"havePermission":true}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := jiraGatewayURL
	jiraGatewayURL = srv.URL + "/ex/jira"
	defer func() { jiraGatewayURL = old }()

	ch := jiraTestChannel(1, srv.URL, models.JiraTokenScoped)
	items := (&JiraProvider{}).CheckCredential(context.Background(), ch, &http.Client{Timeout: 5 * time.Second})
	var cred CheckItem
	for _, it := range items {
		if it.Name == "Credentials" {
			cred = it
		}
	}
	if cred.OK || !strings.Contains(cred.Message, "choose Classic API token") {
		t.Fatalf("scoped token rejected by gateway but accepted by site should suggest classic: %+v", cred)
	}

	ch.RequestConfig.JiraRequestConfig.TokenType = models.JiraTokenClassic
	items = (&JiraProvider{}).CheckCredential(context.Background(), ch, &http.Client{Timeout: 5 * time.Second})
	got := map[string]CheckItem{}
	for _, it := range items {
		got[it.Name] = it
	}
	if !got["Credentials"].OK || !got["Account"].OK || !got["Deployment type"].OK || !got["Create issues"].OK || got["Add comments"].OK || got["Add comments"].Required {
		t.Fatalf("unexpected check items: %+v", items)
	}
}
