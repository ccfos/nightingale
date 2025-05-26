package relabel

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/ccfos/nightingale/v6/alert/pipeline"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/ccfos/nightingale/v6/pushgw/writer"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

// RelabelConfig
type RelabelConfig struct {
	SourceLabels  []string `json:"source_labels"`
	Separator     string   `json:"separator"`
	Regex         string   `json:"regex"`
	RegexCompiled *regexp.Regexp
	If            string `json:"if"`
	IfRegex       *regexp.Regexp
	Modulus       uint64 `json:"modulus"`
	TargetLabel   string `json:"target_label"`
	Replacement   string `json:"replacement"`
	Action        string `json:"action"`
}

func Init() {
}

func init() {
	pipeline.RegisterProcessor("relabel", &RelabelConfig{})
}

func (r *RelabelConfig) Init(settings interface{}) (pipeline.Processor, error) {
	b, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}

	newProcessor := &RelabelConfig{}
	err = json.Unmarshal(b, &newProcessor)
	if err != nil {
		return nil, err
	}

	return newProcessor, nil
}

const (
	REPLACE_DOT = "___"
)

func (r *RelabelConfig) Process(ctx *ctx.Context, event *models.AlertCurEvent) {
	sourceLabels := make([]model.LabelName, len(r.SourceLabels))
	for i := range r.SourceLabels {
		sourceLabels[i] = model.LabelName(strings.ReplaceAll(r.SourceLabels[i], ".", REPLACE_DOT))
	}

	relabelConfigs := []*pconf.RelabelConfig{
		{
			SourceLabels:  sourceLabels,
			Separator:     r.Separator,
			Regex:         r.Regex,
			RegexCompiled: r.RegexCompiled,
			If:            r.If,
			IfRegex:       r.IfRegex,
			Modulus:       r.Modulus,
			TargetLabel:   r.TargetLabel,
			Replacement:   r.Replacement,
			Action:        r.Action,
		},
	}

	EventRelabel(event, relabelConfigs)
}

func EventRelabel(event *models.AlertCurEvent, relabelConfigs []*pconf.RelabelConfig) {
	labels := make([]prompb.Label, len(event.TagsJSON))
	event.OriginalTagsJSON = make([]string, len(event.TagsJSON))
	for i, tag := range event.TagsJSON {
		label := strings.SplitN(tag, "=", 2)
		if len(label) != 2 {
			continue
		}
		event.OriginalTagsJSON[i] = tag

		label[0] = strings.ReplaceAll(string(label[0]), ".", REPLACE_DOT)
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

		for j := range relabelConfigs[i].SourceLabels {
			relabelConfigs[i].SourceLabels[j] = model.LabelName(strings.ReplaceAll(string(relabelConfigs[i].SourceLabels[j]), ".", REPLACE_DOT))
		}
	}

	gotLabels := writer.Process(labels, relabelConfigs...)
	event.TagsJSON = make([]string, len(gotLabels))
	event.TagsMap = make(map[string]string, len(gotLabels))
	for i, label := range gotLabels {
		label.Name = strings.ReplaceAll(string(label.Name), REPLACE_DOT, ".")
		event.TagsJSON[i] = fmt.Sprintf("%s=%s", label.Name, label.Value)
		event.TagsMap[label.Name] = label.Value
	}
	event.Tags = strings.Join(event.TagsJSON, ",,")
}
