package calc

import (
	"math"
	"sort"

	"github.com/didi/nightingale/src/common/dataobj"
)

type AggrTsValue struct {
	Value dataobj.JsonFloat
	Count int
}

func Compute(f string, datas []*dataobj.TsdbQueryResponse) []*dataobj.RRDData {
	datasLen := len(datas)
	if datasLen < 1 {
		return nil
	}

	dataMap := make(map[int64]*AggrTsValue)
	switch f {
	case "sum":
		dataMap = sum(datas)
	case "avg":
		dataMap = avg(datas)
	case "max":
		dataMap = max(datas)
	case "min":
		dataMap = min(datas)
	default:
		return nil
	}

	var tmpValues dataobj.RRDValues
	for ts, v := range dataMap {
		d := &dataobj.RRDData{
			Timestamp: ts,
			Value:     v.Value,
		}
		tmpValues = append(tmpValues, d)
	}
	sort.Sort(tmpValues)
	return tmpValues
}

func sum(datas []*dataobj.TsdbQueryResponse) map[int64]*AggrTsValue {
	dataMap := make(map[int64]*AggrTsValue)
	datasLen := len(datas)
	for i := 0; i < datasLen; i++ {
		for j := 0; j < len(datas[i].Values); j++ {
			value := datas[i].Values[j].Value
			if math.IsNaN(float64(value)) {
				continue
			}
			if _, exists := dataMap[datas[i].Values[j].Timestamp]; exists {
				dataMap[datas[i].Values[j].Timestamp].Value += value
			} else {
				dataMap[datas[i].Values[j].Timestamp] = &AggrTsValue{Value: value}
			}
		}
	}
	return dataMap
}

func avg(datas []*dataobj.TsdbQueryResponse) map[int64]*AggrTsValue {
	dataMap := make(map[int64]*AggrTsValue)
	datasLen := len(datas)
	for i := 0; i < datasLen; i++ {
		for j := 0; j < len(datas[i].Values); j++ {
			value := datas[i].Values[j].Value
			if math.IsNaN(float64(value)) {
				continue
			}

			if _, exists := dataMap[datas[i].Values[j].Timestamp]; exists {
				dataMap[datas[i].Values[j].Timestamp].Count += 1
				dataMap[datas[i].Values[j].Timestamp].Value += (datas[i].Values[j].Value - dataMap[datas[i].Values[j].Timestamp].Value) /
					dataobj.JsonFloat(dataMap[datas[i].Values[j].Timestamp].Count)
			} else {
				dataMap[datas[i].Values[j].Timestamp] = &AggrTsValue{Value: value, Count: 1}
			}
		}
	}
	return dataMap
}

func minOrMax(datas []*dataobj.TsdbQueryResponse, fn func(a, b dataobj.JsonFloat) bool) map[int64]*AggrTsValue {
	dataMap := make(map[int64]*AggrTsValue)
	datasLen := len(datas)
	for i := 0; i < datasLen; i++ {
		for j := 0; j < len(datas[i].Values); j++ {
			value := datas[i].Values[j].Value
			if math.IsNaN(float64(value)) {
				continue
			}

			if _, exists := dataMap[datas[i].Values[j].Timestamp]; exists {
				if fn(value, dataMap[datas[i].Values[j].Timestamp].Value) {
					dataMap[datas[i].Values[j].Timestamp].Value = value
				}
			} else {
				dataMap[datas[i].Values[j].Timestamp] = &AggrTsValue{Value: value}
			}
		}
	}
	return dataMap
}

func max(datas []*dataobj.TsdbQueryResponse) map[int64]*AggrTsValue {
	return minOrMax(datas, func(a, b dataobj.JsonFloat) bool { return a > b })
}

func min(datas []*dataobj.TsdbQueryResponse) map[int64]*AggrTsValue {
	return minOrMax(datas, func(a, b dataobj.JsonFloat) bool { return a < b })
}
