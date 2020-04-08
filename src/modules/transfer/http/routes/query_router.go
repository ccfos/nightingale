package routes

import (
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/backend"
	"github.com/didi/nightingale/src/toolkits/http/render"
	"github.com/didi/nightingale/src/toolkits/stats"
)

type QueryDataReq struct {
	Start  int64               `json:"start"`
	End    int64               `json:"end"`
	Series []backend.SeriesReq `json:"series"`
}

func QueryDataForJudge(c *gin.Context) {
	var inputs []dataobj.QueryData

	errors.Dangerous(c.ShouldBindJSON(&inputs))
	resp := backend.FetchData(inputs)
	render.Data(c, resp, nil)
}

func QueryData(c *gin.Context) {
	stats.Counter.Set("data.api.qp10s", 1)

	var input QueryDataReq

	errors.Dangerous(c.ShouldBindJSON(&input))

	queryData, err := backend.GetSeries(input.Start, input.End, input.Series)
	if err != nil {
		logger.Error(err, input)
		render.Message(c, "query err")
		return
	}

	resp := backend.FetchData(queryData)
	render.Data(c, resp, nil)
}

func QueryDataForUI(c *gin.Context) {
	stats.Counter.Set("data.ui.qp10s", 1)
	var input dataobj.QueryDataForUI
	var respData []*dataobj.QueryDataForUIResp
	errors.Dangerous(c.ShouldBindJSON(&input))
	start := input.Start
	end := input.End

	resp := backend.FetchDataForUI(input)
	for _, d := range resp {
		data := &dataobj.QueryDataForUIResp{
			Start:    d.Start,
			End:      d.End,
			Endpoint: d.Endpoint,
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
			res := backend.FetchDataForUI(input)
			for _, d := range res {
				for j := range d.Values {
					d.Values[j].Timestamp += comparison
				}

				data := &dataobj.QueryDataForUIResp{
					Start:      d.Start,
					End:        d.End,
					Endpoint:   d.Endpoint,
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

	render.Data(c, respData, nil)
}
