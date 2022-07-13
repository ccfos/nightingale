package router

import (
	"context"
	"encoding/json"
	. "github.com/didi/nightingale/v5/src/pkg/common"
	"github.com/didi/nightingale/v5/src/webapi/reader"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"

	"github.com/didi/nightingale/v5/src/webapi/config"
	"github.com/didi/nightingale/v5/src/webapi/prom"
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

type batchQueryRes struct {
	Status string            `json:"status"`
	Data   []json.RawMessage `json:"data"`
}

func promBatchQueryRange(c *gin.Context) {
	target, cluster := getClusterAndTarget(c)
	var f batchQueryForm
	err := c.BindJSON(&f)
	if err != nil {
		c.String(500, "%s", err.Error())
	}
	res, err := batchQueryRange(target, cluster, f.Queries)

	ginx.NewRender(c).Data(res, err)
}

func prometheusProxy(c *gin.Context) {

	target, cluster := getClusterAndTarget(c)

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

func batchQueryRange(target *url.URL, cluster *prom.ClusterType, data []queryFormItem) (batchQueryRes, error) {
	var res batchQueryRes

	req, err := http.NewRequest("GET", target.Path, nil)
	if err != nil {
		return res, err
	}
	req.URL.Scheme = target.Scheme
	req.URL.Host = target.Host
	req.Header.Set("Host", target.Host)

	req.URL.Path = strings.TrimRight(target.Path, "/") + "/api/v1/query_range"

	if target.RawQuery != "" {
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

	for _, item := range data {
		q := req.URL.Query()
		q.Set("query", item.Query)
		q.Set("end", strconv.FormatInt(item.End, 10))
		q.Set("step", strconv.FormatInt(item.Step, 10))
		q.Set("start", strconv.FormatInt(item.Start, 10))
		req.URL.RawQuery = q.Encode()
		resp, body, err := reader.Client.Do(context.Background(), req)

		if err != nil {
			return res, err
		}

		code := resp.StatusCode

		if code/100 != 2 && !ApiError(code) {
			errorType, errorMsg := ErrorTypeAndMsgFor(resp)
			return batchQueryRes{}, &Error{
				Type:   errorType,
				Msg:    errorMsg,
				Detail: string(body),
			}
		}

		var result ApiResponse

		if http.StatusNoContent != code {
			if jsonErr := json.Unmarshal(body, &result); jsonErr != nil {
				return batchQueryRes{}, &Error{
					Type: ErrBadResponse,
					Msg:  jsonErr.Error(),
				}
			}
		}

		if ApiError(code) != (result.Status == "error") {
			err = &Error{
				Type: ErrBadResponse,
				Msg:  "inconsistent body for response code",
			}
		}

		if ApiError(code) && result.Status == "error" {
			err = &Error{
				Type: result.ErrorType,
				Msg:  result.Error,
			}
		}

		res.Data = append(res.Data, result.Data)
	}
	res.Status = "success"
	return res, nil
}

func getClusterAndTarget(c *gin.Context) (*url.URL, *prom.ClusterType) {
	xcluster := c.GetHeader("X-Cluster")
	if xcluster == "" {
		c.String(http.StatusBadRequest, "X-Cluster missed")
		return nil, nil
	}

	cluster, exists := prom.Clusters.Get(xcluster)
	if !exists {
		c.String(http.StatusBadRequest, "No such cluster: %s", xcluster)
		return nil, nil
	}

	target, err := url.Parse(cluster.Opts.Prom)
	if err != nil {
		c.String(http.StatusInternalServerError, "invalid prometheus url: %s", cluster.Opts.Prom)
		return nil, nil
	}
	return target, cluster
}
