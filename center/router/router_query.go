package router

import (
	"fmt"
	"sort"
	"sync"

	"github.com/ccfos/nightingale/v6/alert/eval"
	"github.com/ccfos/nightingale/v6/dscache"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

func CheckDsPerm(c *gin.Context, dsId int64, cate string, q interface{}) bool {
	// todo: 后续需要根据 cate 判断是否需要权限
	return true
}

type QueryFrom struct {
	Queries []Query `json:"queries"`
	Exps    []Exp   `json:"exps"`
}

type Query struct {
	Ref    string      `json:"ref"`
	Did    int64       `json:"ds_id"`
	DsCate string      `json:"ds_cate"`
	Query  interface{} `json:"query"`
}

type Exp struct {
	Exp string `json:"exp"`
	Ref string `json:"ref"`
}

type LogResp struct {
	Total int64         `json:"total"`
	List  []interface{} `json:"list"`
}

func QueryLogBatchConcurrently(anonymousAccess bool, ctx *gin.Context, f QueryFrom) (LogResp, error) {
	var resp LogResp
	var mu sync.Mutex
	var wg sync.WaitGroup
	var errs []error

	for _, q := range f.Queries {
		if !anonymousAccess && !CheckDsPerm(ctx, q.Did, q.DsCate, q) {
			return LogResp{}, fmt.Errorf("forbidden")
		}

		plug, exists := dscache.DsCache.Get(q.DsCate, q.Did)
		if !exists {
			logger.Warningf("cluster:%d not exists query:%+v", q.Did, q)
			return LogResp{}, fmt.Errorf("cluster not exists")
		}

		// 根据数据源类型对 Query 进行模板渲染处理
		err := eval.ExecuteQueryTemplate(q.DsCate, q.Query, nil)
		if err != nil {
			logger.Warningf("query template execute error: %v", err)
			return LogResp{}, fmt.Errorf("query template execute error: %v", err)
		}

		wg.Add(1)
		go func(query Query) {
			defer wg.Done()

			data, total, err := plug.QueryLog(ctx.Request.Context(), query.Query)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errMsg := fmt.Sprintf("query data error: %v query:%v\n ", err, query)
				logger.Warningf(errMsg)
				errs = append(errs, err)
				return
			}

			m := make(map[string]interface{})
			m["ref"] = query.Ref
			m["ds_id"] = query.Did
			m["ds_cate"] = query.DsCate
			m["data"] = data

			resp.List = append(resp.List, m)
			resp.Total += total
		}(q)
	}

	wg.Wait()

	if len(errs) > 0 {
		return LogResp{}, errs[0]
	}

	if len(resp.List) == 0 {
		return LogResp{}, fmt.Errorf("no data")
	}

	return resp, nil
}

func (rt *Router) QueryLogBatch(c *gin.Context) {
	var f QueryFrom
	ginx.BindJSON(c, &f)

	resp, err := QueryLogBatchConcurrently(rt.Center.AnonymousAccess.PromQuerier, c, f)
	if err != nil {
		ginx.Bomb(200, "err:%v", err)
	}

	ginx.NewRender(c).Data(resp, nil)
}

func QueryDataConcurrently(anonymousAccess bool, ctx *gin.Context, f models.QueryParam) ([]models.DataResp, error) {
	var resp []models.DataResp
	var mu sync.Mutex
	var wg sync.WaitGroup
	var errs []error

	for _, q := range f.Queries {
		if !anonymousAccess && !CheckDsPerm(ctx, f.DatasourceId, f.Cate, q) {
			return nil, fmt.Errorf("forbidden")
		}

		plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
		if !exists {
			logger.Warningf("cluster:%d not exists", f.DatasourceId)
			return nil, fmt.Errorf("cluster not exists")
		}

		wg.Add(1)
		go func(query interface{}) {
			defer wg.Done()

			data, err := plug.QueryData(ctx.Request.Context(), query)
			if err != nil {
				logger.Warningf("query data error: req:%+v err:%v", query, err)
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}

			logger.Debugf("query data: req:%+v resp:%+v", query, data)
			mu.Lock()
			resp = append(resp, data...)
			mu.Unlock()
		}(q)
	}

	wg.Wait()

	if len(errs) > 0 {
		return nil, errs[0]
	}

	// 面向API的统一处理
	// 按照 .Metric 排序
	// 确保仪表盘中相同图例的曲线颜色相同
	if len(resp) > 1 {
		sort.Slice(resp, func(i, j int) bool {
			if resp[i].Metric != nil && resp[j].Metric != nil {
				return resp[i].Metric.String() < resp[j].Metric.String()
			}
			return false
		})
	}

	return resp, nil
}

func (rt *Router) QueryData(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	resp, err := QueryDataConcurrently(rt.Center.AnonymousAccess.PromQuerier, c, f)
	if err != nil {
		ginx.Bomb(200, "err:%v", err)
	}

	ginx.NewRender(c).Data(resp, nil)
}

// QueryLogConcurrently 并发查询日志
func QueryLogConcurrently(anonymousAccess bool, ctx *gin.Context, f models.QueryParam) (LogResp, error) {
	var resp LogResp
	var mu sync.Mutex
	var wg sync.WaitGroup
	var errs []error

	for _, q := range f.Queries {
		if !anonymousAccess && !CheckDsPerm(ctx, f.DatasourceId, f.Cate, q) {
			return LogResp{}, fmt.Errorf("forbidden")
		}

		plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
		if !exists {
			logger.Warningf("cluster:%d not exists query:%+v", f.DatasourceId, f)
			return LogResp{}, fmt.Errorf("cluster not exists")
		}

		wg.Add(1)
		go func(query interface{}) {
			defer wg.Done()

			data, total, err := plug.QueryLog(ctx.Request.Context(), query)
			logger.Debugf("query log: req:%+v resp:%+v", query, data)
			if err != nil {
				errMsg := fmt.Sprintf("query data error: %v query:%v\n ", err, query)
				logger.Warningf(errMsg)
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
				return
			}

			mu.Lock()
			resp.List = append(resp.List, data...)
			resp.Total += total
			mu.Unlock()
		}(q)
	}

	wg.Wait()

	if len(errs) > 0 {
		return LogResp{}, errs[0]
	}

	if len(resp.List) == 0 {
		return LogResp{}, fmt.Errorf("no data")
	}

	return resp, nil
}

func (rt *Router) QueryLogV2(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	resp, err := QueryLogConcurrently(rt.Center.AnonymousAccess.PromQuerier, c, f)
	ginx.NewRender(c).Data(resp, err)
}

func (rt *Router) QueryLog(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	var resp []interface{}
	for _, q := range f.Queries {
		if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, q) {
			ginx.Bomb(200, "forbidden")
		}

		plug, exists := dscache.DsCache.Get("elasticsearch", f.DatasourceId)
		if !exists {
			logger.Warningf("cluster:%d not exists", f.DatasourceId)
			ginx.Bomb(200, "cluster not exists")
		}

		data, _, err := plug.QueryLog(c.Request.Context(), q)
		if err != nil {
			logger.Warningf("query data error: %v", err)
			ginx.Bomb(200, "err:%v", err)
			continue
		}
		resp = append(resp, data...)
	}

	ginx.NewRender(c).Data(resp, nil)
}
