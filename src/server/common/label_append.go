package common

import (
	"github.com/didi/nightingale/v5/src/models"
	"github.com/didi/nightingale/v5/src/server/config"
	"github.com/didi/nightingale/v5/src/server/memsto"
	"github.com/prometheus/prometheus/prompb"
)

func AppendLabels(pt *prompb.TimeSeries, target *models.Target) {
	if target == nil {
		return
	}

	labelKeys := make(map[string]struct{})
	for j := 0; j < len(pt.Labels); j++ {
		labelKeys[pt.Labels[j].Name] = struct{}{}
	}

	for key, value := range target.TagsMap {
		if _, has := labelKeys[key]; has {
			continue
		}

		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  key,
			Value: value,
		})
	}

	// e.g. busigroup=cloud
	if _, has := labelKeys[config.C.BusiGroupLabelKey]; has {
		return
	}

	if target.GroupId > 0 && len(config.C.BusiGroupLabelKey) > 0 {
		bg := memsto.BusiGroupCache.GetByBusiGroupId(target.GroupId)
		if bg == nil {
			return
		}

		if bg.LabelEnable == 0 {
			return
		}

		pt.Labels = append(pt.Labels, &prompb.Label{
			Name:  config.C.BusiGroupLabelKey,
			Value: bg.LabelValue,
		})
	}
}
