package provider

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/models"
)

// 对接真实第三方服务的冒烟测试，凭证写在 alert/sender/.env.json（已被 .gitignore 忽略），
// 缺哪个服务的凭证就跳过哪组，默认 go test 不会访问外网。
//
//	{
//	  "JiraSiteURL": "https://your-site.atlassian.net",
//	  "JiraEmail": "bot@example.com",
//	  "JiraAPIToken": "...",
//	  "JiraTokenType": "scoped",          // scoped | classic，默认 scoped
//	  "JiraProjectKey": "OPS",
//	  "JiraIssueType": "Task",            // 选填，默认取项目里第一个非子任务类型
//	  "DiscordWebhookURL": "https://discord.com/api/webhooks/<id>/<token>",
//	  "DiscordForumWebhookURL": "...",    // 选填，论坛频道的 Webhook
//	  "JSMAPIKey": "...",                 // JSM 团队里 API 集成的 key
//	  "JSMAPIURL": "https://api.atlassian.com" // 选填
//	}
//
// 访问外网需要代理时设置 HTTPS_PROXY 环境变量。

func liveEnv(t *testing.T, keys ...string) map[string]string {
	t.Helper()
	data, err := readKeyValueFromJsonFile("../.env.json")
	if err != nil {
		t.Skipf("跳过：读取 ../.env.json 失败: %v", err)
	}
	for _, k := range keys {
		if data[k] == "" {
			t.Skipf("跳过：在 .env.json 填写 %s", strings.Join(keys, "、"))
		}
	}
	return data
}

func liveHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, Transport: http.DefaultTransport}
}

func liveEvent(hash string, recovered bool) *models.AlertCurEvent {
	now := time.Now().Unix()
	e := &models.AlertCurEvent{
		Hash: hash, RuleName: "n9e live test", Severity: 2, TriggerTime: now - 60, LastEvalTime: now,
		TriggerValue: "81.5", TagsJSON: []string{"ident=n9e-live-host", "source=provider-live-test"},
	}
	e.IsRecovered = recovered
	return e
}

func TestJiraLive(t *testing.T) {
	env := liveEnv(t, "JiraSiteURL", "JiraEmail", "JiraAPIToken", "JiraProjectKey")
	resetJiraCaches()
	ctx := context.Background()
	tokenType := env["JiraTokenType"]
	if tokenType == "" {
		tokenType = models.JiraTokenScoped
	}
	ch := &models.NotifyChannelConfig{
		ID: time.Now().UnixNano(), Name: "jira-live", Ident: "jira", RequestType: models.RequestTypeJira,
		RequestConfig: &models.RequestConfig{JiraRequestConfig: &models.JiraRequestConfig{
			SiteURL: env["JiraSiteURL"], TokenType: tokenType, Email: env["JiraEmail"], APIToken: env["JiraAPIToken"],
			NativeNetworkConfig: models.NativeNetworkConfig{RetryTimes: 2, RetrySleep: 1000},
		}},
	}
	project := env["JiraProjectKey"]
	p := &JiraProvider{}
	if err := p.Check(ch); err != nil {
		t.Fatalf("check: %v", err)
	}

	t.Run("credential check", func(t *testing.T) {
		for _, it := range p.CheckCredential(ctx, ch, liveHTTPClient()) {
			t.Logf("%-40s ok=%v required=%v skipped=%v %s", it.Name, it.OK, it.Required, it.Skipped, it.Message)
			if it.Required && !it.OK {
				t.Errorf("required check %q failed: %s", it.Name, it.Message)
			}
		}
	})

	api, err := NewJiraClientForChannel(ctx, ch)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	issueType := env["JiraIssueType"]
	t.Run("rule dropdowns", func(t *testing.T) {
		projects, err := api.Projects(ctx)
		if err != nil {
			t.Fatalf("projects: %v", err)
		}
		found := false
		for _, pr := range projects {
			found = found || pr.Key == project
		}
		if !found {
			t.Fatalf("project %s not in %+v", project, projects)
		}
		types, err := api.IssueTypes(ctx, project)
		if err != nil || len(types) == 0 {
			t.Fatalf("issue types: %v %+v", err, types)
		}
		if issueType == "" {
			for _, it := range types {
				if !it.Subtask {
					issueType = it.Name
					break
				}
			}
		}
		chk, err := api.IssueTypeCheck(ctx, project, issueType)
		if err != nil {
			t.Fatalf("issue type check: %v", err)
		}
		t.Logf("issue type %q: missing permissions=%v required fields=%+v", issueType, chk.MissingPermissions, chk.RequiredFields)
		if len(chk.MissingPermissions) > 0 {
			t.Errorf("token lacks permissions %v", chk.MissingPermissions)
		}
		if _, err := api.Priorities(ctx); err != nil {
			t.Errorf("priorities: %v", err)
		}
	})
	if issueType == "" {
		t.Fatal("no issue type available")
	}

	hash := fmt.Sprintf("n9e-live-%d", time.Now().UnixNano())
	params := map[string]string{"project_key": project, "issue_type": issueType}
	req := func(recovered bool) *NotifyRequest {
		return &NotifyRequest{
			Config:       ch,
			Events:       []*models.AlertCurEvent{liveEvent(hash, recovered)},
			TplContent:   map[string]interface{}{"title": "[S2] n9e live test " + hash, "content": "Created by the nightingale provider live test.\n\nSafe to ignore."},
			CustomParams: params,
			HttpClient:   liveHTTPClient(),
		}
	}

	res := p.Notify(ctx, req(false))
	if res.Err != nil {
		t.Fatalf("create: %v", res.Err)
	}
	key := res.Target
	t.Logf("created %s: %s", key, res.Response)

	res = p.Notify(ctx, req(false))
	if res.Err != nil || res.Target != key {
		t.Fatalf("repeat should reuse %s, got target=%s err=%v resp=%s", key, res.Target, res.Err, res.Response)
	}

	// 用一个从没写过的缓存键查，确认按 eventHash 标签用 JQL 能搜回来；不清全局缓存，
	// 搜索索引慢时后面的恢复仍能凭缓存找到工单。Cloud 搜索索引有秒级延迟，重试一会儿。
	t.Run("jql search finds the issue", func(t *testing.T) {
		c, err := newJiraClient(ctx, ch, liveHTTPClient())
		if err != nil {
			t.Fatal(err)
		}
		jp, _ := parseJiraParams(params)
		var issue *jiraIssue
		for i := 0; i < 30 && issue == nil; i++ {
			if i > 0 {
				time.Sleep(2 * time.Second)
			}
			issue, err = p.lookup(ctx, c, jp, fmt.Sprintf("live-search|%s|%d", hash, i), hash, true)
			if err != nil {
				t.Fatalf("lookup: %v", err)
			}
		}
		if issue == nil || issue.Key != key {
			t.Fatalf("jql search did not find %s, got %+v", key, issue)
		}
	})

	res = p.Notify(ctx, req(true))
	if res.Err != nil {
		t.Fatalf("recover: %v", res.Err)
	}
	t.Logf("recovered %s: %s", key, res.Response)
	c, _ := newJiraClient(ctx, ch, liveHTTPClient())
	issue, err := c.getIssue(ctx, key)
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	if !issue.done() {
		t.Errorf("%s should be closed after recovery, status=%+v (check the workflow has a transition to a Done-category status)", key, issue.Fields.Status)
	}
}

func TestDiscordLive(t *testing.T) {
	env := liveEnv(t, "DiscordWebhookURL")
	ctx := context.Background()
	p := &DiscordProvider{}
	ch := &models.NotifyChannelConfig{Name: "Discord", Ident: "discord", RequestType: models.RequestTypeDiscord}
	hash := fmt.Sprintf("n9e-live-%d", time.Now().UnixNano())
	send := func(params map[string]string, recovered bool) *NotifyResult {
		title := "Triggered"
		if recovered {
			title = "Recovered"
		}
		return p.Notify(ctx, &NotifyRequest{
			Config:       ch,
			Events:       []*models.AlertCurEvent{liveEvent(hash, recovered)},
			TplContent:   map[string]interface{}{"content": "**Status**: " + title + "\n**Rule**: n9e live test\nSafe to ignore."},
			CustomParams: params,
			HttpClient:   liveHTTPClient(),
		})
	}

	channel := map[string]string{"webhook_url": env["DiscordWebhookURL"], "bot_name": "live-channel"}
	for _, recovered := range []bool{false, true} {
		res := send(channel, recovered)
		if res.Err != nil {
			t.Fatalf("channel (recovered=%v): %v", recovered, res.Err)
		}
		if strings.Contains(res.Target, "/webhooks/") {
			t.Fatalf("target must not expose the webhook url: %s", res.Target)
		}
		t.Logf("channel recovered=%v: %s", recovered, res.Response)
	}

	bad := map[string]string{"webhook_url": strings.TrimRight(env["DiscordWebhookURL"], "/") + "x"}
	if res := send(bad, false); res.Err == nil {
		t.Errorf("a wrong webhook token should fail")
	} else {
		t.Logf("wrong token: %v", res.Err)
	}

	if forum := env["DiscordForumWebhookURL"]; forum != "" {
		res := send(map[string]string{"webhook_url": forum, "target": "forum_post", "thread_name": "{{$event.RuleName}} " + hash}, false)
		if res.Err != nil {
			t.Fatalf("forum post: %v", res.Err)
		}
		t.Logf("forum post: %s", res.Response)
		if res := send(map[string]string{"webhook_url": forum}, false); res.Err == nil {
			t.Errorf("posting to a forum channel without a thread should fail")
		} else {
			t.Logf("forum without thread: %v", res.Err)
		}
	}
}

func TestJSMAlertLive(t *testing.T) {
	env := liveEnv(t, "JSMAPIKey")
	ctx := context.Background()
	p := &JSMAlertProvider{}
	ch := &models.NotifyChannelConfig{Name: "JSM Alert", Ident: models.JSMAlert, RequestType: models.RequestTypeJSMAlert,
		RequestConfig: &models.RequestConfig{JSMAlertRequestConfig: &models.JSMAlertRequestConfig{APIURL: env["JSMAPIURL"]}}}
	hash := fmt.Sprintf("n9e-live-%d", time.Now().UnixNano())
	send := func(params map[string]string, recovered bool) *NotifyResult {
		return p.Notify(ctx, &NotifyRequest{
			Config:       ch,
			Events:       []*models.AlertCurEvent{liveEvent(hash, recovered)},
			TplContent:   map[string]interface{}{"title": "[S2] n9e live test " + hash, "content": "Created by the nightingale provider live test.\nSafe to ignore."},
			CustomParams: params,
			HttpClient:   liveHTTPClient(),
		})
	}
	// 带上测试 nonce 走「等处理完再返回」的路径，拿到 JSM 异步处理的真实结果
	params := map[string]string{"api_key": env["JSMAPIKey"], "bot_name": "live", TestNonceParam: "live"}
	for _, recovered := range []bool{false, true} {
		res := send(params, recovered)
		if res.Err != nil {
			t.Fatalf("recovered=%v: %v", recovered, res.Err)
		}
		t.Logf("recovered=%v: %s", recovered, res.Response)
	}
	// 生产路径（不带 nonce）只拿到 202
	plain := map[string]string{"api_key": env["JSMAPIKey"]}
	if res := send(plain, false); res.Err != nil || !strings.Contains(res.Response, "accepted") {
		t.Fatalf("production create: %+v", res)
	}
	if res := send(plain, true); res.Err != nil {
		t.Fatalf("production close: %v", res.Err)
	}
	if res := send(map[string]string{"api_key": "00000000-0000-0000-0000-000000000000"}, false); res.Err == nil {
		t.Errorf("a wrong api key should fail")
	} else {
		t.Logf("wrong key: %v", res.Err)
	}
}
