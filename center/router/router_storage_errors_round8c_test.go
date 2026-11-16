package router

import (
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Missing tables simulate unavailable storage without involving a live database.
// Each route must surface an actionable error instead of reporting success.
func TestR8cRouterStorageFailures(t *testing.T) {
	rt := r8cRouter(t)
	for _, tc := range []struct {
		name    string
		handler gin.HandlerFunc
		body    interface{}
		path    string
		status  int
		message string
	}{
		{"user install", rt.installDateGet, nil, "/1", 200, `"dat":0`},
		{"user delete", rt.userDel, nil, "/1", 200, "no such table"},
		{"user find", rt.userFindAll, nil, "/1", 200, "no such table"},
		{"user groups", rt.userBusiGroupsGets, nil, "/1?userid=1", 200, "no such table"},
		{"channel add", rt.notifyChannelsAdd, []models.NotifyChannelConfig{{Name: "test", Ident: "test", RequestType: "http", RequestConfig: &models.RequestConfig{HTTPRequestConfig: &models.HTTPRequestConfig{URL: "http://127.0.0.1"}}}}, "/1", 200, "no such table"},
		{"channel get", rt.notifyChannelGet, nil, "/1", 200, "no such table"},
		{"channel by ident", rt.notifyChannelGetBy, nil, "/1?ident=test", 200, "no such table"},
		{"channel list", rt.notifyChannelsGet, nil, "/1", 200, "no such table"},
		{"channel normal", rt.notifyChannelsGetForNormalUser, nil, "/1", 200, "no such table"},
		{"channel idents", rt.notifyChannelIdentsGet, nil, "/1", 200, "no such table"},
		{"feishu chats", rt.feishuVisibleChatsGet, map[string]string{"query": "Ops"}, "/1", 200, "no such table"},
		{"flashduty", rt.flashDutyNotifyChannelsGet, nil, "/1", 200, "no such table"},
		{"pipeline list", rt.eventPipelinesList, nil, "/1", 200, "no such table"},
		{"pipeline service", rt.eventPipelinesListByService, nil, "/1", 200, "no such table"},
		{"pipeline get", rt.getEventPipeline, nil, "/1", 200, "no such table"},
		{"pipeline delete", rt.deleteEventPipelines, map[string]interface{}{"ids": []int64{1}}, "/1", 200, "no such table"},
		{"pipeline disable", rt.updateEventPipelinesDisabled, map[string]interface{}{"ids": []int64{1}}, "/1", 200, "no such table"},
		{"pipeline executions", rt.listEventPipelineExecutions, nil, "/1?limit=-1&p=-1", 200, "no such table"},
		{"pipeline all executions", rt.listAllEventPipelineExecutions, nil, "/1?limit=1001&p=-1", 200, "no such table"},
		{"pipeline execution", func(c *gin.Context) {
			c.Params = append(c.Params, gin.Param{Key: "exec_id", Value: "missing"})
			rt.getEventPipelineExecution(c)
		}, nil, "/1", 404, "execution not found"},
		{"pipeline cleanup", rt.cleanEventPipelineExecutions, nil, "/1", 200, "no such table"},
		{"target filter", rt.targetGetsByHostFilter, TargetQuery{P: 1, Limit: 10}, "/1", 200, "no such table"},
		{"target tags add service", rt.targetBindTagsByService, targetTagsForm{Idents: []string{"a"}, Tags: []string{"env=prod"}}, "/1", 400, "no such table"},
		{"target tags add frontend", rt.targetBindTagsByFE, targetTagsForm{Idents: []string{"a"}, Tags: []string{"env=prod"}}, "/1", 400, "no such table"},
		{"target tags remove service", rt.targetUnbindTagsByService, targetTagsForm{Idents: []string{"a"}, Tags: []string{"env"}}, "/1", 400, "no such table"},
		{"target tags remove frontend", rt.targetUnbindTagsByFE, targetTagsForm{Idents: []string{"a"}, Tags: []string{"env"}}, "/1", 400, "no such table"},
		{"host query", rt.targetsOfHostQuery, []models.HostQuery{}, "/1", 500, "no such table"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := r8cRequest(t, tc.handler, tc.body, tc.path)
			require.Equal(t, tc.status, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), tc.message)
		})
	}
}

func TestR8cAdditionalStorageErrors(t *testing.T) {
	rt := r8cRouter(t)
	for _, h := range []gin.HandlerFunc{rt.userGroupGets, rt.userGroupGetsByService, rt.userFindAll, rt.alertRuleGets, rt.alertRuleGetsByGids, rt.alertRulesGetByService, rt.messageTemplatesGet, rt.ssoConfigGets} {
		w := r8cRequest(t, h, nil, "/1")
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "no such table")
	}
	for _, h := range []gin.HandlerFunc{rt.targetUpdateNote, rt.targetUpdateNoteByService, rt.targetUpdateBgidByService, rt.targetDel, rt.targetDelByService, rt.targetBindBgids} {
		w := r8cRequest(t, h, map[string]interface{}{"idents": []string{"host"}, "note": "test", "bgid": 1}, "/1")
		require.Equal(t, 400, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "no such table")
	}
}
