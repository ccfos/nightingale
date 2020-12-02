package m3db

import (
	"math"
	"strings"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/calc"
	"github.com/didi/nightingale/src/toolkits/str"
	"github.com/m3db/m3/src/query/storage/m3/consolidators"
	"github.com/m3db/m3/src/x/ident"
	"github.com/toolkits/pkg/logger"
)

func mvID(in *dataobj.MetricValue) ident.ID {
	if in.Nid != "" {
		in.Endpoint = dataobj.NidToEndpoint(in.Nid)
	}
	return ident.StringID(str.MD5(in.Endpoint, in.Metric, str.SortedTags(in.TagsMap)))
}

func mvTags(item *dataobj.MetricValue) ident.Tags {
	tags := ident.NewTags()

	for k, v := range item.TagsMap {
		tags.Append(ident.Tag{
			Name:  ident.StringID(k),
			Value: ident.StringID(v),
		})
	}
	if item.Nid != "" {
		tags.Append(ident.Tag{
			Name:  ident.StringID(NID_NAME),
			Value: ident.StringID(item.Nid),
		})
	}
	if item.Endpoint != "" {
		tags.Append(ident.Tag{
			Name:  ident.StringID(ENDPOINT_NAME),
			Value: ident.StringID(item.Endpoint),
		})
	}
	tags.Append(ident.Tag{
		Name:  ident.StringID(METRIC_NAME),
		Value: ident.StringID(item.Metric),
	})

	return tags
}

func tagsMr(tags *consolidators.CompleteTagsResult) *dataobj.MetricResp {
	for _, tag := range tags.CompletedTags {
		if name := string(tag.Name); name == METRIC_NAME {
			metrics := make([]string, len(tag.Values))
			for i, v := range tag.Values {
				metrics[i] = string(v)
			}
			return &dataobj.MetricResp{Metrics: metrics}
		}
	}

	return nil
}

func tagsIndexTagkvResp(tags *consolidators.CompleteTagsResult) *dataobj.IndexTagkvResp {
	ret := &dataobj.IndexTagkvResp{}

	for _, tag := range tags.CompletedTags {
		name := string(tag.Name)
		switch name {
		case METRIC_NAME:
			ret.Metric = string(tag.Values[0])
		case ENDPOINT_NAME:
			ret.Endpoints = make([]string, len(tag.Values))
			for i, v := range tag.Values {
				ret.Endpoints[i] = string(v)
			}
		case NID_NAME:
			ret.Nids = make([]string, len(tag.Values))
			for i, v := range tag.Values {
				ret.Nids[i] = string(v)
			}
		default:
			kv := &dataobj.TagPair{Key: string(tag.Name)}
			kv.Values = make([]string, len(tag.Values))
			for i, v := range tag.Values {
				kv.Values[i] = string(v)
			}
			ret.Tagkv = append(ret.Tagkv, kv)
		}
	}

	return ret
}

func xcludeResp(iter ident.TagIterator) *dataobj.XcludeResp {
	ret := &dataobj.XcludeResp{}
	tags := map[string]string{}
	for iter.Next() {
		tag := iter.Current()
		switch key := tag.Name.String(); key {
		case METRIC_NAME:
			ret.Metric = tag.Value.String()
		case ENDPOINT_NAME:
			ret.Endpoint = tag.Value.String()
		case NID_NAME:
			ret.Nid = tag.Value.String()
		default:
			tags[key] = tag.Value.String()
		}
	}

	ret.Tags = append(ret.Tags, dataobj.SortedTags(tags))

	if err := iter.Err(); err != nil {
		logger.Errorf("FetchTaggedIDs iter: %s", err)
	}

	return ret
}

func resampleResp(data []*dataobj.TsdbQueryResponse, opts dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse {
	for _, v := range data {
		if len(v.Values) <= MAX_PONINTS {
			continue
		}
		v.Values = resample(v.Values, opts.Start, opts.End, int64(opts.Step), opts.ConsolFunc)
	}
	return data
}

func aggregateResp(data []*dataobj.TsdbQueryResponse, opts dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse {
	// aggregateResp
	if len(data) < 2 || opts.AggrFunc == "" {
		return data
	}

	// 没有聚合 tag, 或者曲线没有其他 tags, 直接所有曲线进行计算
	if len(opts.GroupKey) == 0 || getTags(data[0].Counter) == "" {
		return []*dataobj.TsdbQueryResponse{&dataobj.TsdbQueryResponse{
			Counter: opts.AggrFunc,
			Start:   opts.Start,
			End:     opts.End,
			Values:  calc.Compute(opts.AggrFunc, data),
		}}
	}

	aggrDatas := make([]*dataobj.TsdbQueryResponse, 0)
	aggrCounter := make(map[string][]*dataobj.TsdbQueryResponse)
	for _, v := range data {
		counterMap := make(map[string]string)

		tagsMap, err := dataobj.SplitTagsString(getTags(v.Counter))
		if err != nil {
			logger.Warningf("split tag string error: %+v", err)
			continue
		}
		if v.Nid != "" {
			tagsMap["node"] = v.Nid
		} else {
			tagsMap["endpoint"] = v.Endpoint
		}

		// 校验 GroupKey 是否在 tags 中
		for _, key := range opts.GroupKey {
			if value, exists := tagsMap[key]; exists {
				counterMap[key] = value
			}
		}

		counter := dataobj.SortedTags(counterMap)
		if _, exists := aggrCounter[counter]; exists {
			aggrCounter[counter] = append(aggrCounter[counter], v)
		} else {
			aggrCounter[counter] = []*dataobj.TsdbQueryResponse{v}
		}
	}

	// 有需要聚合的 tag 需要将 counter 带上
	for counter, datas := range aggrCounter {
		if counter != "" {
			counter = "/" + opts.AggrFunc + "," + counter
		}
		aggrData := &dataobj.TsdbQueryResponse{
			Start:   opts.Start,
			End:     opts.End,
			Counter: counter,
			Values:  calc.Compute(opts.AggrFunc, datas),
		}
		aggrDatas = append(aggrDatas, aggrData)
	}

	return aggrDatas
}

func resample(data []*dataobj.RRDData, start, end, step int64, consolFunc string) []*dataobj.RRDData {

	l := int((end - start) / step)
	if l <= 0 {
		return []*dataobj.RRDData{}
	}

	ret := make([]*dataobj.RRDData, 0, l)

	ts := start
	if t := data[0].Timestamp; t > start {
		ts = t
	}

	j := 0
	for ; ts < end; ts += step {
		get := func() (ret []dataobj.JsonFloat) {
			if j == len(data) {
				return
			}
			for {
				if j == len(data) {
					return
				}
				if d := data[j]; d.Timestamp < ts {
					ret = append(ret, d.Value)
					j++
					continue
				} else if d.Timestamp >= ts+step {
					return
				} else {
					ret = append(ret, d.Value)
					j++
					return
				}
			}
		}
		ret = append(ret, &dataobj.RRDData{
			Timestamp: ts,
			Value:     aggrData(consolFunc, get()),
		})
	}

	return ret
}

func aggrData(fn string, data []dataobj.JsonFloat) dataobj.JsonFloat {
	if len(data) == 0 {
		return dataobj.JsonFloat(math.NaN())
	}
	switch fn {
	case "sum":
		return sum(data)
	case "avg", "AVERAGE":
		return avg(data)
	case "max", "MAX":
		return max(data)
	case "min", "MIN":
		return min(data)
		// case "last":
	default:
		return last(data)
	}
}

func sum(data []dataobj.JsonFloat) (ret dataobj.JsonFloat) {
	for _, v := range data {
		ret += v
	}
	return ret
}

func avg(data []dataobj.JsonFloat) (ret dataobj.JsonFloat) {
	for _, v := range data {
		ret += v
	}
	return ret / dataobj.JsonFloat(len(data))
}

func max(data []dataobj.JsonFloat) (ret dataobj.JsonFloat) {
	ret = data[0]
	for i := 1; i < len(data); i++ {
		if data[i] > ret {
			ret = data[i]
		}
	}
	return ret
}

func min(data []dataobj.JsonFloat) (ret dataobj.JsonFloat) {
	ret = data[0]
	for i := 1; i < len(data); i++ {
		if data[i] < ret {
			ret = data[i]
		}
	}
	return ret
}

func last(data []dataobj.JsonFloat) (ret dataobj.JsonFloat) {
	return data[len(data)-1]
}

func getTags(counter string) (tags string) {
	idx := strings.IndexAny(counter, "/")
	if idx == -1 {
		return ""
	}
	return counter[idx+1:]
}
