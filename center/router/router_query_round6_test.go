package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/dskit/types"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

type queryRound6Datasource struct {
	queryBatchV2FakeDatasource
	data      func(context.Context, interface{}) ([]models.DataResp, error)
	logs      func(context.Context, interface{}) ([]interface{}, int64, error)
	databases func(context.Context) ([]string, error)
	tables    func(context.Context, string) ([]string, error)
	columns   func(context.Context, interface{}) ([]*types.ColumnProperty, error)
}

func (f *queryRound6Datasource) QueryData(c context.Context, q interface{}) ([]models.DataResp, error) {
	return f.data(c, q)
}

func (f *queryRound6Datasource) QueryLog(c context.Context, q interface{}) ([]interface{}, int64, error) {
	return f.logs(c, q)
}

func (f *queryRound6Datasource) ShowDatabases(c context.Context) ([]string, error) {
	return f.databases(c)
}
func (f *queryRound6Datasource) ShowTables(c context.Context, db string) ([]string, error) {
	return f.tables(c, db)
}
func (f *queryRound6Datasource) DescribeTable(c context.Context, q interface{}) ([]*types.ColumnProperty, error) {
	return f.columns(c, q)
}

func putRound6Datasource(t *testing.T, cate string, id int64, ds datasource.Datasource) {
	t.Helper()
	_, exists := dscache.DsCache.Get(cate, id)
	require.False(t, exists, "test datasource ID must be unused")
	dscache.DsCache.Put(cate, id, ds)
	t.Cleanup(func() {
		dscache.DsCache.Delete(cate, id)
	})
}

func queryRound6Context() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/query", nil)
	return c
}

func TestQueryDataConcurrently_SortsMetricsAcrossQueries(t *testing.T) {
	fake := &queryRound6Datasource{data: func(_ context.Context, q interface{}) ([]models.DataResp, error) {
		host := q.(string)
		return []models.DataResp{{Ref: host, Metric: model.Metric{"host": model.LabelValue(host)}, Values: [][]float64{{100, 7}}}}, nil
	}}
	putRound6Datasource(t, "iotdb", 660001, fake)
	form := models.QueryParam{Cate: "iotdb", DatasourceId: 660001, Queries: []interface{}{"z", "a"}}

	got, err := QueryDataConcurrently(true, queryRound6Context(), form)

	require.NoError(t, err)
	require.Equal(t, []models.DataResp{
		{Ref: "a", Metric: model.Metric{"host": "a"}, Values: [][]float64{{100, 7}}},
		{Ref: "z", Metric: model.Metric{"host": "z"}, Values: [][]float64{{100, 7}}},
	}, got)
}

func TestQueryDataConcurrently_PropagatesDatasourceError(t *testing.T) {
	want := errors.New("query rejected")
	fake := &queryRound6Datasource{data: func(context.Context, interface{}) ([]models.DataResp, error) { return nil, want }}
	putRound6Datasource(t, "iotdb", 660001, fake)

	got, err := QueryDataConcurrently(true, queryRound6Context(), models.QueryParam{
		Cate: "iotdb", DatasourceId: 660001, Queries: []interface{}{"select value"},
	})

	require.ErrorIs(t, err, want)
	require.Nil(t, got)
}

func TestQueryLogConcurrently_MergesRecordsAndTotals(t *testing.T) {
	fake := &queryRound6Datasource{logs: func(_ context.Context, q interface{}) ([]interface{}, int64, error) {
		return []interface{}{q}, 9, nil
	}}
	putRound6Datasource(t, "iotdb", 660001, fake)

	got, err := QueryLogConcurrently(true, queryRound6Context(), models.QueryParam{
		Cate: "iotdb", DatasourceId: 660001, Queries: []interface{}{"first", "second"},
	})

	require.NoError(t, err)
	require.Equal(t, int64(18), got.Total)
	require.ElementsMatch(t, []interface{}{"first", "second"}, got.List)
}

func TestQueryLogConcurrently_RejectsErrorsAndEmptyResults(t *testing.T) {
	cases := []struct {
		name     string
		queryErr error
		message  string
	}{
		{name: "error", queryErr: errors.New("log unavailable"), message: "log unavailable"},
		{name: "empty", message: "no data"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &queryRound6Datasource{logs: func(context.Context, interface{}) ([]interface{}, int64, error) {
				return nil, 0, tc.queryErr
			}}
			putRound6Datasource(t, "iotdb", 660001, fake)

			got, err := QueryLogConcurrently(true, queryRound6Context(), models.QueryParam{
				Cate: "iotdb", DatasourceId: 660001, Queries: []interface{}{"logs"},
			})

			require.EqualError(t, err, tc.message)
			require.Empty(t, got)
		})
	}
}

func TestQueryLogBatchConcurrently_PreservesQueryIdentityAndReadOnlyContext(t *testing.T) {
	c := queryRound6Context()
	c.Set("user", &models.User{Username: "alice"})
	c.Request = c.Request.WithContext(types.WithCallContext(c.Request.Context(), types.CallContext{EnforceReadOnly: true}))
	contexts := make(chan types.CallContext, 2)
	fake := &queryRound6Datasource{logs: func(c context.Context, q interface{}) ([]interface{}, int64, error) {
		cc, _ := types.CallContextFromCtx(c)
		contexts <- cc
		return []interface{}{q}, 4, nil
	}}
	putRound6Datasource(t, "iotdb", 660001, fake)
	form := QueryFrom{Queries: []Query{
		{Ref: "A", Did: 660001, DsCate: "iotdb", Query: map[string]interface{}{"sql": "select 1"}},
		{Ref: "B", Did: 660001, DsCate: "iotdb", Query: map[string]interface{}{"sql": "select 2"}},
	}}

	got, err := QueryLogBatchConcurrently(false, c, form)

	require.NoError(t, err)
	require.Equal(t, int64(8), got.Total)
	require.ElementsMatch(t, []interface{}{
		map[string]interface{}{"ref": "A", "ds_id": int64(660001), "ds_cate": "iotdb", "data": []interface{}{form.Queries[0].Query}},
		map[string]interface{}{"ref": "B", "ds_id": int64(660001), "ds_cate": "iotdb", "data": []interface{}{form.Queries[1].Query}},
	}, got.List)
	for range form.Queries {
		require.Equal(t, types.CallContext{DatasourceID: 660001, Operator: "alice", EnforceReadOnly: true}, <-contexts)
	}
}

func TestQueryLogBatchConcurrently_PropagatesDatasourceFailure(t *testing.T) {
	want := errors.New("batch query failed")
	fake := &queryRound6Datasource{logs: func(context.Context, interface{}) ([]interface{}, int64, error) { return nil, 0, want }}
	putRound6Datasource(t, "iotdb", 660001, fake)

	got, err := QueryLogBatchConcurrently(true, queryRound6Context(), QueryFrom{Queries: []Query{
		{Ref: "A", Did: 660001, DsCate: "iotdb", Query: map[string]interface{}{"sql": "select 1"}},
	}})

	require.ErrorIs(t, err, want)
	require.Empty(t, got)
}

func TestQueryHandlers_EnforcePermissionBeforeCallingDatasource(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*Router) gin.HandlerFunc
		form    interface{}
	}{
		{name: "data", handler: func(r *Router) gin.HandlerFunc { return r.QueryData }, form: models.QueryParam{
			Cate: "iotdb", DatasourceId: 660001, Queries: []interface{}{"select 1"},
		}},
		{name: "log v2", handler: func(r *Router) gin.HandlerFunc { return r.QueryLogV2 }, form: models.QueryParam{
			Cate: "iotdb", DatasourceId: 660001, Queries: []interface{}{"select 1"},
		}},
		{name: "legacy log", handler: func(r *Router) gin.HandlerFunc { return r.QueryLog }, form: models.QueryParam{
			Cate: "elasticsearch", DatasourceId: 660001, Queries: []interface{}{"select 1"},
		}},
		{name: "batch log", handler: func(r *Router) gin.HandlerFunc { return r.QueryLogBatch }, form: QueryFrom{Queries: []Query{
			{Ref: "A", Did: 660001, DsCate: "iotdb", Query: map[string]interface{}{"sql": "select 1"}},
		}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old := CheckDsPerm
			t.Cleanup(func() { CheckDsPerm = old })
			checks := 0
			CheckDsPerm = func(_ *gin.Context, id int64, cate string, q interface{}) bool {
				checks++
				require.Equal(t, int64(660001), id)
				require.NotEmpty(t, cate)
				require.NotNil(t, q)
				return false
			}
			rt := &Router{}

			w := identityRequest(t, rt, tc.handler(rt), tc.form, "0", &models.User{Username: "alice"})

			require.Equal(t, http.StatusOK, w.Code)
			require.Contains(t, w.Body.String(), "forbidden")
			require.Equal(t, 1, checks)
		})
	}
}

func TestQueryHandlers_ReturnDatasourceResults(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*Router) gin.HandlerFunc
		cate    string
		want    string
	}{
		{name: "data", handler: func(r *Router) gin.HandlerFunc { return r.QueryData }, cate: "iotdb",
			want: `{"dat":[{"ref":"A","query":"","metric":{"host":"one"},"values":[[100,2]]}],"err":"","request_id":""}`},
		{name: "log v2", handler: func(r *Router) gin.HandlerFunc { return r.QueryLogV2 }, cate: "iotdb",
			want: `{"dat":{"list":[{"message":"ok"}],"total":5},"err":"","request_id":""}`},
		{name: "legacy log", handler: func(r *Router) gin.HandlerFunc { return r.QueryLog }, cate: "elasticsearch",
			want: `{"dat":[{"message":"ok"}],"err":"","request_id":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var query interface{}
			fake := &queryRound6Datasource{
				data: func(_ context.Context, q interface{}) ([]models.DataResp, error) {
					query = q
					return []models.DataResp{{Ref: "A", Metric: model.Metric{"host": "one"}, Values: [][]float64{{100, 2}}}}, nil
				},
				logs: func(_ context.Context, q interface{}) ([]interface{}, int64, error) {
					query = q
					return []interface{}{map[string]interface{}{"message": "ok"}}, 5, nil
				},
			}
			putRound6Datasource(t, tc.cate, 660001, fake)
			rt := &Router{}

			w := identityRequest(t, rt, tc.handler(rt), models.QueryParam{
				Cate: tc.cate, DatasourceId: 660001, Queries: []interface{}{"select 1"},
			}, "0", &models.User{Username: "alice"})

			require.Equal(t, http.StatusOK, w.Code)
			require.JSONEq(t, tc.want, w.Body.String())
			require.Equal(t, "select 1", query)
		})
	}
}

func TestShowDatabases_FiltersMetadataThroughHook(t *testing.T) {
	fake := &queryRound6Datasource{databases: func(context.Context) ([]string, error) {
		return []string{"public", "private"}, nil
	}}
	putRound6Datasource(t, "iotdb", 660001, fake)
	withMetaHook(t, func(_ *gin.Context, f *models.QueryParam, got []string) ([]string, error) {
		require.Equal(t, int64(660001), f.DatasourceId)
		require.Equal(t, []string{"public", "private"}, got)
		return got[:1], nil
	})
	rt := &Router{}

	w := identityRequest(t, rt, rt.ShowDatabases, models.QueryParam{Cate: "iotdb", DatasourceId: 660001}, "0", nil)

	require.JSONEq(t, `{"dat":["public"],"err":"","request_id":""}`, w.Body.String())
}

func TestShowTables_UsesFirstDatabaseAndFiltersMetadata(t *testing.T) {
	var database string
	fake := &queryRound6Datasource{tables: func(_ context.Context, db string) ([]string, error) {
		database = db
		return []string{"visible", "hidden"}, nil
	}}
	putRound6Datasource(t, "iotdb", 660001, fake)
	withMetaHook(t, func(_ *gin.Context, _ *models.QueryParam, got []string) ([]string, error) { return got[:1], nil })
	rt := &Router{}

	w := identityRequest(t, rt, rt.ShowTables, models.QueryParam{
		Cate: "iotdb", DatasourceId: 660001, Queries: []interface{}{"app", "ignored"},
	}, "0", nil)

	require.Equal(t, "app", database)
	require.JSONEq(t, `{"dat":["visible"],"err":"","request_id":""}`, w.Body.String())
}

func TestShowTables_RejectsInvalidDatabaseWithoutQuerying(t *testing.T) {
	cases := []struct {
		name    string
		queries []interface{}
	}{
		{name: "missing"},
		{name: "not string", queries: []interface{}{42}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			fake := &queryRound6Datasource{tables: func(context.Context, string) ([]string, error) { calls++; return nil, nil }}
			putRound6Datasource(t, "iotdb", 660001, fake)
			rt := &Router{}

			w := identityRequest(t, rt, rt.ShowTables, models.QueryParam{
				Cate: "iotdb", DatasourceId: 660001, Queries: tc.queries,
			}, "0", nil)

			require.JSONEq(t, `{"dat":[],"err":"","request_id":""}`, w.Body.String())
			require.Zero(t, calls)
		})
	}
}

func TestDatasourceMetadata_RejectsHookDeniedResults(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*Router) gin.HandlerFunc
	}{
		{name: "databases", handler: func(r *Router) gin.HandlerFunc { return r.ShowDatabases }},
		{name: "tables", handler: func(r *Router) gin.HandlerFunc { return r.ShowTables }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &queryRound6Datasource{
				databases: func(context.Context) ([]string, error) { return []string{"secret"}, nil },
				tables:    func(context.Context, string) ([]string, error) { return []string{"secret"}, nil },
			}
			putRound6Datasource(t, "iotdb", 660001, fake)
			withMetaHook(t, func(*gin.Context, *models.QueryParam, []string) ([]string, error) {
				return nil, errors.New("access denied")
			})
			rt := &Router{}

			w := identityRequest(t, rt, tc.handler(rt), models.QueryParam{
				Cate: "iotdb", DatasourceId: 660001, Queries: []interface{}{"app"},
			}, "0", nil)

			require.Equal(t, http.StatusForbidden, w.Code)
			require.Equal(t, "access denied", w.Body.String())
			require.NotContains(t, w.Body.String(), "secret")
		})
	}
}

func TestDescribeTable_UsesFirstQueryAndReturnsColumns(t *testing.T) {
	var query interface{}
	want := []*types.ColumnProperty{{Field: "host", Type: "String"}}
	fake := &queryRound6Datasource{columns: func(_ context.Context, q interface{}) ([]*types.ColumnProperty, error) {
		query = q
		return want, nil
	}}
	putRound6Datasource(t, "iotdb", 660001, fake)
	rt := &Router{}

	w := identityRequest(t, rt, rt.DescribeTable, models.QueryParam{
		Cate: "iotdb", DatasourceId: 660001, Queries: []interface{}{"app.events", "ignored"},
	}, "0", nil)

	require.Equal(t, "app.events", query)
	var got []*types.ColumnProperty
	require.Empty(t, identityReply(t, w, &got))
	require.Equal(t, want, got)
}

func TestDescribeTable_DenialPreventsDatasourceCall(t *testing.T) {
	calls := 0
	fake := &queryRound6Datasource{columns: func(context.Context, interface{}) ([]*types.ColumnProperty, error) {
		calls++
		return nil, nil
	}}
	putRound6Datasource(t, "iotdb", 660001, fake)
	old := DatasourceDescribeHook
	t.Cleanup(func() { DatasourceDescribeHook = old })
	DatasourceDescribeHook = func(_ *gin.Context, f *models.QueryParam) error {
		require.Equal(t, []interface{}{"private.events"}, f.Queries)
		return errors.New("table denied")
	}
	rt := &Router{}

	w := identityRequest(t, rt, rt.DescribeTable, models.QueryParam{
		Cate: "iotdb", DatasourceId: 660001, Queries: []interface{}{"private.events"},
	}, "0", nil)

	require.Equal(t, http.StatusForbidden, w.Code)
	require.Equal(t, "table denied", w.Body.String())
	require.Zero(t, calls)
}

func TestQueryFunctions_ReportMissingDatasource(t *testing.T) {
	c := queryRound6Context()
	form := models.QueryParam{Cate: "round6-missing", DatasourceId: 660001, Queries: []interface{}{"select 1"}}

	data, dataErr := QueryDataConcurrently(true, c, form)
	logs, logErr := QueryLogConcurrently(true, c, form)
	batch, batchErr := QueryLogBatchConcurrently(true, c, QueryFrom{Queries: []Query{
		{Ref: "A", Did: 660001, DsCate: "round6-missing", Query: map[string]interface{}{"sql": "select 1"}},
	}})

	require.EqualError(t, dataErr, "cluster not exists")
	require.Nil(t, data)
	require.EqualError(t, logErr, "cluster not exists")
	require.Empty(t, logs)
	require.EqualError(t, batchErr, "cluster not exists")
	require.Empty(t, batch)
}

func TestQueryLogBatch_ReturnsGroupedResults(t *testing.T) {
	fake := &queryRound6Datasource{logs: func(_ context.Context, q interface{}) ([]interface{}, int64, error) {
		return []interface{}{map[string]interface{}{"message": q.(map[string]interface{})["sql"]}}, 3, nil
	}}
	putRound6Datasource(t, "iotdb", 660001, fake)
	rt := &Router{}

	w := identityRequest(t, rt, rt.QueryLogBatch, QueryFrom{Queries: []Query{
		{Ref: "A", Did: 660001, DsCate: "iotdb", Query: map[string]interface{}{"sql": "select 1"}},
	}}, "0", &models.User{Username: "alice"})

	require.JSONEq(t, `{"dat":{"total":3,"list":[{
  "ref":"A","ds_id":660001,"ds_cate":"iotdb","data":[{"message":"select 1"}]
 }]},"err":"","request_id":""}`, w.Body.String())
}

func TestQueryLogBatchConcurrently_EmptyQueryListReportsNoData(t *testing.T) {
	got, err := QueryLogBatchConcurrently(true, queryRound6Context(), QueryFrom{})

	require.EqualError(t, err, "no data")
	require.Empty(t, got)
}

func TestDatasourceMetadata_ReportsMissingOrUnsupportedPlugins(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*Router) gin.HandlerFunc
	}{
		{name: "databases", handler: func(r *Router) gin.HandlerFunc { return r.ShowDatabases }},
		{name: "tables", handler: func(r *Router) gin.HandlerFunc { return r.ShowTables }},
		{name: "columns", handler: func(r *Router) gin.HandlerFunc { return r.DescribeTable }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &Router{}

			missing := identityRequest(t, rt, tc.handler(rt), models.QueryParam{Cate: "round6-missing", DatasourceId: 660001}, "0", nil)

			require.Contains(t, missing.Body.String(), "cluster not exists")
			putRound6Datasource(t, "iotdb", 660001, &queryBatchV2FakeDatasource{})

			unsupported := identityRequest(t, rt, tc.handler(rt), models.QueryParam{Cate: "iotdb", DatasourceId: 660001}, "0", nil)

			require.Contains(t, unsupported.Body.String(), "datasource not exists")
		})
	}
}

func TestDatasourceMetadata_PropagatesUpstreamErrors(t *testing.T) {
	cases := []struct {
		name    string
		handler func(*Router) gin.HandlerFunc
	}{
		{name: "databases", handler: func(r *Router) gin.HandlerFunc { return r.ShowDatabases }},
		{name: "tables", handler: func(r *Router) gin.HandlerFunc { return r.ShowTables }},
		{name: "columns", handler: func(r *Router) gin.HandlerFunc { return r.DescribeTable }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := errors.New("metadata unavailable")
			fake := &queryRound6Datasource{
				databases: func(context.Context) ([]string, error) { return nil, want },
				tables:    func(context.Context, string) ([]string, error) { return nil, want },
				columns:   func(context.Context, interface{}) ([]*types.ColumnProperty, error) { return nil, want },
			}
			putRound6Datasource(t, "iotdb", 660001, fake)
			rt := &Router{}

			w := identityRequest(t, rt, tc.handler(rt), models.QueryParam{
				Cate: "iotdb", DatasourceId: 660001, Queries: []interface{}{"events"},
			}, "0", nil)

			require.Equal(t, http.StatusOK, w.Code)
			require.Contains(t, w.Body.String(), want.Error())
		})
	}
}

func TestDescribeTable_EmptyQueryReturnsEmptyColumnsWithoutCallingDatasource(t *testing.T) {
	calls := 0
	fake := &queryRound6Datasource{columns: func(context.Context, interface{}) ([]*types.ColumnProperty, error) {
		calls++
		return nil, nil
	}}
	putRound6Datasource(t, "iotdb", 660001, fake)
	rt := &Router{}

	w := identityRequest(t, rt, rt.DescribeTable, models.QueryParam{Cate: "iotdb", DatasourceId: 660001}, "0", nil)

	var got []*types.ColumnProperty
	require.Empty(t, identityReply(t, w, &got))
	require.NotNil(t, got)
	require.Empty(t, got)
	require.Zero(t, calls)
}

func TestQueryLogBatchConcurrently_RejectsInvalidTemplateBeforeDatasourceCall(t *testing.T) {
	calls := 0
	fake := &queryRound6Datasource{logs: func(context.Context, interface{}) ([]interface{}, int64, error) {
		calls++
		return nil, 0, nil
	}}
	putRound6Datasource(t, "iotdb", 660001, fake)
	form := QueryFrom{Queries: []Query{
		{Ref: "A", Did: 660001, DsCate: "iotdb", Query: map[string]interface{}{"sql": "select {{unknown_template_function}}"}},
	}}

	got, err := QueryLogBatchConcurrently(true, queryRound6Context(), form)

	require.ErrorContains(t, err, "query template execute error")
	require.ErrorContains(t, err, "unknown_template_function")
	require.Empty(t, got)
	require.Zero(t, calls)
}
