package router

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/webapi/config"
	"github.com/didi/nightingale/v5/src/webapi/prom"
)

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
