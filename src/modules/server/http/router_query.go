package http

import (
	"github.com/didi/nightingale/v4/src/common/dataobj"
	statsd "github.com/didi/nightingale/v4/src/common/stats"
	"github.com/didi/nightingale/v4/src/modules/server/backend"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
)

func QueryData(c *gin.Context) {
	statsd.Counter.Set("data.api.qp10s", 1)

	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}

	var input []dataobj.QueryData
	errors.Dangerous(c.ShouldBindJSON(&input))
	resp := dataSource.QueryData(input)
	renderData(c, resp, nil)
}

func QueryDataForUI(c *gin.Context) {
	statsd.Counter.Set("data.ui.qp10s", 1)
	var input dataobj.QueryDataForUI
	var respData []*dataobj.QueryDataForUIResp

	dangerous(c.ShouldBindJSON(&input))
	start := input.Start
	end := input.End

	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}
	resp := dataSource.QueryDataForUI(input)
	for _, d := range resp {
		data := &dataobj.QueryDataForUIResp{
			Start:    d.Start,
			End:      d.End,
			Endpoint: d.Endpoint,
			Nid:      d.Nid,
			Counter:  d.Counter,
			DsType:   d.DsType,
			Step:     d.Step,
			Values:   d.Values,
		}
		respData = append(respData, data)
	}

	if len(input.Comparisons) > 1 {
		for i := 1; i < len(input.Comparisons); i++ {
			comparison := input.Comparisons[i]
			input.Start = start - comparison
			input.End = end - comparison
			res := dataSource.QueryDataForUI(input)
			for _, d := range res {
				for j := range d.Values {
					d.Values[j].Timestamp += comparison
				}

				data := &dataobj.QueryDataForUIResp{
					Start:      d.Start,
					End:        d.End,
					Endpoint:   d.Endpoint,
					Nid:        d.Nid,
					Counter:    d.Counter,
					DsType:     d.DsType,
					Step:       d.Step,
					Values:     d.Values,
					Comparison: comparison,
				}
				respData = append(respData, data)
			}
		}
	}

	renderData(c, respData, nil)
}

func GetMetrics(c *gin.Context) {
	statsd.Counter.Set("metric.qp10s", 1)
	recv := dataobj.EndpointsRecv{}
	errors.Dangerous(c.ShouldBindJSON(&recv))

	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}

	resp := dataSource.QueryMetrics(recv)

	renderData(c, resp, nil)
}

func GetTagPairs(c *gin.Context) {
	statsd.Counter.Set("tag.qp10s", 1)
	recv := dataobj.EndpointMetricRecv{}
	errors.Dangerous(c.ShouldBindJSON(&recv))

	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}

	resp := dataSource.QueryTagPairs(recv)
	renderData(c, resp, nil)
}

func GetIndexByClude(c *gin.Context) {
	statsd.Counter.Set("xclude.qp10s", 1)
	recvs := make([]dataobj.CludeRecv, 0)
	errors.Dangerous(c.ShouldBindJSON(&recvs))

	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}

	resp := dataSource.QueryIndexByClude(recvs)
	renderData(c, resp, nil)
}

func GetIndexByFullTags(c *gin.Context) {
	statsd.Counter.Set("counter.qp10s", 1)
	recvs := make([]dataobj.IndexByFullTagsRecv, 0)
	errors.Dangerous(c.ShouldBindJSON(&recvs))

	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		renderMessage(c, err)
		return
	}

	resp, count := dataSource.QueryIndexByFullTags(recvs)
	renderData(c, &listResp{List: resp, Count: count}, nil)
}

type listResp struct {
	List  interface{} `json:"list"`
	Count int         `json:"count"`
}
