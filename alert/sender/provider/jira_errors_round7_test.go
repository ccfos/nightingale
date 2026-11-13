package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func jiraRound7Client(t *testing.T, respond func(int, capturedRequest) (int, string)) (*jiraClient, func() []capturedRequest) {
	t.Helper()
	server, requests := newCaptureServer(t, respond)
	client, err := newJiraClient(context.Background(), jiraTestChannel(77, server.URL, models.JiraTokenClassic), server.Client())
	require.NoError(t, err)
	return client, requests
}

func TestJiraListRequiredFields_FiltersDefaultsAndAutomaticallyFilledFieldsAcrossPages(t *testing.T) {
	client, requests := jiraRound7Client(t, func(n int, r capturedRequest) (int, string) {
		if n == 1 {
			return http.StatusOK, `{"values":[
				{"fieldId":"summary","required":true},
				{"fieldId":"customfield_1","name":"Environment","required":true},
				{"fieldId":"customfield_2","required":true,"hasDefaultValue":true},
				{"fieldId":"customfield_3","required":false}
			],"total":5}`
		}
		return http.StatusOK, `{"fields":[{"fieldId":"customfield_4","name":"Team","required":true}],"total":5}`
	})

	fields, err := client.listRequiredFields(context.Background(), "OPS", "10")

	require.NoError(t, err)
	require.Equal(t, []JiraField{
		{FieldID: "customfield_1", Name: "Environment", Required: true},
		{FieldID: "customfield_4", Name: "Team", Required: true},
	}, fields)
	require.Len(t, requests(), 2)
	require.Equal(t, "0", requests()[0].Query.Get("startAt"))
	require.Equal(t, "4", requests()[1].Query.Get("startAt"))
	require.Equal(t, "/rest/api/3/issue/createmeta/OPS/issuetypes/10", requests()[1].Path)
}

func TestJiraGetIssue_DeletedIssueReturnsNoMatch(t *testing.T) {
	client, requests := jiraRound7Client(t, always(http.StatusNotFound, `{"errorMessages":["Issue deleted"]}`))

	issue, err := client.getIssue(context.Background(), "OPS-7")

	require.NoError(t, err)
	require.Nil(t, issue)
	require.Len(t, requests(), 1)
	require.Equal(t, "/rest/api/3/issue/OPS-7", requests()[0].Path)
	require.Equal(t, "status", requests()[0].Query.Get("fields"))
}

func TestJiraCreateIssue_RejectsSuccessResponseWithoutIssueKey(t *testing.T) {
	client, requests := jiraRound7Client(t, always(http.StatusCreated, `{"id":"7"}`))

	key, err := client.createIssue(context.Background(), map[string]interface{}{"summary": "CPU high"})

	require.Empty(t, key)
	require.ErrorContains(t, err, "empty issue key")
	require.Len(t, requests(), 1)
	require.JSONEq(t, `{"fields":{"summary":"CPU high"}}`, string(requests()[0].Body))
}

func TestJiraDo_NonJSONResponseSuggestsCheckingRedirects(t *testing.T) {
	client, requests := jiraRound7Client(t, always(http.StatusOK, `<html>Sign in</html>`))
	var output map[string]interface{}

	response, err := client.do(context.Background(), http.MethodGet, "/myself", nil, nil, &output)

	require.ErrorContains(t, err, "response is not JSON")
	require.ErrorContains(t, err, "redirected to a login page")
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Nil(t, output)
	require.Equal(t, "en", requests()[0].Header.Get("Accept-Language"))
	require.Equal(t, "application/json", requests()[0].Header.Get("Accept"))
}

func TestJiraDo_UnserializableBodyDoesNotSendRequest(t *testing.T) {
	client, requests := jiraRound7Client(t, always(http.StatusOK, `{}`))

	response, err := client.do(context.Background(), http.MethodPost, "/issue", nil, make(chan int), nil)

	require.ErrorContains(t, err, "unsupported type")
	require.Nil(t, response)
	require.Empty(t, requests())
}

func TestJiraListQueries_PropagatePermissionErrors(t *testing.T) {
	tests := []struct {
		name string
		call func(*jiraClient) error
	}{
		{name: "projects", call: func(c *jiraClient) error { _, err := c.listProjects(context.Background()); return err }},
		{
			name: "issue types",
			call: func(c *jiraClient) error { _, err := c.listIssueTypes(context.Background(), "OPS"); return err },
		},
		{
			name: "required fields",
			call: func(c *jiraClient) error {
				_, err := c.listRequiredFields(context.Background(), "OPS", "10")
				return err
			},
		},
		{
			name: "priorities",
			call: func(c *jiraClient) error { _, err := c.listPriorities(context.Background()); return err },
		},
		{
			name: "permissions",
			call: func(c *jiraClient) error { _, err := c.myPermissions(context.Background(), "OPS"); return err },
		},
		{
			name: "transitions",
			call: func(c *jiraClient) error { _, err := c.listTransitions(context.Background(), "OPS-7"); return err },
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, requests := jiraRound7Client(t, always(http.StatusForbidden, `{"message":"project denied"}`))

			err := tc.call(client)

			require.ErrorContains(t, err, "project denied")
			require.ErrorContains(t, err, "lacks permission")
			var status *HTTPStatusError
			require.ErrorAs(t, err, &status)
			require.Equal(t, http.StatusForbidden, status.StatusCode)
			require.Len(t, requests(), 1)
		})
	}
}

func TestResolveJiraCloudID_RejectsUnexpectedTenantResponse(t *testing.T) {
	for _, body := range []string{`{"cloudId":""}`, `not JSON`} {
		t.Run(body, func(t *testing.T) {
			server, requests := newCaptureServer(t, always(http.StatusOK, body))
			t.Cleanup(func() { jiraCloudIDCache.Delete(server.URL) })

			cloudID, err := resolveJiraCloudID(context.Background(), server.Client(), server.URL)

			require.Empty(t, cloudID)
			require.ErrorContains(t, err, "unexpected response")
			require.ErrorContains(t, err, "fill in the Cloud ID manually")
			require.Equal(t, "/_edge/tenant_info", requests()[0].Path)
			_, cached := jiraCloudIDCache.Get(server.URL)
			require.False(t, cached)
		})
	}
}

func TestNewJiraClient_RejectsUnsupportedDeployment(t *testing.T) {
	channel := jiraTestChannel(77, "http://127.0.0.1", models.JiraTokenClassic)
	channel.RequestConfig.JiraRequestConfig.DeploymentType = "server"

	client, err := newJiraClient(context.Background(), channel, &http.Client{})

	require.Nil(t, client)
	require.ErrorContains(t, err, `deployment type "server" is not supported`)
}

func TestLocalizeCheckItems_TranslatesHintsAndPreservesOriginalItems(t *testing.T) {
	cause := errors.New("remote error")
	items := []CheckItem{
		{Name: "Credentials", Required: true, err: &HintError{Err: cause, Hint: "Check token"}},
		{Name: "Account", OK: true, Message: "Connected"},
		{Name: "Permissions", Skipped: true},
	}

	got := LocalizeCheckItems(items, strings.ToUpper)

	require.Equal(t, "CREDENTIALS", got[0].Name)
	require.Equal(t, "remote error (CHECK TOKEN)", got[0].Message)
	require.True(t, got[0].Required)
	require.ErrorIs(t, got[0].err, cause)
	require.Equal(t, "ACCOUNT", got[1].Name)
	require.Equal(t, "CONNECTED", got[1].Message)
	require.True(t, got[1].OK)
	require.Empty(t, got[2].Message)
	require.True(t, got[2].Skipped)
	require.Equal(t, "Credentials", items[0].Name)
	require.Empty(t, items[0].Message)
	require.Equal(t, "Connected", items[1].Message)
}

func TestParseJiraParams_RejectsInvalidRuleParameters(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		wantError string
	}{
		{name: "missing project", key: "project_key", value: " ", wantError: "project_key is required"},
		{name: "invalid recovery action", key: "on_resolve", value: "delete", wantError: "invalid on_resolve"},
		{name: "priority map", key: "priority_map", value: "[", wantError: "invalid priority_map"},
		{name: "labels", key: "labels", value: "{}", wantError: "invalid labels"},
		{name: "custom fields", key: "fields", value: "[]", wantError: "invalid fields"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := map[string]string{"project_key": "OPS", "issue_type": "Task"}
			params[tc.key] = tc.value

			got, err := parseJiraParams(params)

			require.Nil(t, got)
			require.ErrorContains(t, err, tc.wantError)
		})
	}
}

func TestSeverityPresentation_MapsPriorityAndRecovery(t *testing.T) {
	tests := []struct {
		name      string
		severity  int
		recovered bool
		color     int
		emoji     string
	}{
		{name: "critical", severity: 1, color: 0xE01E5A, emoji: "🔴"},
		{name: "warning", severity: 2, color: 0xF2994A, emoji: "🟠"},
		{name: "info", severity: 3, color: 0xECB22E, emoji: "🟡"},
		{name: "unknown", severity: 99, color: 0xECB22E, emoji: "🟡"},
		{name: "recovered critical", severity: 1, recovered: true, color: 0x2EB67D, emoji: "✅"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			color := severityColor(tc.severity, tc.recovered)
			emoji := severityEmoji(tc.severity, tc.recovered)

			require.Equal(t, tc.color, color)
			require.Equal(t, tc.emoji, emoji)
		})
	}
}
