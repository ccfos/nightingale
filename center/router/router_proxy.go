package router

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	pkgprom "github.com/ccfos/nightingale/v6/pkg/prom"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/ginx"
)

type queryFormItem struct {
	Start int64  `json:"start" binding:"required"`
	End   int64  `json:"end" binding:"required"`
	Step  int64  `json:"step" binding:"required"`
	Query string `json:"query" binding:"required"`
}

type batchQueryForm struct {
	DatasourceId int64           `json:"datasource_id" binding:"required"`
	Queries      []queryFormItem `json:"queries" binding:"required"`
}

func (rt *Router) promBatchQueryRange(c *gin.Context) {
	var f batchQueryForm
	ginx.Dangerous(c.BindJSON(&f))

	cli := rt.PromClients.GetCli(f.DatasourceId)

	var lst []model.Value

	for _, item := range f.Queries {
		r := pkgprom.Range{
			Start: time.Unix(item.Start, 0),
			End:   time.Unix(item.End, 0),
			Step:  time.Duration(item.Step) * time.Second,
		}

		resp, _, err := cli.QueryRange(context.Background(), item.Query, r)
		ginx.Dangerous(err)

		lst = append(lst, resp)
	}

	ginx.NewRender(c).Data(lst, nil)
}

type batchInstantForm struct {
	DatasourceId int64             `json:"datasource_id" binding:"required"`
	Queries      []InstantFormItem `json:"queries" binding:"required"`
}

type InstantFormItem struct {
	Time  int64  `json:"time" binding:"required"`
	Query string `json:"query" binding:"required"`
}

func (rt *Router) promBatchQueryInstant(c *gin.Context) {
	var f batchInstantForm
	ginx.Dangerous(c.BindJSON(&f))

	cli := rt.PromClients.GetCli(f.DatasourceId)

	var lst []model.Value

	for _, item := range f.Queries {
		resp, _, err := cli.Query(context.Background(), item.Query, time.Unix(item.Time, 0))
		ginx.Dangerous(err)

		lst = append(lst, resp)
	}

	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) dsProxy(c *gin.Context) {
	dsId := ginx.UrlParamInt64(c, "id")
	ds := rt.DatasourceCache.GetById(dsId)

	if ds == nil {
		c.String(http.StatusBadRequest, "no such datasource")
		return
	}

	target, err := url.Parse(ds.HTTPJson.Url)
	if err != nil {
		c.String(http.StatusInternalServerError, "invalid  url: %s", ds.HTTPJson.Url)
		return
	}

	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host

		req.Header.Set("Host", target.Host)

		// fe request e.g. /api/n9e/proxy/:id/*
		arr := strings.Split(req.URL.Path, "/")
		if len(arr) < 6 {
			c.String(http.StatusBadRequest, "invalid url path")
			return
		}

		req.URL.Path = strings.TrimRight(target.Path, "/") + "/" + strings.Join(arr[5:], "/")
		if target.RawQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = target.RawQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = target.RawQuery + "&" + req.URL.RawQuery
		}

		if _, ok := req.Header["User-Agent"]; !ok {
			req.Header.Set("User-Agent", "")
		}

		if ds.AuthJson.BasicAuthUser != "" {
			req.SetBasicAuth(ds.AuthJson.BasicAuthUser, ds.AuthJson.BasicAuthPassword)
		}

		headerCount := len(ds.HTTPJson.Headers)
		if headerCount > 0 {
			for key, value := range ds.HTTPJson.Headers {
				req.Header.Set(key, value)
				if key == "Host" {
					req.Host = value
				}
			}
		}
	}

	errFunc := func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, err.Error(), http.StatusBadGateway)
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: ds.HTTPJson.TLS.SkipTlsVerify},
		Proxy:           http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout: time.Duration(ds.HTTPJson.DialTimeout) * time.Millisecond,
		}).DialContext,
		ResponseHeaderTimeout: time.Duration(ds.HTTPJson.Timeout) * time.Millisecond,
		MaxIdleConnsPerHost:   ds.HTTPJson.MaxIdleConnsPerHost,
	}

	proxy := &httputil.ReverseProxy{
		Director:     director,
		Transport:    transport,
		ErrorHandler: errFunc,
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}
