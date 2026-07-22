package router

import (
	"net/http"
	"sort"
	"strings"

	"github.com/ccfos/nightingale/v6/datasource/loki"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/ccfos/nightingale/v6/pkg/logx"

	"github.com/gin-gonic/gin"
)

const (
	lokiDefaultLabelNamesLimit  = 100
	lokiDefaultLabelValuesLimit = 100
	lokiDefaultParsedLimit      = 200
	lokiMaxParsedLimit          = 500
)

type LokiLabelNamesReq struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id"`
	Query        string `json:"query"`
	Start        int64  `json:"start"`
	End          int64  `json:"end"`
	Filter       string `json:"filter"`
	Limit        int    `json:"limit"`
}

type LokiLabelValuesReq struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id"`
	Query        string `json:"query"`
	Start        int64  `json:"start"`
	End          int64  `json:"end"`
	Label        string `json:"label"`
	Filter       string `json:"filter"`
	Limit        int    `json:"limit"`
}

type LokiParsedFieldsReq struct {
	Cate         string `json:"cate"`
	DatasourceId int64  `json:"datasource_id"`
	Query        string `json:"query"`
	Start        int64  `json:"start"`
	End          int64  `json:"end"`
	Limit        int    `json:"limit"`
}

type LokiHistogramReq struct {
	Cate         string                `json:"cate"`
	DatasourceId int64                 `json:"datasource_id"`
	Query        []loki.HistogramQuery `json:"query"`
}

func (rt *Router) QueryLokiLabelNames(c *gin.Context) {
	var f LokiLabelNamesReq
	ginx.BindJSON(c, &f)

	validateLokiTimeRange(f.Start, f.End)
	f.Limit = validateLokiLimit(f.Limit, lokiDefaultLabelNamesLimit)
	if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, f) {
		ginx.Bomb(403, "no permission")
	}

	vl := getLoki(c, f.Cate, f.DatasourceId)
	ctx := withCallContext(c.Request.Context(), f.DatasourceId, ginUser(c))
	ret, err := vl.QueryLabelNames(ctx, f.Query, f.Start, f.End, f.Filter, f.Limit)
	ginx.Dangerous(err)
	sort.Strings(ret)

	ginx.NewRender(c).Data(ret, nil)
}

func (rt *Router) QueryLokiLabelValues(c *gin.Context) {
	var f LokiLabelValuesReq
	ginx.BindJSON(c, &f)

	validateLokiLabel(f.Label)
	validateLokiTimeRange(f.Start, f.End)
	f.Limit = validateLokiLimit(f.Limit, lokiDefaultLabelValuesLimit)
	if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, f) {
		ginx.Bomb(403, "no permission")
	}

	vl := getLoki(c, f.Cate, f.DatasourceId)
	ctx := withCallContext(c.Request.Context(), f.DatasourceId, ginUser(c))
	ret, err := vl.QueryLabelValues(ctx, f.Query, f.Start, f.End, f.Label, f.Filter, f.Limit)
	ginx.Dangerous(err)
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Value < ret[j].Value
	})

	ginx.NewRender(c).Data(ret, nil)
}

func (rt *Router) QueryLokiParsedFields(c *gin.Context) {
	var f LokiParsedFieldsReq
	ginx.BindJSON(c, &f)

	if strings.TrimSpace(f.Query) == "" {
		ginx.Bomb(http.StatusBadRequest, "query is required")
	}
	validateLokiTimeRange(f.Start, f.End)
	f.Limit = validateLokiParsedLimit(f.Limit)
	if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, f) {
		ginx.Bomb(403, "no permission")
	}

	vl := getLoki(c, f.Cate, f.DatasourceId)
	ctx := withCallContext(c.Request.Context(), f.DatasourceId, ginUser(c))
	ret, err := vl.QueryParsedFields(ctx, f.Query, f.Start, f.End, f.Limit)
	ginx.Dangerous(err)
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Field < ret[j].Field
	})

	ginx.NewRender(c).Data(ret, nil)
}

func (rt *Router) QueryLokiHistogram(c *gin.Context) {
	var f LokiHistogramReq
	ginx.BindJSON(c, &f)

	if len(f.Query) == 0 {
		ginx.Bomb(http.StatusBadRequest, "query is required")
	}

	vl := getLoki(c, f.Cate, f.DatasourceId)
	ret := make([]loki.HistogramValues, 0)
	for _, q := range f.Query {
		if strings.TrimSpace(q.Query) == "" {
			ginx.Bomb(http.StatusBadRequest, "query is required")
		}
		validateLokiTimeRange(q.Start, q.End)
		if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, q) {
			ginx.Bomb(403, "no permission")
		}

		ctx := withCallContext(c.Request.Context(), f.DatasourceId, ginUser(c))
		data, err := vl.QueryHistogram(ctx, q)
		ginx.Dangerous(err)
		ret = append(ret, data...)
	}

	ginx.NewRender(c).Data(ret, nil)
}

func validateLokiTimeRange(start, end int64) {
	if start <= 0 || end <= start {
		ginx.Bomb(http.StatusBadRequest, "invalid time range")
	}
}

func validateLokiLimit(limit, defaultLimit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > 1000 {
		return 1000
	}
	return limit
}

func validateLokiParsedLimit(limit int) int {
	if limit <= 0 {
		return lokiDefaultParsedLimit
	}
	if limit > lokiMaxParsedLimit {
		return lokiMaxParsedLimit
	}
	return limit
}

func validateLokiLabel(label string) {
	if strings.TrimSpace(label) == "" {
		ginx.Bomb(http.StatusBadRequest, "label is required")
	}
	if len(label) > 512 {
		ginx.Bomb(http.StatusBadRequest, "label is too long")
	}
	for _, r := range label {
		if r < 32 || r == 127 {
			ginx.Bomb(http.StatusBadRequest, "invalid label")
		}
	}
}

func getLoki(c *gin.Context, cate string, datasourceId int64) *loki.Loki {
	plug, exists := dscache.DsCache.Get(cate, datasourceId)
	if !exists {
		logx.Warningf(c.Request.Context(), "cluster:%d not exists", datasourceId)
		ginx.Bomb(200, "cluster not exists")
	}

	vl, ok := plug.(*loki.Loki)
	if !ok {
		ginx.Bomb(200, "cluster not loki")
	}

	return vl
}
