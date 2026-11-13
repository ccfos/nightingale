package router

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/memsto"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	pkgprom "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/ccfos/nightingale/v6/prom"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func proxyRound6Router(t *testing.T, ds *models.Datasource) *Router {
	t.Helper()
	rt := identityRouter(t, &models.BoardPayload{})
	rt.DatasourceCache = &memsto.DatasourceCacheType{}
	rt.DatasourceCache.Set(map[int64]*models.Datasource{ds.Id: ds}, 1, 1)
	transportsLock.Lock()
	oldTransport, hadTransport := transports[ds.Id]
	oldUpdated, hadUpdated := updatedAts[ds.Id]
	delete(transports, ds.Id)
	delete(updatedAts, ds.Id)
	transportsLock.Unlock()
	t.Cleanup(func() {
		transportsLock.Lock()
		defer transportsLock.Unlock()
		if transport, ok := transports[ds.Id].(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
		delete(transports, ds.Id)
		delete(updatedAts, ds.Id)
		if hadTransport {
			transports[ds.Id] = oldTransport
		}
		if hadUpdated {
			updatedAts[ds.Id] = oldUpdated
		}
	})
	return rt
}

func proxyRound6Request(rt *Router, method, path, body string, shared bool) *httptest.ResponseRecorder {
	engine := gin.New()
	engine.Use(aop.Recovery())
	engine.Any("/api/n9e/proxy/:id/*url", func(c *gin.Context) {
		c.Set("username", "alice")
		if shared {
			c.Set(boardTokenBidKey, int64(660002))
		}
		rt.dsProxy(c)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer incoming-secret")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestDsProxy_ForwardsPathQueryHeadersAndUntruncatedBody(t *testing.T) {
	cases := []struct {
		name     string
		auth     models.Auth
		headers  map[string]string
		wantHost string
		wantAuth string
	}{
		{name: "strips caller authorization"},
		{name: "datasource basic auth", auth: models.Auth{BasicAuthUser: "a", BasicAuthPassword: "b"}, wantAuth: "Basic YTpi"},
		{
			name: "configured headers", headers: map[string]string{"Host": "tenant.internal", "Authorization": "Bearer upstream"},
			wantHost: "tenant.internal", wantAuth: "Bearer upstream",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan *http.Request, 2)
			bodies := make(chan string, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				requests <- r.Clone(r.Context())
				bodies <- string(b)
				w.Header().Set("X-Upstream", "ok")
				w.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(w, `{"result":"ok"}`)
			}))
			t.Cleanup(server.Close)
			rt := proxyRound6Router(t, &models.Datasource{
				Id: 660002, UpdatedAt: 1, PluginType: models.PROMETHEUS, AuthJson: tc.auth,
				HTTPJson: models.HTTP{Url: server.URL + "/base?tenant=one", Headers: tc.headers},
			})
			body := strings.Repeat("a", 5000)

			// The second call also exercises reuse of the datasource transport.
			for i := 0; i < 2; i++ {
				w := proxyRound6Request(rt, http.MethodPost, "/api/n9e/proxy/660002/api/v1/query?query=up", body, false)

				require.Equal(t, http.StatusAccepted, w.Code)
				require.Equal(t, `{"result":"ok"}`, w.Body.String())
				require.Equal(t, "ok", w.Header().Get("X-Upstream"))
				received := <-requests
				require.Equal(t, "/base/api/v1/query", received.URL.Path)
				require.Equal(t, "one", received.URL.Query().Get("tenant"))
				require.Equal(t, "up", received.URL.Query().Get("query"))
				require.Equal(t, tc.wantAuth, received.Header.Get("Authorization"))
				require.Equal(t, "application/json", received.Header.Get("Content-Type"))
				if tc.wantHost != "" {
					require.Equal(t, tc.wantHost, received.Host)
				}
				require.Equal(t, body, <-bodies)
			}
		})
	}
}

func TestDsProxy_ConvertsUpstreamUnauthorizedToBadGateway(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "upstream secret")
	}))
	t.Cleanup(server.Close)
	rt := proxyRound6Router(t, &models.Datasource{Id: 660002, HTTPJson: models.HTTP{Url: server.URL}})

	w := proxyRound6Request(rt, http.MethodGet, "/api/n9e/proxy/660002/api/v1/query", "", false)

	require.Equal(t, http.StatusBadGateway, w.Code)
	require.Contains(t, w.Body.String(), "unauthorized access")
	require.NotContains(t, w.Body.String(), "upstream secret")
}

func TestDsProxy_RejectsMissingDatasourceOrInvalidTLS(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		tls     models.TLS
		status  int
		message string
	}{
		{name: "missing", path: "/api/n9e/proxy/999/api/v1/query", status: 400, message: "no such datasource"},
		{
			name: "invalid TLS", path: "/api/n9e/proxy/660002/api/v1/query", tls: models.TLS{CACert: "invalid"},
			status: 500, message: "failed to create TLS config",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := proxyRound6Router(t, &models.Datasource{
				Id: 660002, HTTPJson: models.HTTP{Url: "http://127.0.0.1", TLS: tc.tls},
			})

			w := proxyRound6Request(rt, http.MethodGet, tc.path, "", false)

			require.Equal(t, tc.status, w.Code)
			require.Contains(t, w.Body.String(), tc.message)
		})
	}
}

func TestDsProxy_SharedDashboardStripsTokenAndRejectsWrites(t *testing.T) {
	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.URL.RawQuery
		_, _ = io.WriteString(w, "query result")
	}))
	t.Cleanup(server.Close)
	rt := proxyRound6Router(t, &models.Datasource{Id: 660002, HTTPJson: models.HTTP{Url: server.URL}})
	require.NoError(t, models.BoardPayloadSave(rt.Ctx, 660002, `{"panels":[{"datasourceValue":660002}]}`))
	boardDsSetMu.Lock()
	original := boardDsSetItems
	boardDsSetItems = make(map[int64]boardDsSetItem)
	boardDsSetMu.Unlock()
	t.Cleanup(func() { boardDsSetMu.Lock(); boardDsSetItems = original; boardDsSetMu.Unlock() })

	allowed := proxyRound6Request(rt, http.MethodGet, "/api/n9e/proxy/660002/api/v1/query?__token=share-secret&query=up", "", true)
	rejected := proxyRound6Request(rt, http.MethodPost, "/api/n9e/proxy/660002/api/v1/admin/tsdb/delete_series", "", true)
	foreign := proxyRound6Request(rt, http.MethodGet, "/api/n9e/proxy/999/api/v1/query", "", true)

	require.Equal(t, http.StatusOK, allowed.Code)
	require.Equal(t, "query result", allowed.Body.String())
	require.Equal(t, "query=up", <-requests)
	require.Equal(t, http.StatusForbidden, rejected.Code)
	require.Contains(t, rejected.Body.String(), "proxy path is not allowed")
	require.Equal(t, http.StatusForbidden, foreign.Code)
	require.Contains(t, foreign.Body.String(), "datasource is not referenced")
	require.Empty(t, requests)
}

func TestTransportGet_InvalidatesStaleOrUnversionedTransport(t *testing.T) {
	cases := []struct {
		name      string
		versioned bool
		updated   int64
	}{
		{name: "matching", versioned: true, updated: 1},
		{name: "stale", versioned: true, updated: 2},
		{name: "unversioned", updated: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			proxyRound6Router(t, &models.Datasource{Id: 660002})
			transport := &http.Transport{}
			transportPut(660002, 1, transport)
			if !tc.versioned {
				transportsLock.Lock()
				delete(updatedAts, 660002)
				transportsLock.Unlock()
			}

			got, ok := transportGet(660002, tc.updated)

			if tc.name == "matching" {
				require.True(t, ok)
				require.Same(t, transport, got)
			} else {
				require.False(t, ok)
				require.Nil(t, got)
				transportsLock.Lock()
				_, cached := transports[660002]
				transportsLock.Unlock()
				require.False(t, cached)
			}
		})
	}
}

type batchRound6PromAPI struct {
	pkgprom.API
	query      func(context.Context, string, time.Time) (model.Value, pkgprom.Warnings, error)
	queryRange func(context.Context, string, pkgprom.Range) (model.Value, pkgprom.Warnings, error)
}

func (f *batchRound6PromAPI) Query(c context.Context, q string, ts time.Time) (model.Value, pkgprom.Warnings, error) {
	return f.query(c, q, ts)
}
func (f *batchRound6PromAPI) QueryRange(c context.Context, q string, r pkgprom.Range) (model.Value, pkgprom.Warnings, error) {
	return f.queryRange(c, q, r)
}

func TestPromBatchQueryRange_PreservesQueryOrderAndRange(t *testing.T) {
	var queries []string
	var ranges []pkgprom.Range
	fake := &batchRound6PromAPI{queryRange: func(_ context.Context, q string, r pkgprom.Range) (model.Value, pkgprom.Warnings, error) {
		queries = append(queries, q)
		ranges = append(ranges, r)
		return &model.Scalar{Timestamp: 1000, Value: model.SampleValue(len(queries))}, nil, nil
	}}
	pc := &prom.PromClientMap{ReaderClients: map[int64]pkgprom.API{1: fake}}

	got, err := PromBatchQueryRange(context.Background(), pc, BatchQueryForm{DatasourceId: 1, Queries: []QueryFormItem{
		{Start: 100, End: 200, Step: 10, Query: "up"},
		{Start: 300, End: 500, Step: 20, Query: "cpu"},
	}})

	require.NoError(t, err)
	require.Equal(t, []string{"up", "cpu"}, queries)
	require.Equal(t, []pkgprom.Range{
		{Start: time.Unix(100, 0), End: time.Unix(200, 0), Step: 10 * time.Second},
		{Start: time.Unix(300, 0), End: time.Unix(500, 0), Step: 20 * time.Second},
	}, ranges)
	require.Equal(t, []model.Value{&model.Scalar{Timestamp: 1000, Value: 1}, &model.Scalar{Timestamp: 1000, Value: 2}}, got)
}

func TestPromBatchQueryInstant_PreservesQueryOrderAndTimestamp(t *testing.T) {
	var queries []string
	var times []time.Time
	fake := &batchRound6PromAPI{query: func(_ context.Context, q string, ts time.Time) (model.Value, pkgprom.Warnings, error) {
		queries = append(queries, q)
		times = append(times, ts)
		return &model.Scalar{Value: model.SampleValue(len(queries))}, nil, nil
	}}
	pc := &prom.PromClientMap{ReaderClients: map[int64]pkgprom.API{1: fake}}

	got, err := PromBatchQueryInstant(context.Background(), pc, BatchInstantForm{DatasourceId: 1, Queries: []InstantFormItem{
		{Time: 100, Query: "up"},
		{Time: 200, Query: "cpu"},
	}})

	require.NoError(t, err)
	require.Equal(t, []string{"up", "cpu"}, queries)
	require.Equal(t, []time.Time{time.Unix(100, 0), time.Unix(200, 0)}, times)
	require.Equal(t, []model.Value{&model.Scalar{Value: 1}, &model.Scalar{Value: 2}}, got)
}

func TestPromBatchQueries_StopAfterUpstreamFailure(t *testing.T) {
	cases := []struct {
		name    string
		instant bool
	}{
		{name: "range"},
		{name: "instant", instant: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			wantErr := errors.New("query failed")
			query := func() (model.Value, pkgprom.Warnings, error) {
				calls++
				if calls == 2 {
					return nil, nil, wantErr
				}
				return &model.Scalar{Value: 7}, nil, nil
			}
			fake := &batchRound6PromAPI{
				query:      func(context.Context, string, time.Time) (model.Value, pkgprom.Warnings, error) { return query() },
				queryRange: func(context.Context, string, pkgprom.Range) (model.Value, pkgprom.Warnings, error) { return query() },
			}
			pc := &prom.PromClientMap{ReaderClients: map[int64]pkgprom.API{1: fake}}
			var got []model.Value
			var err error

			if tc.instant {
				got, err = PromBatchQueryInstant(context.Background(), pc, BatchInstantForm{DatasourceId: 1, Queries: []InstantFormItem{
					{Time: 100, Query: "up"}, {Time: 100, Query: "invalid"}, {Time: 100, Query: "not-called"},
				}})
			} else {
				got, err = PromBatchQueryRange(context.Background(), pc, BatchQueryForm{DatasourceId: 1, Queries: []QueryFormItem{
					{Start: 100, End: 200, Step: 10, Query: "up"},
					{Start: 100, End: 200, Step: 10, Query: "invalid"},
					{Start: 100, End: 200, Step: 10, Query: "not-called"},
				}})
			}

			require.ErrorIs(t, err, wantErr)
			require.Equal(t, []model.Value{&model.Scalar{Value: 7}}, got)
			require.Equal(t, 2, calls)
		})
	}
}

func TestPromBatchQueries_ReportMissingDatasource(t *testing.T) {
	pc := &prom.PromClientMap{}

	ranges, rangeErr := PromBatchQueryRange(context.Background(), pc, BatchQueryForm{DatasourceId: 404})
	instant, instantErr := PromBatchQueryInstant(context.Background(), pc, BatchInstantForm{DatasourceId: 404})

	require.EqualError(t, rangeErr, "no such datasource id: 404")
	require.Nil(t, ranges)
	require.EqualError(t, instantErr, "no such datasource id: 404")
	require.Nil(t, instant)
}
