package conv

import (
	"math"
	"strings"
	"time"

	"github.com/didi/nightingale/v5/src/models"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

const (
	LabelName = "__name__"
)

func ConvertToTimeSeries(value model.Value, rule *models.RecordingRule) (lst []*prompb.TimeSeries) {
	switch value.Type() {
	case model.ValVector:
		items, ok := value.(model.Vector)
		if !ok {
			return
		}

		for _, item := range items {
			if math.IsNaN(float64(item.Value)) {
				continue
			}
			s := prompb.Sample{}
			s.Timestamp = time.Unix(item.Timestamp.Unix(), 0).UnixNano() / 1e6
			s.Value = float64(item.Value)
			l := labelsToLabelsProto(item.Metric, rule)
			lst = append(lst, &prompb.TimeSeries{
				Labels:  l,
				Samples: []prompb.Sample{s},
			})
		}
	case model.ValMatrix:
		items, ok := value.(model.Matrix)
		if !ok {
			return
		}

		for _, item := range items {
			if len(item.Values) == 0 {
				return
			}

			last := item.Values[len(item.Values)-1]

			if math.IsNaN(float64(last.Value)) {
				continue
			}
			l := labelsToLabelsProto(item.Metric, rule)
			var slst []prompb.Sample
			for _, v := range item.Values {
				if math.IsNaN(float64(v.Value)) {
					continue
				}
				slst = append(slst, prompb.Sample{
					Timestamp: time.Unix(v.Timestamp.Unix(), 0).UnixNano() / 1e6,
					Value:     float64(v.Value),
				})
			}
			lst = append(lst, &prompb.TimeSeries{
				Labels:  l,
				Samples: slst,
			})
		}
	case model.ValScalar:
		item, ok := value.(*model.Scalar)
		if !ok {
			return
		}

		if math.IsNaN(float64(item.Value)) {
			return
		}

		lst = append(lst, &prompb.TimeSeries{
			Labels:  nil,
			Samples: []prompb.Sample{{Value: float64(item.Value), Timestamp: time.Unix(item.Timestamp.Unix(), 0).UnixNano() / 1e6}},
		})
	default:
		return
	}

	return
}

func labelsToLabelsProto(labels model.Metric, rule *models.RecordingRule) (result []*prompb.Label) {
	//name
	nameLs := &prompb.Label{
		Name:  LabelName,
		Value: rule.Name,
	}
	result = append(result, nameLs)
	for k, v := range labels {
		if k == LabelName {
			continue
		}
		if model.LabelNameRE.MatchString(string(k)) {
			result = append(result, &prompb.Label{
				Name:  string(k),
				Value: string(v),
			})
		}
	}
	if len(rule.AppendTagsJSON) != 0 {
		for _, v := range rule.AppendTagsJSON {
			index := strings.Index(v, "=")
			if model.LabelNameRE.MatchString(v[:index]) {
				result = append(result, &prompb.Label{
					Name:  v[:index],
					Value: v[index+1:],
				})
			}
		}
	}
	return result
}
