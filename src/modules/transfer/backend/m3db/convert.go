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

func xcludeResp(iter ident.TagIterator) (ret dataobj.XcludeResp) {
	tags := map[string]string{}
	for iter.Next() {
		tag := iter.Current()
		switch key := tag.Name.String(); key {
		case METRIC_NAME:
			ret.Metric = tag.Value.String()
		case ENDPOINT_NAME, NID_NAME:
			ret.Endpoint = tag.Value.String()
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

func aggregateResp(data []*dataobj.TsdbQueryResponse, opts dataobj.QueryDataForUI) []*dataobj.TsdbQueryResponse {
	if len(data) < 2 || opts.AggrFunc == "" {
		return data
	}

	// Adjust the data
	for _, v := range data {
		v.Values = resample(v.Values, opts.Start, opts.End, int64(opts.Step))
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

var (
	nanFloat = dataobj.JsonFloat(math.NaN())
)

func resample(data []*dataobj.RRDData, start, end, step int64) []*dataobj.RRDData {
	l := int((end - start) / step)
	if l <= 0 {
		return []*dataobj.RRDData{}
	}

	ret := make([]*dataobj.RRDData, 0, l)

	ts := start
	if t := data[0].Timestamp; t > start {
		ts = t - t%step
	}

	j := 0
	for ; ts < end; ts += step {
		get := func() *dataobj.RRDData {
			if j == len(data) {
				return nil
			}
			for {
				if j == len(data) {
					return &dataobj.RRDData{Timestamp: ts, Value: nanFloat}
				}
				if d := data[j]; d.Timestamp < ts {
					j++
					continue
				} else if d.Timestamp >= ts+step {
					return &dataobj.RRDData{Timestamp: ts, Value: nanFloat}
				} else {
					j++
					return d
				}
			}
		}
		ret = append(ret, get())
	}

	return ret
}

func getTags(counter string) (tags string) {
	idx := strings.IndexAny(counter, "/")
	if idx == -1 {
		return ""
	}
	return counter[idx+1:]
}
