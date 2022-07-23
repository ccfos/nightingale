package router

import (
	"context"

	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	pkgprom "github.com/didi/nightingale/v5/src/pkg/prom"
	"github.com/didi/nightingale/v5/src/webapi/config"
	"github.com/didi/nightingale/v5/src/webapi/prom"
	"github.com/prometheus/common/model"
)

type queryFormItem struct {
	Start int64  `json:"start" binding:"required"`
	End   int64  `json:"end" binding:"required"`
	Step  int64  `json:"step" binding:"required"`
	Query string `json:"query" binding:"required"`
}

type batchQueryForm struct {
	Queries []queryFormItem `json:"queries" binding:"required"`
}

func promBatchQueryRange(c *gin.Context) {
	xcluster := c.GetHeader("X-Cluster")
	if xcluster == "" {
		ginx.Bomb(http.StatusBadRequest, "header(X-Cluster) is blank")
	}

	var f batchQueryForm
	ginx.Dangerous(c.BindJSON(&f))

	cluster, exist := prom.Clusters.Get(xcluster)
	if !exist {
		ginx.Bomb(http.StatusBadRequest, "cluster(%s) not found", xcluster)
	}

	var lst []model.Value

	for _, item := range f.Queries {
		r := pkgprom.Range{
			Start: time.Unix(item.Start, 0),
			End:   time.Unix(item.End, 0),
			Step:  time.Duration(item.Step) * time.Second,
		}

		resp, _, err := cluster.PromClient.QueryRange(context.Background(), item.Query, r)
		ginx.Dangerous(err)

		lst = append(lst, resp)
	}

	ginx.NewRender(c).Data(lst, nil)
}

func prometheusProxy(c *gin.Context) {
	xcluster := c.GetHeader("X-Cluster")
	if xcluster == "" {
		c.String(http.StatusBadRequest, "X-Cluster missed")
		return
	}

	cluster, exists := prom.Clusters.Get(xcluster)
	if !exists {
		c.String(http.StatusBadRequest, "No such cluster: %s", xcluster)
		return
	}

	target, err := url.Parse(cluster.Opts.Prom)
	if err != nil {
		c.String(http.StatusInternalServerError, "invalid prometheus url: %s", cluster.Opts.Prom)
		return
	}

	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host

		req.Header.Set("Host", target.Host)

		// fe request e.g. /api/n9e/prometheus/api/v1/query
		index := strings.Index(req.URL.Path, "/prometheus")
		if index == -1 {
			panic("url path invalid")
		}

		req.URL.Path = strings.TrimRight(target.Path, "/") + req.URL.Path[index+11:]

		if target.RawQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = target.RawQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = target.RawQuery + "&" + req.URL.RawQuery
		}

		if _, ok := req.Header["User-Agent"]; !ok {
			req.Header.Set("User-Agent", "")
		}

		if cluster.Opts.BasicAuthUser != "" {
			req.SetBasicAuth(cluster.Opts.BasicAuthUser, cluster.Opts.BasicAuthPass)
		}

		headerCount := len(cluster.Opts.Headers)
		if headerCount > 0 && headerCount%2 == 0 {
			for i := 0; i < len(cluster.Opts.Headers); i += 2 {
				req.Header.Add(cluster.Opts.Headers[i], cluster.Opts.Headers[i+1])
				if cluster.Opts.Headers[i] == "Host" {
					req.Host = cluster.Opts.Headers[i+1]
				}
			}
		}
	}

	errFunc := func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, err.Error(), http.StatusBadGateway)
	}

	proxy := &httputil.ReverseProxy{
		Director:     director,
		Transport:    cluster.Transport,
		ErrorHandler: errFunc,
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

func clustersGets(c *gin.Context) {
	count := len(config.C.Clusters)
	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		names = append(names, config.C.Clusters[i].Name)
	}
	ginx.NewRender(c).Data(names, nil)
}
