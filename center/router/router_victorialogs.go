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

type VictoriaLogsStreamFieldsReq struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id"`
	Query        string `json:"query"`
	Start        int64  `json:"start"`
	End          int64  `json:"end"`
	Filter       string `json:"filter"`
}

type VictoriaLogsStreamFieldValuesReq struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id"`
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

func (rt *Router) QueryVictoriaLogsStreamFields(c *gin.Context) {
	var f VictoriaLogsStreamFieldsReq
	ginx.BindJSON(c, &f)

	validateVictoriaLogsTimeRange(f.Start, f.End)

	if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, f) {
		ginx.Bomb(403, "no permission")
	}

	vl := getVictoriaLogs(c, f.Cate, f.DatasourceId)
	fields, err := vl.StreamFieldNames(c.Request.Context(), f.Query, f.Start, f.End, f.Filter)
	ginx.Dangerous(err)

	ret := make([]string, 0, len(fields))
	for _, field := range fields {
		ret = append(ret, field.Value)
	}
	sort.Strings(ret)

	ginx.NewRender(c).Data(ret, nil)
}

func (rt *Router) QueryVictoriaLogsStreamFieldValues(c *gin.Context) {
	var f VictoriaLogsStreamFieldValuesReq
	ginx.BindJSON(c, &f)

	if f.Field == "" {
		ginx.Bomb(http.StatusBadRequest, "field is required")
	}
	validateVictoriaLogsTimeRange(f.Start, f.End)

	if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, f) {
		ginx.Bomb(403, "no permission")
	}

	vl := getVictoriaLogs(c, f.Cate, f.DatasourceId)
	values, err := vl.StreamFieldValues(c.Request.Context(), f.Query, f.Start, f.End, f.Field, f.Limit, f.Filter)
	ginx.Dangerous(err)

	ret := make([]string, 0, len(values))
	for _, value := range values {
		ret = append(ret, value.Value)
	}

	ginx.NewRender(c).Data(ret, nil)
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
