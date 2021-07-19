package http

import (
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io/ioutil"

	"github.com/didi/nightingale/v5/backend"
	"github.com/didi/nightingale/v5/cache"
	"github.com/didi/nightingale/v5/trans"
	"github.com/didi/nightingale/v5/vos"

	"github.com/gin-gonic/gin"
	agentpayload "github.com/n9e/agent-payload/gogen"
	"github.com/toolkits/pkg/logger"
)

// 错误消息也是返回了200，是和客户端的约定，客户端如果发现code!=200就会重试
func PushSeries(c *gin.Context) {
	req := agentpayload.N9EMetricsPayload{}

	r := c.Request
	reader := r.Body

	var err error
	if encoding := r.Header.Get("Content-Encoding"); encoding == "gzip" {
		if reader, err = gzip.NewReader(r.Body); err != nil {
			message := fmt.Sprintf("error: get gzip reader occur error: %v", err)
			logger.Warning(message)
			c.String(200, message)
			return
		}
		defer reader.Close()
	} else if encoding == "deflate" {
		if reader, err = zlib.NewReader(r.Body); err != nil {
			message := fmt.Sprintf("error: get zlib reader occur error: %v", err)
			logger.Warning(message)
			c.String(200, message)
			return
		}
		defer reader.Close()
	}

	b, err := ioutil.ReadAll(reader)
	if err != nil {
		message := fmt.Sprintf("error: ioutil occur error: %v", err)
		logger.Warning(message)
		c.String(200, message)
		return
	}

	if r.Header.Get("Content-Type") == "application/x-protobuf" {
		if err := req.Unmarshal(b); err != nil {
			message := fmt.Sprintf("error: decode protobuf body occur error: %v", err)
			logger.Warning(message)
			c.String(200, message)
			return
		}

		count := len(req.Samples)
		if count == 0 {
			c.String(200, "error: samples is empty")
			return
		}

		metricPoints := make([]*vos.MetricPoint, 0, count)
		for i := 0; i < count; i++ {
			logger.Debugf("recv %v", req.Samples[i])
			metricPoints = append(metricPoints, convertAgentdPoint(req.Samples[i]))
		}

		if err = trans.Push(metricPoints); err != nil {
			logger.Warningf("error: trans.push %+v err:%v", req.Samples, err)
			c.String(200, "error: "+err.Error())
		} else {
			c.String(200, "success: received %d points", len(metricPoints))
		}
	} else {
		logger.Warningf("error: trans.push %+v Content-Type(%s) not equals application/x-protobuf", req.Samples)
		c.String(200, "error: Content-Type(%s) not equals application/x-protobuf")
	}
}

func convertAgentdPoint(obj *agentpayload.N9EMetricsPayload_Sample) *vos.MetricPoint {
	return &vos.MetricPoint{
		Metric:       obj.Metric,
		Ident:        obj.Ident,
		Alias:        obj.Alias,
		TagsMap:      obj.Tags,
		Time:         obj.Time,
		ValueUntyped: obj.Value,
	}
}

func PushData(c *gin.Context) {
	var points []*vos.MetricPoint
	err := c.ShouldBindJSON(&points)
	if err != nil {
		message := fmt.Sprintf("error: decode json body occur error: %v", err)
		logger.Warning(message)
		c.String(200, message)
		return
	}

	if err = trans.Push(points); err != nil {
		c.String(200, "error: "+err.Error())
	} else {
		c.String(200, "success")
	}
}

func GetTagKeys(c *gin.Context) {
	recv := vos.CommonTagQueryParam{}
	dangerous(c.ShouldBindJSON(&recv))

	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}

	resp := dataSource.QueryTagKeys(recv)
	renderData(c, resp, nil)
}

func GetTagValues(c *gin.Context) {
	recv := vos.CommonTagQueryParam{}
	dangerous(c.ShouldBindJSON(&recv))

	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}
	if recv.TagKey == "" {
		renderMessage(c, errors.New("missing tag_key"))
		return
	}
	resp := dataSource.QueryTagValues(recv)
	renderData(c, resp, nil)
}

func GetMetrics(c *gin.Context) {
	recv := vos.MetricQueryParam{}
	dangerous(c.ShouldBindJSON(&recv))

	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}

	resp := dataSource.QueryMetrics(recv)
	logger.Debugf("[GetMetrics][recv:%+v][resp:%+v]", recv, resp)
	res := &vos.MetricDesQueryResp{
		Metrics: make([]vos.MetricsWithDescription, 0),
	}

	for _, metric := range resp.Metrics {
		t := vos.MetricsWithDescription{
			Name: metric,
		}

		description, exists := cache.MetricDescMapper.Get(metric)
		if exists {
			t.Description = description.(string)
		}

		res.Metrics = append(res.Metrics, t)
	}

	renderData(c, res, nil)
}

func GetTagPairs(c *gin.Context) {
	recv := vos.CommonTagQueryParam{}
	dangerous(c.ShouldBindJSON(&recv))

	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}

	resp := dataSource.QueryTagPairs(recv)
	renderData(c, resp, nil)
}

func GetData(c *gin.Context) {
	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}

	var input vos.DataQueryParam
	dangerous(c.ShouldBindJSON(&input))
	resp := dataSource.QueryData(input)
	renderData(c, resp, nil)
}

func GetDataInstant(c *gin.Context) {
	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}

	var input vos.DataQueryInstantParam
	dangerous(c.ShouldBindJSON(&input))
	resp := dataSource.QueryDataInstant(input.PromeQl)
	renderData(c, resp, nil)
}
