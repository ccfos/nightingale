package router

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/sender/provider"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func jiraRound6Channel(url string) models.NotifyChannelConfig {
	return models.NotifyChannelConfig{
		ID: 1, Name: "jira", Ident: "jira", RequestType: models.RequestTypeJira,
		RequestConfig: &models.RequestConfig{JiraRequestConfig: &models.JiraRequestConfig{
			SiteURL: url, TokenType: models.JiraTokenClassic, Email: "reader@example.invalid", APIToken: "fake-token",
		}},
	}
}

func TestJiraProjectList_ReturnsProjectsFromConfiguredSite(t *testing.T) {
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		_, _ = io.WriteString(w, `{"values":[{"id":"7","key":"OPS","name":"Operations"}],"isLast":true}`)
	}))
	t.Cleanup(server.Close)
	rt := channelBehaviorRouter(t)
	channel := jiraRound6Channel(server.URL)
	require.NoError(t, rt.Ctx.DB.Create(&channel).Error)

	w := channelBehaviorRequest(t, rt.jiraProjectList, nil, "")

	var got []provider.JiraProject
	require.Empty(t, channelBehaviorReply(t, w, &got))
	require.Equal(t, []provider.JiraProject{{ID: "7", Key: "OPS", Name: "Operations"}}, got)
	received := <-requests
	require.Equal(t, http.MethodGet, received.Method)
	require.Equal(t, "/rest/api/3/project/search", received.URL.Path)
	require.Equal(t, "50", received.URL.Query().Get("maxResults"))
	username, password, ok := received.BasicAuth()
	require.True(t, ok)
	require.Equal(t, "reader@example.invalid", username)
	require.Equal(t, "fake-token", password)
}

func TestJiraIssueTypeList_TrimsProjectAndExcludesSubtasks(t *testing.T) {
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Clone(r.Context())
		_, _ = io.WriteString(w, `{"issueTypes":[
   {"id":"1","name":"Bug","subtask":false},{"id":"2","name":"Subtask","subtask":true}
  ],"isLast":true}`)
	}))
	t.Cleanup(server.Close)
	rt := channelBehaviorRouter(t)
	channel := jiraRound6Channel(server.URL)
	require.NoError(t, rt.Ctx.DB.Create(&channel).Error)

	w := channelBehaviorRequest(t, rt.jiraIssueTypeList, nil, "?project=%20OPS%20")

	var got []provider.JiraIssueType
	require.Empty(t, channelBehaviorReply(t, w, &got))
	require.Equal(t, []provider.JiraIssueType{{ID: "1", Name: "Bug"}}, got)
	require.Equal(t, "/rest/api/3/issue/createmeta/OPS/issuetypes", (<-requests).URL.Path)
}

func TestJiraPriorityList_NormalizesEmptyUpstreamList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `null`)
	}))
	t.Cleanup(server.Close)
	rt := channelBehaviorRouter(t)
	channel := jiraRound6Channel(server.URL)
	require.NoError(t, rt.Ctx.DB.Create(&channel).Error)

	w := channelBehaviorRequest(t, rt.jiraPriorityList, nil, "")

	var got []provider.JiraPriority
	require.Empty(t, channelBehaviorReply(t, w, &got))
	require.NotNil(t, got)
	require.Empty(t, got)
}

func TestJiraIssueTypeCheck_ReturnsMissingPermissionsAndRequiredFields(t *testing.T) {
	paths := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		switch r.URL.Path {
		case "/rest/api/3/mypermissions":
			_, _ = io.WriteString(w, `{"permissions":{
    "BROWSE_PROJECTS":{"havePermission":true},"CREATE_ISSUES":{"havePermission":false},
    "ADD_COMMENTS":{"havePermission":true},"TRANSITION_ISSUES":{"havePermission":true}
   }}`)
		case "/rest/api/3/issue/createmeta/OPS/issuetypes":
			_, _ = io.WriteString(w, `{"issueTypes":[{"id":"1","name":"Bug"}],"isLast":true}`)
		default:
			_, _ = io.WriteString(w, `{"fields":[
    {"fieldId":"summary","name":"Summary","required":true},
    {"fieldId":"customfield_1","name":"Service","required":true,"schema":{"type":"string"}},
    {"fieldId":"optional","name":"Optional","required":false}
   ],"isLast":true}`)
		}
	}))
	t.Cleanup(server.Close)
	rt := channelBehaviorRouter(t)
	channel := jiraRound6Channel(server.URL)
	require.NoError(t, rt.Ctx.DB.Create(&channel).Error)

	w := channelBehaviorRequest(t, rt.jiraIssueTypeCheck, nil, "?project=%20OPS%20&issue_type=%20Bug%20")

	var got provider.JiraIssueTypeCheck
	require.Empty(t, channelBehaviorReply(t, w, &got))
	require.Equal(t, []string{"CREATE_ISSUES"}, got.MissingPermissions)
	require.Len(t, got.RequiredFields, 1)
	require.Equal(t, "customfield_1", got.RequiredFields[0].FieldID)
	require.Equal(t, "Service", got.RequiredFields[0].Name)
	require.Equal(t, "/rest/api/3/mypermissions", <-paths)
	require.Equal(t, "/rest/api/3/issue/createmeta/OPS/issuetypes", <-paths)
	require.Equal(t, "/rest/api/3/issue/createmeta/OPS/issuetypes/1", <-paths)
}

func TestJiraReads_RejectMissingWrongTypeAndInvalidConfiguration(t *testing.T) {
	cases := []struct {
		name    string
		channel *models.NotifyChannelConfig
		status  int
		message string
	}{
		{name: "missing", status: 404, message: "notify channel not found"},
		{name: "wrong type", channel: &models.NotifyChannelConfig{ID: 1, RequestType: "http"}, status: 400,
			message: "notify channel is not a jira channel"},
		{name: "missing configuration", channel: &models.NotifyChannelConfig{ID: 1, RequestType: "jira"}, status: 400,
			message: "jira request config not found"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := channelBehaviorRouter(t)
			if tc.channel != nil {
				require.NoError(t, rt.Ctx.DB.Create(tc.channel).Error)
			}

			w := channelBehaviorRequest(t, rt.jiraProjectList, nil, "")

			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
		})
	}
}

func TestJiraReads_RenderUpstreamClientError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"errorMessages":["invalid project"]}`)
	}))
	t.Cleanup(server.Close)
	rt := channelBehaviorRouter(t)
	channel := jiraRound6Channel(server.URL)
	require.NoError(t, rt.Ctx.DB.Create(&channel).Error)

	w := channelBehaviorRequest(t, rt.jiraProjectList, nil, "")

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, channelBehaviorReply(t, w, nil), "invalid project")
}

func TestNotifyChannelConfigCheck_ReturnsCredentialAndPermissionResults(t *testing.T) {
	paths := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		switch r.URL.Path {
		case "/rest/api/3/project/search":
			_, _ = io.WriteString(w, `{"values":[]}`)
		case "/rest/api/3/myself":
			_, _ = io.WriteString(w, `{"displayName":"Reader","emailAddress":"reader@example.invalid"}`)
		case "/rest/api/3/serverInfo":
			_, _ = io.WriteString(w, `{"deploymentType":"Cloud"}`)
		default:
			_, _ = io.WriteString(w, `{"permissions":{
    "BROWSE_PROJECTS":{"havePermission":true},"CREATE_ISSUES":{"havePermission":true},
    "ADD_COMMENTS":{"havePermission":false},"TRANSITION_ISSUES":{"havePermission":true}
   }}`)
		}
	}))
	t.Cleanup(server.Close)
	rt := &Router{}
	channel := jiraRound6Channel(server.URL)

	w := channelBehaviorRequest(t, rt.notifyChannelConfigCheck, NotifyChannelCheckForm{Config: channel}, "")

	var got []provider.CheckItem
	require.Empty(t, channelBehaviorReply(t, w, &got))
	require.Len(t, got, 7)
	results := make(map[string]provider.CheckItem)
	for _, item := range got {
		results[item.Name] = item
	}
	require.True(t, results["Credentials"].OK)
	require.True(t, results["Credentials"].Required)
	require.True(t, results["Account"].OK)
	require.Contains(t, results["Account"].Message, "Reader")
	require.True(t, results["Deployment type"].OK)
	require.True(t, results["Create issues"].OK)
	require.False(t, results["Add comments"].OK)
	require.False(t, results["Add comments"].Required)
	require.Len(t, paths, 4)
}

func TestNotifyChannelConfigCheck_RejectsInvalidAndUnsupportedChecks(t *testing.T) {
	cases := []struct {
		name    string
		config  models.NotifyChannelConfig
		message string
	}{
		{name: "invalid jira", config: models.NotifyChannelConfig{Ident: "jira", RequestType: "jira"}, message: "jira request config"},
		{name: "http has no check", config: validBehaviorChannel(), message: "does not support credential check"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}

			w := channelBehaviorRequest(t, rt.notifyChannelConfigCheck, NotifyChannelCheckForm{Config: tc.config}, "")

			require.Equal(t, http.StatusBadRequest, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
		})
	}
}
