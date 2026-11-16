package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/stretchr/testify/require"
)

func TestR8dEdgeReadContracts(t *testing.T) {
	cases := []struct {
		name, path, body string
		call             func(*ctx.Context) (interface{}, error)
	}{
		{"users", "/v1/n9e/users", `[{"id":7,"username":"edge-user"}]`, func(c *ctx.Context) (interface{}, error) { return UserGetAll(c) }},
		{"datasources", "/v1/n9e/datasources", `[{"id":7,"name":"edge-ds"}]`, func(c *ctx.Context) (interface{}, error) { return GetDatasources(c) }},
		{"datasource map", "/v1/n9e/datasources", `[{"id":7,"name":"edge-ds"}]`, func(c *ctx.Context) (interface{}, error) { return DatasourceGetMap(c) }},
		{"datasource ids", "/v1/n9e/datasource-ids?name=engine", `[7,8]`, func(c *ctx.Context) (interface{}, error) { return GetDatasourceIdsByEngineName(c, "engine") }},
		{"configs", "/v1/n9e/all-configs", `[{"id":7,"ckey":"site"}]`, func(c *ctx.Context) (interface{}, error) { return ConfigsGetAll(c) }},
		{"config", "/v1/n9e/config?key=site", `"edge-site"`, func(c *ctx.Context) (interface{}, error) { return ConfigsGet(c, "site") }},
		{"groups", "/v1/n9e/busi-groups", `[{"id":7,"name":"edge-group"}]`, func(c *ctx.Context) (interface{}, error) { return BusiGroupGetMap(c) }},
		{"subscriptions", "/v1/n9e/alert-subscribes", `[{"id":7,"severities":[1,2],"datasource_ids":[3]}]`, func(c *ctx.Context) (interface{}, error) { return AlertSubscribeGetsAll(c) }},
		{"mutes", "/v1/n9e/active-alert-mutes", `[{"id":7,"datasource_ids":[3]}]`, func(c *ctx.Context) (interface{}, error) { return AlertMuteGetsAll(c) }},
		{"recording", "/v1/n9e/recording-rules", `[{"id":7,"name":"cpu_total","datasource_ids":[3]}]`, func(c *ctx.Context) (interface{}, error) { return RecordingRuleGetsByCluster(c) }},
		{"channels", "/v1/n9e/notify-channels", `[{"id":7,"name":"edge-channel"}]`, func(c *ctx.Context) (interface{}, error) { return NotifyChannelGetsAll(c) }},
		{"tasks", "/v1/n9e/task-tpls", `[{"id":7,"title":"edge-task"}]`, func(c *ctx.Context) (interface{}, error) { return TaskTplGetAll(c) }},
		{"task", "/v1/n9e/task-tpl/7", `{"id":7,"title":"edge-task"}`, func(c *ctx.Context) (interface{}, error) { return TaskTplGetById(c, 7) }},
		{"events", "/v1/n9e/alert-cur-events-get-by-rid?rid=7&dsid=3", `[{"id":7,"rule_name":"edge-rule"}]`, func(c *ctx.Context) (interface{}, error) { return AlertCurEventGetByRuleIdAndDsId(c, 7, 3) }},
		{"variables", "/v1/n9e/user-variable/decrypt", `{"token":"value"}`, func(c *ctx.Context) (interface{}, error) { return ConfigUserVariableGetDecryptMap(c, nil, "") }},
		{"user statistics", "/v1/n9e/statistic?name=user", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return UserStatistics(c) }},
		{"datasource statistics", "/v1/n9e/statistic?name=datasource", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return DatasourceStatistics(c) }},
		{"busi_group statistics", "/v1/n9e/statistic?name=busi_group", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return BusiGroupStatistics(c) }},
		{"alert_rule statistics", "/v1/n9e/statistic?name=alert_rule", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return AlertRuleStatistics(c) }},
		{"alert_mute statistics", "/v1/n9e/statistic?name=alert_mute", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return AlertMuteStatistics(c) }},
		{"alert_subscribe statistics", "/v1/n9e/statistic?name=alert_subscribe", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return AlertSubscribeStatistics(c) }},
		{"recording_rule statistics", "/v1/n9e/statistic?name=recording_rule", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return RecordingRuleStatistics(c) }},
		{"notify_channel statistics", "/v1/n9e/statistic?name=notify_channel", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return NotifyChannelStatistics(c) }},
		{"target statistics", "/v1/n9e/statistic?name=target", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return TargetStatistics(c) }},
		{"user_variable statistics", "/v1/n9e/statistic?name=user_variable", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return ConfigsUserVariableStatistics(c) }},
		{"cval statistics", "/v1/n9e/statistic?name=cval", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return ConfigCvalStatistics(c) }},
		{"task statistics", "/v1/n9e/task-tpl/statistics", `{"Total":7,"LastUpdated":42}`, func(c *ctx.Context) (interface{}, error) { return TaskTplStatistics(c) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, fail := range []bool{false, true} {
				t.Run(map[bool]string{false: "success", true: "center error"}[fail], func(t *testing.T) {
					requests := make(chan string, 1)
					srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests <- r.Method + " " + r.URL.RequestURI()
						w.Header().Set("Content-Type", "application/json")
						if fail {
							_, _ = w.Write([]byte(`{"err":"center unavailable","dat":null}`))
							return
						}
						_, _ = w.Write([]byte(`{"err":"","dat":` + tc.body + `}`))
					}))
					t.Cleanup(srv.Close)
					c := ctx.NewContext(context.Background(), nil, false, conf.CenterApi{Addrs: []string{srv.URL}})
					got, err := tc.call(c)
					require.Equal(t, "GET "+tc.path, <-requests)
					if fail {
						require.ErrorContains(t, err, "center unavailable")
						return
					}
					require.NoError(t, err)
					// Re-encode to check the actual typed result, including model conversion.
					b, err := json.Marshal(got)
					require.NoError(t, err)
					var expected interface{}
					require.NoError(t, json.Unmarshal([]byte(tc.body), &expected))
					var actual interface{}
					require.NoError(t, json.Unmarshal(b, &actual))
					if tc.name == "datasource map" || tc.name == "groups" {
						require.Contains(t, string(b), `"7"`)
					} else if _, ok := expected.(map[string]interface{}); ok {
						for k, v := range expected.(map[string]interface{}) {
							require.Equal(t, v, actual.(map[string]interface{})[k])
						}
					} else if arr, ok := expected.([]interface{}); ok {
						require.Len(t, actual, len(arr))
						for i, v := range arr {
							if m, ok := v.(map[string]interface{}); ok {
								for k, w := range m {
									require.Equal(t, w, actual.([]interface{})[i].(map[string]interface{})[k])
								}
							} else {
								require.Equal(t, v, actual.([]interface{})[i])
							}
						}
					} else {
						require.Equal(t, expected, actual)
					}
				})
			}
		})
	}
}
