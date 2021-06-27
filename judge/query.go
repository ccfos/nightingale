package judge

import (
	"errors"
	"sort"

	"github.com/didi/nightingale/v5/backend"
	"github.com/didi/nightingale/v5/vos"

	"github.com/toolkits/pkg/logger"
)

var (
	ErrorIndexParamIllegal = errors.New("index param illegal")
	ErrorQueryParamIllegal = errors.New("query param illegal")
)

func queryDataByBackend(args vos.DataQueryParam) []*vos.DataQueryResp {
	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("could not find datasource")
		return nil
	}
	reply := dataSource.QueryData(args)

	return reply
}

// 执行Query操作
// 默认不重试, 如果要做重试, 在这里完成
func Query(reqs *vos.DataQueryParam) []*vos.HPoint {
	hisD := make([]*vos.HPoint, 0)

	// 默认重试
	queryResD := queryDataByBackend(*reqs)

	if len(queryResD) == 0 {
		return hisD
	}
	logger.Debugf("[reqs:%+v][queryResD:%+v]\n", reqs, queryResD[0])
	// TODO 如何判断查询到的多条曲线？ 与条件希望都是配置时就是一条曲线
	fD := queryResD[0]

	var values vos.HistoryDataS

	//裁剪掉多余的点
	for _, i := range fD.Values {
		oneV := &vos.HPoint{
			Timestamp: i.Timestamp,
			Value:     i.Value,
		}
		values = append(values, oneV)
	}

	sort.Sort(values)

	return values
}

func NewQueryRequest(ident, metric string, tagsMap map[string]string,
	start, end int64) (*vos.DataQueryParam, error) {
	if end <= start || start < 0 {
		return nil, ErrorQueryParamIllegal
	}

	tagPairs := make([]*vos.TagPair, 0)
	for k, v := range tagsMap {
		oneKeyV := &vos.TagPair{
			Key:   k,
			Value: v,
		}
		tagPairs = append(tagPairs, oneKeyV)

	}

	paramOne := vos.DataQueryParamOne{
		Idents:   []string{ident},
		Metric:   metric,
		TagPairs: tagPairs,
	}
	paramS := make([]vos.DataQueryParamOne, 0)
	paramS = append(paramS, paramOne)
	return &vos.DataQueryParam{
		Start:  start,
		End:    end,
		Params: paramS,
	}, nil
}
