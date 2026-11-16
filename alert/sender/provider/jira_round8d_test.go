package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestR8dJiraTransitionSelection(t *testing.T) {
	for _, tc := range []struct {
		name, selected, body, want string
		auto                       bool
	}{
		{"missing named", "missing", `{"transitions":[{"id":"1","name":"Open"}]}`, "not found", false},
		{"none available", "", `{"transitions":[]}`, "no transition leads", true},
		{"unspecified", "", `{"transitions":[]}`, "no transition specified", false},
		{"named id", "2", `{"transitions":[{"id":"2","name":"Done"}]}`, "", false},
		{"prefer done", "", `{"transitions":[{"id":"1","name":"Archive","to":{"statusCategory":{"key":"done"}}},{"id":"2","name":"Done","to":{"statusCategory":{"key":"done"}}}]}`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, got := jiraRound7Client(t, func(n int, r capturedRequest) (int, string) {
				if n == 1 {
					return 200, tc.body
				}
				if n == 2 {
					return 400, `{"errors":{"resolution":"required"}}`
				}
				return 204, ``
			})
			var auto func(jiraTransition) bool
			if tc.auto {
				auto = func(v jiraTransition) bool { return v.To.StatusCategory.Key == "done" }
			}
			name, err := (&JiraProvider{}).transitionByName(context.Background(), client, "OPS-1", tc.selected, auto)
			require.Equal(t, "/rest/api/3/issue/OPS-1/transitions", got()[0].Path)
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
				require.Len(t, got(), 1)
			} else {
				require.NoError(t, err)
				require.Equal(t, "Done", name)
				require.Len(t, got(), 3)
				require.JSONEq(t, `{"transition":{"id":"2"}}`, string(got()[1].Body))
				require.JSONEq(t, `{"transition":{"id":"2"},"fields":{"resolution":{"name":"Done"}}}`, string(got()[2].Body))
			}
		})
	}
}
func TestR8dJiraCredentialPartialFailures(t *testing.T) {
	for _, deployment := range []string{"Cloud", "Server", ""} {
		t.Run(deployment, func(t *testing.T) {
			srv, got := newCaptureServer(t, func(_ int, r capturedRequest) (int, string) {
				switch r.Path {
				case "/rest/api/3/project/search":
					return 200, `{"values":[]}`
				case "/rest/api/3/myself":
					return 403, `{"message":"denied"}`
				case "/rest/api/3/serverInfo":
					return 200, `{"deploymentType":"` + deployment + `"}`
				default:
					return 403, `{"message":"permission denied"}`
				}
			})
			ch := jiraTestChannel(908, srv.URL, models.JiraTokenClassic)
			items := (&JiraProvider{}).CheckCredential(context.Background(), ch, srv.Client())
			require.Len(t, items, 7)
			require.True(t, items[0].OK)
			require.True(t, items[1].Skipped)
			if deployment == "Cloud" {
				require.True(t, items[2].OK)
			} else if deployment == "" {
				require.True(t, items[2].Skipped)
			} else {
				require.False(t, items[2].OK)
				require.True(t, items[2].Required)
			}
			for _, it := range items[3:] {
				require.True(t, it.Skipped)
				require.Contains(t, it.Message, "permission denied")
			}
			require.Len(t, got(), 4)
			require.Equal(t, "GET", got()[0].Method)
			require.NotEmpty(t, got()[0].Header.Get("Authorization"))
		})
	}
}
func TestR8dJiraIssueTypeCheckErrors(t *testing.T) {
	for _, failAt := range []int{1, 2, 3} {
		t.Run(string(rune('0'+failAt)), func(t *testing.T) {
			client, got := jiraRound7Client(t, func(n int, r capturedRequest) (int, string) {
				if n == failAt {
					return http.StatusForbidden, `{"message":"denied"}`
				}
				if n == 1 {
					return 200, `{"permissions":{}}`
				}
				return 200, `{"issueTypes":[{"id":"10","name":"Task"}]}`
			})
			result, err := (&JiraAPI{c: client}).IssueTypeCheck(context.Background(), "OPS", "Task")
			require.Nil(t, result)
			require.ErrorContains(t, err, "denied")
			require.Len(t, got(), failAt)
		})
	}
}
