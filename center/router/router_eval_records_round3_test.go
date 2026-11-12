package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/alert/naming"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/evallog"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func evalRecordsRouter(t *testing.T) *Router {
	t.Helper()
	rt := identityRouter(t, &models.AlertRule{}, &models.AlertingEngines{}, &models.BusiGroup{},
		&models.BusiGroupMember{}, &models.UserGroupMember{})
	seedEventListingMembership(t, rt)
	rt.Alert.Heartbeat.IP = "127.0.0.1"
	rt.HTTP.Port = 17000
	rt.Alert.Heartbeat.EngineName = "round3-eval"
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertRule{Id: 1, GroupId: 1, Prod: "host", Cate: "prometheus"}).Error)
	setEmptyEvalRecordsRing(t, rt.Alert.Heartbeat.EngineName)
	return rt
}

func setEmptyEvalRecordsRing(t *testing.T, key string) {
	t.Helper()
	naming.DatasourceHashRing.RLock()
	original, existed := naming.DatasourceHashRing.Rings[key]
	naming.DatasourceHashRing.RUnlock()
	naming.DatasourceHashRing.Set(key, naming.NewConsistentHashRing(naming.NodeReplicas, nil))
	t.Cleanup(func() {
		if existed {
			naming.DatasourceHashRing.Set(key, original)
		} else {
			naming.DatasourceHashRing.Del(key)
		}
	})
}

func evalRecordsHandler(rt *Router) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "arid", Value: c.Param("id")})
		rt.alertRuleEvalRecords(c)
	}
}

func TestAlertRuleEvalRecords_RejectsForeignGroupBeforeQuerying(t *testing.T) {
	rt := evalRecordsRouter(t)

	w := identityRequest(t, rt, evalRecordsHandler(rt), nil, "1?datasource_id=7301", &models.User{Id: 2})

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Contains(t, w.Body.String(), "forbidden")
	require.Zero(t, len(evalRecordsAggSem))
}

func TestAlertRuleEvalRecords_MergesPeerRecordsAndAppliesLimit(t *testing.T) {
	rt := evalRecordsRouter(t)
	rt.HTTP.APIForService.BasicAuth = map[string]string{"engine": "password"}
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "engine", username)
		assert.Equal(t, "password", password)
		assert.Equal(t, "/v1/n9e/eval-records", r.URL.Path)
		assert.Equal(t, "1", r.URL.Query().Get("rule_id"))
		assert.Equal(t, "7301", r.URL.Query().Get("datasource_id"))
		assert.Equal(t, "100", r.URL.Query().Get("from"))
		assert.Equal(t, "200", r.URL.Query().Get("to"))
		assert.Equal(t, "190000", r.URL.Query().Get("before"))
		assert.Equal(t, "2", r.URL.Query().Get("limit"))
		fmt.Fprint(w, `{"dat":{"enabled":true,"list":[{"ts":130000},{"ts":110000}]},"err":""}`)
	}))
	t.Cleanup(owner.Close)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"dat":{"enabled":true,"list":[{"ts":150000}],"truncated":true,"note":"byte budget"}}`)
	}))
	t.Cleanup(peer.Close)
	ownerNode := strings.TrimPrefix(owner.URL, "http://")
	peerNode := strings.TrimPrefix(peer.URL, "http://")
	// Use a future heartbeat to keep this fixture independent of test runtime.
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertingEngines{
		{Instance: ownerNode, EngineCluster: "round3-eval", DatasourceId: 7301, Clock: 1 << 62},
		{Instance: peerNode, EngineCluster: "round3-eval", DatasourceId: 7301, Clock: 1 << 62},
	}).Error)

	w := identityRequest(t, rt, evalRecordsHandler(rt), nil,
		"1?datasource_id=7301&from=100&to=200&before=190000&limit=2", &models.User{Id: 1})

	var got struct {
		List      []evallog.EvalRecord `json:"list"`
		Instances []string             `json:"instances"`
		Errors    []evalRecordsNodeErr `json:"errors"`
		Truncated bool                 `json:"truncated"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got.List, 2)
	require.Equal(t, int64(150000), got.List[0].Ts)
	require.Equal(t, int64(130000), got.List[1].Ts)
	require.ElementsMatch(t, []string{ownerNode, peerNode}, got.Instances)
	require.True(t, got.Truncated)
	require.Len(t, got.Errors, 2)
	errorsByNode := make(map[string]string)
	for _, item := range got.Errors {
		errorsByNode[item.Instance] = item.Error
	}
	require.Equal(t, "byte budget", errorsByNode[peerNode])
	require.Contains(t, errorsByNode["127.0.0.1:17000"], "eval records not enabled")
	require.Zero(t, len(evalRecordsAggSem))
}

func TestAlertRuleEvalRecords_ReportsDisabledEngineAndNormalizesEmptyList(t *testing.T) {
	rt := evalRecordsRouter(t)
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, fmt.Sprint(evallog.DefaultQueryLimit), r.URL.Query().Get("limit"))
		fmt.Fprint(w, `{"dat":{"enabled":false,"list":[]}}`)
	}))
	t.Cleanup(engine.Close)
	node := strings.TrimPrefix(engine.URL, "http://")
	require.NoError(t, rt.Ctx.DB.Create(&models.AlertingEngines{
		Instance: node, EngineCluster: "round3-eval", DatasourceId: 7302, Clock: 1 << 62,
	}).Error)

	w := identityRequest(t, rt, evalRecordsHandler(rt), nil,
		"1?datasource_id=7302&from=100&to=200&limit=-1", &models.User{Id: 1})

	var got struct {
		List     []evallog.EvalRecord `json:"list"`
		Disabled []string             `json:"disabled_instances"`
		Errors   []evalRecordsNodeErr `json:"errors"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.NotNil(t, got.List)
	require.Empty(t, got.List)
	require.ElementsMatch(t, []string{node, "127.0.0.1:17000"}, got.Disabled)
	require.Len(t, got.Errors, 2)
	require.Equal(t, int64(7302), got.Errors[0].DatasourceId)
	require.Contains(t, got.Errors[0].Error, "eval records not enabled")
}

func TestAlertRuleEvalRecords_OmitsFailedPeerWhenOtherPeersHaveRecords(t *testing.T) {
	rt := evalRecordsRouter(t)
	owner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"err":"engine storage unavailable"}`)
	}))
	t.Cleanup(owner.Close)
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Missing enabled is supported for older engines.
		fmt.Fprint(w, `{"dat":{"list":[{"ts":125000}]}}`)
	}))
	t.Cleanup(peer.Close)
	ownerNode := strings.TrimPrefix(owner.URL, "http://")
	peerNode := strings.TrimPrefix(peer.URL, "http://")
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertingEngines{
		{Instance: ownerNode, EngineCluster: "round3-eval", DatasourceId: 7303, Clock: 1 << 62},
		{Instance: peerNode, EngineCluster: "round3-eval", DatasourceId: 7303, Clock: 1 << 62},
	}).Error)

	w := identityRequest(t, rt, evalRecordsHandler(rt), nil,
		"1?datasource_id=7303&from=100&to=200", &models.User{Id: 1})

	var got struct {
		List   []evallog.EvalRecord `json:"list"`
		Errors []evalRecordsNodeErr `json:"errors"`
	}
	require.Empty(t, identityReply(t, w, &got))
	require.Len(t, got.List, 1)
	require.Equal(t, int64(125000), got.List[0].Ts)
	require.Len(t, got.Errors, 1)
	require.Equal(t, "127.0.0.1:17000", got.Errors[0].Instance)
	require.Contains(t, got.Errors[0].Error, "eval records not enabled")
}

func TestEvalRecordsPeerInstances_FiltersDatasourceAndDeduplicatesHostCluster(t *testing.T) {
	rt := evalRecordsRouter(t)
	require.NoError(t, rt.Ctx.DB.Create(&[]models.AlertingEngines{
		{Instance: "engine-a", EngineCluster: "round3-eval", DatasourceId: 7, Clock: 1 << 62},
		{Instance: "engine-a", EngineCluster: "round3-eval", DatasourceId: 8, Clock: 1 << 62},
		{Instance: "engine-b", EngineCluster: "other", DatasourceId: 7, Clock: 1 << 62},
		{Instance: "stale", EngineCluster: "round3-eval", DatasourceId: 7, Clock: 1},
	}).Error)

	metric := rt.evalRecordsPeerInstances(&models.AlertRule{Prod: "metric"}, []int64{7, 8})
	host := rt.evalRecordsPeerInstances(&models.AlertRule{Prod: "host"}, []int64{0})

	require.Equal(t, map[int64][]string{7: {"engine-a", "engine-b"}, 8: {"engine-a"}}, metric)
	require.Equal(t, map[int64][]string{0: {"engine-a"}}, host)
}

func TestEvalRecordsOwner_ReportsMissingActiveInstances(t *testing.T) {
	setEmptyEvalRecordsRing(t, "7304")

	_, err := evalRecordsOwner(7304, "rule", nil)

	require.EqualError(t, err, "no active instances for datasource 7304")
}

func TestAcquireEvalRecordsAgg_CanceledWaitDoesNotLeakToken(t *testing.T) {
	original := evalRecordsAggSem
	evalRecordsAggSem = make(chan struct{}, 1)
	t.Cleanup(func() { evalRecordsAggSem = original })
	evalRecordsAggSem <- struct{}{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	acquired := acquireEvalRecordsAgg(ctx)

	require.False(t, acquired)
	require.Len(t, evalRecordsAggSem, 1)
}

func TestForwardEvalRecords_RejectsMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"dat":`)
	}))
	t.Cleanup(server.Close)
	rt := &Router{}

	_, err := rt.forwardEvalRecords(context.Background(), strings.TrimPrefix(server.URL, "http://"), 1, 2, 3, 4, 0, 10)

	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected EOF")
}

func TestEvalRecordsNodeResult_ProvidesDefaultTruncationExplanation(t *testing.T) {
	result := evalRecordsNodeResult{truncated: true}

	note := result.noteOrDefault()

	require.Contains(t, note, "per-query byte budget")
	require.Contains(t, note, "narrow the time range")
}

func TestAlertRuleEvalRecords_MissingRuleDoesNotAcquireSlot(t *testing.T) {
	rt := evalRecordsRouter(t)

	w := identityRequest(t, rt, evalRecordsHandler(rt), nil, "404", &models.User{Id: 1})

	require.Contains(t, identityReply(t, w, nil), "no such alert rule")
	require.Zero(t, len(evalRecordsAggSem))
}
