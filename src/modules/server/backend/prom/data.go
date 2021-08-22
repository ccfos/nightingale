package prom

import (
	"fmt"
	"strings"

	"github.com/didi/nightingale/v4/src/common/dataobj"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/toolkits/pkg/logger"
)

const (
	GaugeTypeStr = "GAUGE"
)

type MetricMeta struct {
	Name     string
	Endpoind string
	Nid      string
	Tags     map[string]string
}

func (prom *PromDataSource) convert2PromTimeSeries(item *dataobj.MetricValue) (*prompb.TimeSeries, error) {
	pt := &prompb.TimeSeries{}
	pt.Samples = append(pt.Samples, prompb.Sample{
		// 时间赋值问题,使用毫秒时间戳
		Timestamp: item.Timestamp * 1000,
		Value:     item.Value,
	})
	var labels []*prompb.Label

	name, err := prom.convertN9eMetricName(item.Metric)
	if err != nil {
		logger.Errorf("convert n9e metric name: %s got error: %v", item.Metric, err)
		return pt, err
	}
	nameLs := &prompb.Label{
		Name:  model.MetricNameLabel,
		Value: name,
	}
	labels = append(labels, nameLs)

	if item.Endpoint != "" {
		identLs := &prompb.Label{
			Name:  LabelEndpoint,
			Value: item.Endpoint,
		}
		labels = append(labels, identLs)
	}

	if item.Nid != "" {
		nodeLs := &prompb.Label{
			Name:  LabelNid,
			Value: item.Nid,
		}
		labels = append(labels, nodeLs)
	}

	for k, v := range item.TagsMap {
		lname, err := convertN9eLabelName(k)
		if err != nil {
			logger.Errorf("conver tag name: %s got error: %+v", k, err)
			return pt, err
		}
		ls := &prompb.Label{
			Name:  lname,
			Value: v,
		}
		labels = append(labels, ls)
	}

	pt.Labels = append(pt.Labels, labels...)
	return pt, nil
}

func convertValuesToRRDResp(value model.Value) []*dataobj.TsdbQueryResponse {
	switch value.Type() {
	case model.ValMatrix:
		v, ok := value.(model.Matrix)
		if !ok {
			return nil
		}
		return convertMatrixValuesToRRDResp(v)
	case model.ValVector:
		v, ok := value.(model.Vector)
		if !ok {
			return nil
		}
		return convertVectorValuesToRRDResp(v)
	case model.ValNone:
		fallthrough
	case model.ValString:
		fallthrough
	case model.ValScalar:
		fallthrough
	default:
		return nil
	}
}

func convertVectorValuesToRRDResp(value model.Vector) []*dataobj.TsdbQueryResponse {
	rmap := make(map[string]*dataobj.TsdbQueryResponse)
	for _, v := range value {
		data := dataobj.RRDData{
			Timestamp: v.Timestamp.Unix(),
			Value:     dataobj.JsonFloat(v.Value),
		}
		_, ok := v.Metric[model.MetricNameLabel]
		if !ok {
			continue
		}

		key := v.Metric.String()
		rdata, ok := rmap[key]
		if ok {
			rdata.Values = append(rdata.Values, &data)
			continue
		}

		rdata = &dataobj.TsdbQueryResponse{
			Values: []*dataobj.RRDData{&data},
		}
		rmap[key] = rdata

		meta := getMetricInfos(v.Metric)
		rdata.Endpoint = meta.Endpoind
		rdata.Nid = meta.Nid

		var tags []string
		for k, v := range meta.Tags {
			tags = append(tags, fmt.Sprintf("%s=%s", k, v))
		}

		rdata.Counter = fmt.Sprintf("%s/%s", meta.Name, strings.Join(tags, ","))
		rdata.DsType = GaugeTypeStr
	}

	var ret []*dataobj.TsdbQueryResponse
	for _, res := range rmap {
		ret = append(ret, res)
	}
	return ret
}

func convertMatrixValuesToRRDResp(value model.Matrix) []*dataobj.TsdbQueryResponse {
	rmap := make(map[string]*dataobj.TsdbQueryResponse)
	for _, v := range value {
		var dataList []*dataobj.RRDData
		for _, vv := range v.Values {
			dataList = append(dataList, &dataobj.RRDData{
				Timestamp: vv.Timestamp.Unix(),
				Value:     dataobj.JsonFloat(vv.Value),
			})
		}
		_, ok := v.Metric[model.MetricNameLabel]
		if !ok {
			continue
		}

		key := v.Metric.String()
		rdata, ok := rmap[key]
		if ok {
			rdata.Values = append(rdata.Values, dataList...)
			continue
		}

		rdata = &dataobj.TsdbQueryResponse{
			Values: dataList,
		}
		rmap[key] = rdata

		// var tagMap map[string]string
		meta := getMetricInfos(v.Metric)
		rdata.Endpoint = meta.Endpoind
		rdata.Nid = meta.Nid

		var tags []string
		for k, v := range meta.Tags {
			tags = append(tags, fmt.Sprintf("%s=%s", k, v))
		}

		rdata.Counter = fmt.Sprintf("%s/%s", meta.Name, strings.Join(tags, ","))
		rdata.DsType = GaugeTypeStr
	}

	var ret []*dataobj.TsdbQueryResponse
	for _, res := range rmap {
		ret = append(ret, res)
	}
	return ret
}

func getMetricInfos(m model.Metric) MetricMeta {
	meta := MetricMeta{
		Tags: make(map[string]string),
	}
	for k, v := range m {
		if string(k) == model.MetricNameLabel {
			meta.Name = string(v)
			continue
		}
		if string(k) == LabelEndpoint {
			meta.Endpoind = string(v)
			continue
		}
		if string(k) == LabelNid {
			meta.Nid = string(v)
		}
		meta.Tags[string(k)] = string(v)
	}
	return meta
}
