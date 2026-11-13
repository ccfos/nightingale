package sender

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	ctxpkg "github.com/ccfos/nightingale/v6/pkg/ctx"
	imodels "github.com/flashcatcloud/ibex/src/models"
	iconfig "github.com/flashcatcloud/ibex/src/server/config"
	istorage "github.com/flashcatcloud/ibex/src/storage"
	"github.com/stretchr/testify/require"
)

func assertIbexCallbackHost(t *testing.T, callbackURL, targetIdent string, tags map[string]string, expectedHost string) {
	t.Helper()
	round4IbexRedis(t)
	hosts := make(chan imodels.TaskHost, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ibex/v1/task/host" {
			var host imodels.TaskHost
			if err := json.NewDecoder(r.Body).Decode(&host); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			hosts <- host
		}
		_, _ = w.Write([]byte(`{"dat":17,"err":""}`))
	}))
	t.Cleanup(server.Close)
	iconfig.C.CenterApi = conf.CenterApi{Addrs: []string{server.URL}}
	templates := &memsto.TaskTplCache{}
	templates.Set(map[int64]*models.TaskTpl{7: {
		Id: 7, Title: "Restart", Account: "root", Script: "echo ok", GroupId: 1,
	}}, 1, 1)
	targets := &memsto.TargetCacheType{}
	targets.Set(map[string]*models.Target{expectedHost: {Ident: expectedHost, GroupIds: []int64{1}}}, 1, 1)
	callback := NewCallBacker(models.IbexDomain, targets, &memsto.UserCacheType{}, templates, nil)

	callback.CallBack(CallBackContext{
		Ctx: &ctxpkg.Context{CenterApi: iconfig.C.CenterApi}, CallBackURL: callbackURL,
		Events: []*models.AlertCurEvent{{Id: 12, TargetIdent: targetIdent, TagsMap: tags}},
	})

	require.Len(t, hosts, 1)
	host := <-hosts
	require.Equal(t, expectedHost, host.Host)
	require.Equal(t, int64(17), host.Id)
	require.Equal(t, "running", host.Status)
}

func TestIbexCallback_UsesExplicitURLHost(t *testing.T) {
	assertIbexCallbackHost(t, "${ibex}/7/explicit", "event-host", map[string]string{"ident": "tag-host"}, "explicit")
}

func TestIbexCallback_FallsBackToEventTarget(t *testing.T) {
	assertIbexCallbackHost(t, "${ibex}/7", "event-host", map[string]string{"ident": "tag-host"}, "event-host")
}

func TestIbexCallback_FallsBackToIdentLabel(t *testing.T) {
	assertIbexCallbackHost(t, "${ibex}/7", "", map[string]string{"ident": "tag-host"}, "tag-host")
}

func TestIbexCallback_RejectsIncompleteOrRecoveredEvents(t *testing.T) {
	for _, tc := range []struct {
		name   string
		url    string
		events []*models.AlertCurEvent
	}{
		{name: "missing URL", events: []*models.AlertCurEvent{{}}},
		{name: "missing events", url: "${ibex}/7/node"},
		{name: "recovered", url: "${ibex}/7/node", events: []*models.AlertCurEvent{{IsRecovered: true}}},
		{name: "invalid id", url: "${ibex}/bad/node", events: []*models.AlertCurEvent{{}}},
		{name: "missing host", url: "${ibex}/7", events: []*models.AlertCurEvent{{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			redis := round4IbexRedis(t)
			requests := make(chan string, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.URL.Path
				_, _ = w.Write([]byte(`{"dat":17,"err":""}`))
			}))
			t.Cleanup(server.Close)
			iconfig.C.CenterApi = conf.CenterApi{Addrs: []string{server.URL}}
			callback := &IbexCallBacker{taskTplCache: &memsto.TaskTplCache{}}

			callback.CallBack(CallBackContext{Ctx: &ctxpkg.Context{}, CallBackURL: tc.url, Events: tc.events})

			require.Empty(t, requests)
			keys, err := redis.Keys(context.Background(), "*").Result()
			require.NoError(t, err)
			require.Empty(t, keys)
		})
	}
}

func TestCallIbex_ReturnsTaskValidationFailure(t *testing.T) {
	round4IbexRedis(t)
	templates := &memsto.TaskTplCache{}
	templates.Set(map[int64]*models.TaskTpl{7: {Id: 7, GroupId: 1, Batch: -1}}, 1, 1)
	targets := &memsto.TargetCacheType{}
	targets.Set(map[string]*models.Target{"node": {Ident: "node", GroupIds: []int64{1}}}, 1, 1)

	id, err := CallIbex(&ctxpkg.Context{}, 7, "node", templates, targets,
		&memsto.UserCacheType{}, &models.AlertCurEvent{Hash: "cpu-1"}, "")

	require.Zero(t, id)
	require.ErrorContains(t, err, "call ibex fail")
	require.ErrorContains(t, err, "arg(batch)")
}

func TestTaskAdd_RequiresRedis(t *testing.T) {
	previous := istorage.Cache
	istorage.Cache = nil
	t.Cleanup(func() { istorage.Cache = previous })

	id, err := TaskAdd(models.TaskForm{Hosts: []string{"node"}}, "alice", false)

	require.Zero(t, id)
	require.EqualError(t, err, "redis cache is nil")
}

func TestTaskAdd_CachesOfflineTaskWhenCenterRejectsWrites(t *testing.T) {
	client := round4IbexRedis(t)
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.Path
		_, _ = w.Write([]byte(`{"err":"center unavailable"}`))
	}))
	t.Cleanup(server.Close)
	iconfig.C.CenterApi = conf.CenterApi{Addrs: []string{server.URL}}

	id, err := TaskAdd(models.TaskForm{
		Title: "Restart", Account: "root", Script: "echo ok", Hosts: []string{"node"}, AlertTriggered: true,
	}, "alice", false)

	require.NoError(t, err)
	require.Equal(t, int64(1), id)
	require.Len(t, requests, 2)
	require.Equal(t, "/ibex/v1/task/meta", <-requests)
	require.Equal(t, "/ibex/v1/task/host", <-requests)
	meta, err := imodels.TaskMetaCacheGet(id)
	require.NoError(t, err)
	require.Equal(t, "Restart FH: node", meta.Title)
	require.Equal(t, "echo ok", meta.Script)
	pending, err := client.HGetAll(context.Background(), imodels.IBEX_HOST_DOING).Result()
	require.NoError(t, err)
	require.Len(t, pending, 1)
	for _, encoded := range pending {
		var task imodels.TaskHostDoing
		require.NoError(t, json.Unmarshal([]byte(encoded), &task))
		require.Equal(t, id, task.Id)
		require.Equal(t, "node", task.Host)
		require.Equal(t, "start", task.Action)
	}
}

func TestCallIbex_ReturnsCreatedTaskIDWhenRecordPersistenceFails(t *testing.T) {
	round4IbexRedis(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/n9e/task-record-add" {
			_, _ = w.Write([]byte(`{"err":"record rejected"}`))
			return
		}
		_, _ = w.Write([]byte(`{"dat":17,"err":""}`))
	}))
	t.Cleanup(server.Close)
	iconfig.C.CenterApi = conf.CenterApi{Addrs: []string{server.URL}}
	templates := &memsto.TaskTplCache{}
	templates.Set(map[int64]*models.TaskTpl{7: {
		Id: 7, Title: "Restart", Script: "echo ok", Account: "root", GroupId: 1,
	}}, 1, 1)
	targets := &memsto.TargetCacheType{}
	targets.Set(map[string]*models.Target{"node": {Ident: "node", GroupIds: []int64{1}}}, 1, 1)

	id, err := CallIbex(&ctxpkg.Context{CenterApi: iconfig.C.CenterApi}, 7, "node", templates, targets,
		&memsto.UserCacheType{}, &models.AlertCurEvent{Id: 12}, "")

	require.Equal(t, int64(17), id)
	require.ErrorContains(t, err, "persist task_record fail")
	require.ErrorContains(t, err, "/v1/n9e/task-record-add")
	meta, cacheErr := imodels.TaskMetaCacheGet(id)
	require.NoError(t, cacheErr)
	require.Equal(t, "Restart FH: node", meta.Title)
}
