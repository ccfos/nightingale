package common

import (
	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/prometheus/prometheus/prompb"
)

func AppendLabels(pt *prompb.TimeSeries, target *models.Target) {
	if target == nil {
		return
	}

	for key, value := range target.TagsMap {
		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  key,
			Value: value,
		})
	}

	if target.GroupId > 0 {
		bg := memsto.BusiGroupCache.GetByBusiGroupId(target.GroupId)
		if bg == nil {
			return
		}

		if bg.LabelEnable == 0 {
			return
		}

		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  "busigroup",
			Value: bg.LabelValue,
		})
	}
}
