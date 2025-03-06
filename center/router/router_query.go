package router

import (
	"fmt"
	"sort"

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

func (rt *Router) QueryLogBatch(c *gin.Context) {
	var f QueryFrom
	ginx.BindJSON(c, &f)

	var resp LogResp
	var errMsg string
	for _, q := range f.Queries {
		if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, q.Did, q.DsCate, q) {
			ginx.Bomb(200, "no permission")
		}

		plug, exists := dscache.DsCache.Get(q.DsCate, q.Did)
		if !exists {
			logger.Warningf("cluster:%d not exists query:%+v", q.Did, q)
			ginx.Bomb(200, "cluster not exists")
		}

		data, total, err := plug.QueryLog(c.Request.Context(), q.Query)
		if err != nil {
			errMsg += fmt.Sprintf("query data error: %v query:%v\n ", err, q)
			logger.Warningf("query data error: %v query:%v", err, q)
			continue
		}

		m := make(map[string]interface{})
		m["ref"] = q.Ref
		m["ds_id"] = q.Did
		m["ds_cate"] = q.DsCate
		m["data"] = data
		resp.List = append(resp.List, m)
		resp.Total += total
	}

	if errMsg != "" || len(resp.List) == 0 {
		ginx.Bomb(200, errMsg)
	}

	ginx.NewRender(c).Data(resp, nil)
}

func (rt *Router) QueryData(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	var resp []models.DataResp
	var err error
	for _, q := range f.Querys {
		if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, q) {
			ginx.Bomb(403, "no permission")
		}

		plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
		if !exists {
			logger.Warningf("cluster:%d not exists", f.DatasourceId)
			ginx.Bomb(200, "cluster not exists")
		}
		var datas []models.DataResp
		datas, err = plug.QueryData(c.Request.Context(), q)
		if err != nil {
			logger.Warningf("query data error: req:%+v err:%v", q, err)
			ginx.Bomb(200, "err:%v", err)
		}
		logger.Debugf("query data: req:%+v resp:%+v", q, datas)
		resp = append(resp, datas...)
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

	ginx.NewRender(c).Data(resp, err)
}

func (rt *Router) QueryLogV2(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	var resp LogResp
	var errMsg string
	for _, q := range f.Querys {
		if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, q) {
			ginx.Bomb(200, "no permission")
		}

		plug, exists := dscache.DsCache.Get(f.Cate, f.DatasourceId)
		if !exists {
			logger.Warningf("cluster:%d not exists query:%+v", f.DatasourceId, f)
			ginx.Bomb(200, "cluster not exists")
		}

		data, total, err := plug.QueryLog(c.Request.Context(), q)
		if err != nil {
			errMsg += fmt.Sprintf("query data error: %v query:%v\n ", err, q)
			logger.Warningf("query data error: %v query:%v", err, q)
			continue
		}
		resp.List = append(resp.List, data...)
		resp.Total += total
	}

	if errMsg != "" || len(resp.List) == 0 {
		ginx.Bomb(200, errMsg)
	}

	ginx.NewRender(c).Data(resp, nil)
}

func (rt *Router) QueryLog(c *gin.Context) {
	var f models.QueryParam
	ginx.BindJSON(c, &f)

	var resp []interface{}
	for _, q := range f.Querys {
		if !rt.Center.AnonymousAccess.PromQuerier && !CheckDsPerm(c, f.DatasourceId, f.Cate, q) {
			ginx.Bomb(200, "no permission")
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
