package sender

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	ctxpkg "github.com/ccfos/nightingale/v6/pkg/ctx"
	imodels "github.com/flashcatcloud/ibex/src/models"
	iconfig "github.com/flashcatcloud/ibex/src/server/config"
	istorage "github.com/flashcatcloud/ibex/src/storage"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func round4IbexRedis(t *testing.T) *redis.Client {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	previousCache, previousConfig := istorage.Cache, iconfig.C
	istorage.Cache = client
	iconfig.C = &iconfig.Config{}
	t.Cleanup(func() {
		istorage.Cache, iconfig.C = previousCache, previousConfig
		require.NoError(t, client.Close())
	})
	return client
}

func TestCanDoIbex_RequiresAdminOrMatchingTargetGroup(t *testing.T) {
	cases := []struct {
		name   string
		user   *models.User
		target *models.Target
		want   bool
	}{
		{name: "admin without target", user: &models.User{RolesLst: []string{models.AdminRole}}, want: true},
		{name: "matching group", target: &models.Target{Ident: "node", GroupIds: []int64{7}}, want: true},
		{name: "other group", target: &models.Target{Ident: "node", GroupIds: []int64{8}}},
		{name: "missing target"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			users := &memsto.UserCacheType{}
			if tc.user != nil {
				tc.user.Username = "alice"
				users.Set(map[int64]*models.User{1: tc.user}, 1, 1, 0, 0)
			}
			targets := &memsto.TargetCacheType{}
			if tc.target != nil {
				targets.Set(map[string]*models.Target{"node": tc.target}, 1, 1)
			}

			allowed, err := CanDoIbex("alice", &models.TaskTpl{GroupId: 7}, "node", targets, users)

			require.NoError(t, err)
			require.Equal(t, tc.want, allowed)
		})
	}
}

func TestCallIbex_RejectsMissingTemplateAndUnauthorizedTarget(t *testing.T) {
	cases := []struct {
		name     string
		template *models.TaskTpl
		want     string
	}{
		{name: "missing template", want: "no such tpl(7)"},
		{name: "unauthorized", template: &models.TaskTpl{Id: 7, UpdateBy: "alice", GroupId: 1}, want: "no permission"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			templates := &memsto.TaskTplCache{}
			if tc.template != nil {
				templates.Set(map[int64]*models.TaskTpl{7: tc.template}, 1, 1)
			}

			id, err := CallIbex(&ctxpkg.Context{}, 7, "node", templates,
				&memsto.TargetCacheType{}, &memsto.UserCacheType{}, &models.AlertCurEvent{Hash: "event"}, "")

			require.Zero(t, id)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestCallIbex_EdgeBuildsTaskMetadataAndNotificationRecord(t *testing.T) {
	client := round4IbexRedis(t)
	type ibexRequest struct {
		path string
		body json.RawMessage
	}
	requests := make(chan ibexRequest, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || r.Method != http.MethodPost {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- ibexRequest{path: r.URL.Path, body: body}
		_, _ = w.Write([]byte(`{"dat":17,"err":""}`))
	}))
	t.Cleanup(server.Close)
	iconfig.C.CenterApi = conf.CenterApi{Addrs: []string{server.URL}}
	c := &ctxpkg.Context{CenterApi: iconfig.C.CenterApi}
	template := &models.TaskTpl{
		Id: 7, GroupId: 1, Title: "Restart", Account: "root", UpdateBy: "alice", Script: "echo ok", Args: "default",
	}
	templates := &memsto.TaskTplCache{}
	templates.Set(map[int64]*models.TaskTpl{7: template}, 1, 1)
	targets := &memsto.TargetCacheType{}
	targets.Set(map[string]*models.Target{"node": {Ident: "node", GroupIds: []int64{1}}}, 1, 1)
	event := &models.AlertCurEvent{
		Id: 9, Severity: 2, TriggerValue: "95", TagsJSON: []string{"env=prod", "invalid", "", "path=a=b"},
	}

	id, err := CallIbex(c, 7, "node", templates, targets, &memsto.UserCacheType{}, event, "override")

	require.NoError(t, err)
	require.Equal(t, int64(17), id)
	require.Len(t, requests, 3)
	metaRequest, hostRequest, recordRequest := <-requests, <-requests, <-requests
	require.Equal(t, "/ibex/v1/task/meta", metaRequest.path)
	var meta imodels.TaskMeta
	require.NoError(t, json.Unmarshal(metaRequest.body, &meta))
	require.Equal(t, "Restart FH: node", meta.Title)
	require.Equal(t, "root", meta.Account)
	require.Equal(t, "override", meta.Args)
	require.Equal(t, "echo ok", meta.Script)
	require.JSONEq(t, `{"env":"prod","path":"a=b","alert_severity":"2","alert_trigger_value":"95","is_recovered":"false"}`,
		meta.Stdin)
	require.Equal(t, "/ibex/v1/task/host", hostRequest.path)
	var host imodels.TaskHost
	require.NoError(t, json.Unmarshal(hostRequest.body, &host))
	require.Equal(t, int64(17), host.Id)
	require.Equal(t, "node", host.Host)
	require.Equal(t, "running", host.Status)
	require.Equal(t, "/v1/n9e/task-record-add", recordRequest.path)
	var record models.TaskRecord
	require.NoError(t, json.Unmarshal(recordRequest.body, &record))
	require.Equal(t, int64(17), record.Id)
	require.Equal(t, event.Id, record.EventId)
	require.Equal(t, int64(1), record.GroupId)
	require.Equal(t, "override", record.Args)
	cached, err := client.Get(context.Background(), "task:meta:17").Result()
	require.NoError(t, err)
	var cachedMeta imodels.TaskMeta
	require.NoError(t, json.Unmarshal([]byte(cached), &cachedMeta))
	require.Equal(t, meta.Title, cachedMeta.Title)
	require.Equal(t, meta.Stdin, cachedMeta.Stdin)
}

func TestTaskAdd_RejectsEmptyHostsAndInvalidMetadata(t *testing.T) {
	round4IbexRedis(t)
	cases := []struct {
		name string
		form models.TaskForm
		want string
	}{
		{
			name: "blank and commented hosts",
			form: models.TaskForm{Hosts: []string{" ", " #ignore"}},
			want: "arg(hosts) empty",
		},
		{name: "invalid batch", form: models.TaskForm{Hosts: []string{"node"}, Batch: -1}, want: "arg(batch)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, err := TaskAdd(tc.form, "alice", false)

			require.Zero(t, id)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestCleanHosts_TrimsAndSkipsCommentsWithoutReordering(t *testing.T) {
	hosts := []string{" node-b ", "", " #comment", "node-a", "node-b"}

	result := cleanHosts(hosts)

	require.Equal(t, []string{"node-b", "node-a", "node-b"}, result)
	require.Equal(t, " node-b ", hosts[0])
}
