package relabel

import (
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/alert/pipeline/processor/common"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/writer"
	"github.com/prometheus/prometheus/prompb"
)

// RelabelConfig
type RelabelConfig struct {
	pconf.RelabelConfig
}

func init() {
	models.RegisterProcessor("relabel", &RelabelConfig{})
}

func (r *RelabelConfig) Init(settings interface{}) (models.Processor, error) {
	result, err := common.InitProcessor[*RelabelConfig](settings)
	return result, err
}

func (r *RelabelConfig) Process(ctx *ctx.Context, event *models.AlertCurEvent) {
	EventRelabel(event, []*pconf.RelabelConfig{&r.RelabelConfig})
}

func EventRelabel(event *models.AlertCurEvent, relabelConfigs []*pconf.RelabelConfig) {
	labels := make([]prompb.Label, len(event.TagsJSON))
	event.OriginalTagsJSON = make([]string, len(event.TagsJSON))
	for i, tag := range event.TagsJSON {
		label := strings.SplitN(tag, "=", 2)
		event.OriginalTagsJSON[i] = tag
		labels[i] = prompb.Label{Name: label[0], Value: label[1]}
	}

	for i := 0; i < len(relabelConfigs); i++ {
		if relabelConfigs[i].Replacement == "" {
			relabelConfigs[i].Replacement = "$1"
		}

		if relabelConfigs[i].Separator == "" {
			relabelConfigs[i].Separator = ";"
		}

		if relabelConfigs[i].Regex == "" {
			relabelConfigs[i].Regex = "(.*)"
		}
	}

	gotLabels := writer.Process(labels, relabelConfigs...)
	event.TagsJSON = make([]string, len(gotLabels))
	event.TagsMap = make(map[string]string, len(gotLabels))
	for i, label := range gotLabels {
		event.TagsJSON[i] = fmt.Sprintf("%s=%s", label.Name, label.Value)
		event.TagsMap[label.Name] = label.Value
	}
	event.Tags = strings.Join(event.TagsJSON, ",,")
}
