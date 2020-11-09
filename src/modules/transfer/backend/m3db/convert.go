package m3db

import (
	"github.com/didi/nightingale/src/common/dataobj"
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
		logger.Errorf("FetchTaggedIDs iter:", err)
	}

	return ret
}
