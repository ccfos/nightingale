package router

import (
	"net/http"
	"sort"
	"strings"

	"github.com/ccfos/nightingale/v6/datasource/victorialogs"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/logx"

	"github.com/gin-gonic/gin"
)

const (
	victoriaLogsDefaultFieldNamesLimit  = 50
	victoriaLogsDefaultFieldValuesLimit = 20
)

type VictoriaLogsFieldNamesReq struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id"`
	Scope        string `json:"scope"`
	Query        string `json:"query"`
	Start        int64  `json:"start"`
	End          int64  `json:"end"`
	Filter       string `json:"filter"`
	Limit        int    `json:"limit"`
}

type VictoriaLogsFieldValuesReq struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id"`
	Scope        string `json:"scope"`
	Query        string `json:"query"`
	Start        int64  `json:"start"`
	End          int64  `json:"end"`
	Field        string `json:"field"`
	Limit        int    `json:"limit"`
	Filter       string `json:"filter"`
}

type VictoriaLogsHistogramReq struct {
	Cate         string                        `json:"cate"`
	DatasourceId int64                         `json:"datasource_id"`
	Query        []victorialogs.HistogramQuery `json:"query"`
}

func (rt *Router) QueryVictoriaLogsFieldNames(c *gin.Context) {
	var f VictoriaLogsFieldNamesReq
	ginx.BindJSON(c, &f)

	if strings.TrimSpace(f.Query) == "" {
		ginx.Bomb(http.StatusBadRequest, "query is required")
	}
	validateVictoriaLogsTimeRange(f.Start, f.End)
	f.Limit = validateVictoriaLogsLimit(f.Limit, victoriaLogsDefaultFieldNamesLimit)
	f.Scope = normalizeVictoriaLogsFieldScope(f.Scope)

	if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, f) {
		ginx.Bomb(403, "no permission")
	}

	vl := getVictoriaLogs(c, f.Cate, f.DatasourceId)
	if f.Scope == "stream_field" {
		fields, err := vl.StreamFieldNames(c.Request.Context(), f.Query, f.Start, f.End, f.Filter)
		ginx.Dangerous(err)

		ret := make([]string, 0, len(fields))
		for _, field := range fields {
			ret = append(ret, field.Value)
		}
		sort.Strings(ret)

		ginx.NewRender(c).Data(ret, nil)
		return
	}

	fields, err := vl.QueryFieldNames(c.Request.Context(), f.Query, f.Start, f.End, f.Limit, f.Filter)
	ginx.Dangerous(err)

	ret := make([]victorialogs.FieldName, 0, len(fields))
	for _, field := range fields {
		if isVictoriaLogsBuilderSuggestionBlockedField(field.Field) {
			continue
		}
		field.Builtin = false
		ret = append(ret, field)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Field < ret[j].Field
	})

	ginx.NewRender(c).Data(ret, nil)
}

func (rt *Router) QueryVictoriaLogsFieldValues(c *gin.Context) {
	var f VictoriaLogsFieldValuesReq
	ginx.BindJSON(c, &f)

	if strings.TrimSpace(f.Query) == "" {
		ginx.Bomb(http.StatusBadRequest, "query is required")
	}
	validateVictoriaLogsField(f.Field)
	validateVictoriaLogsTimeRange(f.Start, f.End)
	f.Limit = validateVictoriaLogsLimit(f.Limit, victoriaLogsDefaultFieldValuesLimit)
	f.Scope = normalizeVictoriaLogsFieldScope(f.Scope)

	if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, f) {
		ginx.Bomb(403, "no permission")
	}

	vl := getVictoriaLogs(c, f.Cate, f.DatasourceId)
	if f.Scope == "stream_field" {
		values, err := vl.StreamFieldValues(c.Request.Context(), f.Query, f.Start, f.End, f.Field, f.Limit, f.Filter)
		ginx.Dangerous(err)

		ret := make([]string, 0, len(values))
		for _, value := range values {
			ret = append(ret, value.Value)
		}

		ginx.NewRender(c).Data(ret, nil)
		return
	}

	if isVictoriaLogsBuilderSuggestionBlockedField(f.Field) {
		ginx.NewRender(c).Data([]victorialogs.FieldValue{}, nil)
		return
	}

	values, err := vl.QueryFieldValues(c.Request.Context(), f.Query, f.Start, f.End, f.Field, f.Limit, f.Filter)
	ginx.Dangerous(err)

	ginx.NewRender(c).Data(values, nil)
}

func (rt *Router) QueryVictoriaLogsHistogram(c *gin.Context) {
	var f VictoriaLogsHistogramReq
	ginx.BindJSON(c, &f)

	if len(f.Query) == 0 {
		ginx.Bomb(http.StatusBadRequest, "query is required")
	}

	for _, q := range f.Query {
		if strings.TrimSpace(q.Query) == "" {
			ginx.Bomb(http.StatusBadRequest, "query is required")
		}
		validateVictoriaLogsTimeRange(q.Start, q.End)
	}

	vl := getVictoriaLogs(c, f.Cate, f.DatasourceId)
	ret := make([]victorialogs.HistogramValues, 0)

	for _, q := range f.Query {
		if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, q) {
			ginx.Bomb(403, "no permission")
		}

		data, err := vl.QueryHistogram(c.Request.Context(), q)
		ginx.Dangerous(err)
		ret = append(ret, data...)
	}

	ginx.NewRender(c).Data(ret, nil)
}

func validateVictoriaLogsTimeRange(start, end int64) {
	if start <= 0 || end <= start {
		ginx.Bomb(http.StatusBadRequest, "invalid time range")
	}
}

func validateVictoriaLogsLimit(limit, defaultLimit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func normalizeVictoriaLogsFieldScope(scope string) string {
	switch scope {
	case "", "field":
		return "field"
	case "stream_field":
		return "stream_field"
	default:
		ginx.Bomb(http.StatusBadRequest, "invalid scope")
		return "field"
	}
}

func validateVictoriaLogsField(field string) {
	if strings.TrimSpace(field) == "" {
		ginx.Bomb(http.StatusBadRequest, "field is required")
	}
	if len(field) > 512 {
		ginx.Bomb(http.StatusBadRequest, "field is too long")
	}
	for _, r := range field {
		if r < 32 || r == 127 {
			ginx.Bomb(http.StatusBadRequest, "invalid field")
		}
	}
}

func isVictoriaLogsBuilderSuggestionBlockedField(field string) bool {
	switch field {
	case "_time", "_msg", "_stream", "_stream_id":
		return true
	}
	if strings.HasPrefix(field, "_stream.") {
		return true
	}
	return false
}

func getVictoriaLogs(c *gin.Context, cate string, datasourceId int64) *victorialogs.VictoriaLogs {
	plug, exists := dscache.DsCache.Get(cate, datasourceId)
	if !exists {
		logx.Warningf(c.Request.Context(), "cluster:%d not exists", datasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	vl, ok := plug.(*victorialogs.VictoriaLogs)
	if !ok {
		ginx.Bomb(200, "cluster not victorialogs")
	}

	return vl
}
