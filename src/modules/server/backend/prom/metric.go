package prom

import (
	"errors"
	"fmt"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/toolkits/pkg/logger"
)

const (
	LabelEndpoint = "endpoint"
	LabelNid      = "rdb_node_id"
)

type metricSelector map[string]metricSelectorValues
type metricSelectorValues []string

func (prom *PromDataSource) newMetricSelector(metrics, endpoints, nids []string) (selector metricSelector) {
	selector = make(map[string]metricSelectorValues)

	var names []string
	if metrics != nil && len(metrics) > 0 {
		for _, m := range metrics {
			name, err := prom.convertN9eMetricName(m)
			if err != nil {
				logger.Errorf("metric convert error: %v", err)
				continue
			}
			names = append(names, name)
		}
	}

	selector.addLabels(model.MetricNameLabel, names)
	selector.addLabels(LabelEndpoint, endpoints)
	selector.addLabels(LabelNid, nids)

	return selector
}

func (prom *PromDataSource) newSelectorByMeta(metric MetricMeta) (selector metricSelector) {
	selector = make(map[string]metricSelectorValues, len(metric.Tags)+3)
	selector.addMetricMeta(metric)

	return selector
}

func (selector *metricSelector) clear() {
	*selector = make(metricSelector)
}

func (selector metricSelector) addLabel(name string, value string) {
	if len(name) <= 0 || len(value) <= 0 {
		return
	}
	name, err := convertN9eLabelName(name)
	if err != nil {
		logger.Errorf("convert n9e tag name: %s got error: %v", name, err)
		return
	}

	selector[name] = append(selector[name], value)
}

func (selector metricSelector) addLabels(name string, values []string) {
	if len(name) <= 0 || values == nil || len(values) <= 0 {
		return
	}
	name, err := convertN9eLabelName(name)
	if err != nil {
		logger.Errorf("convert n9e tag name: %s got error: %v", name, err)
		return
	}

	selector[name] = append(selector[name], values...)
}

func (selector metricSelector) addLabelList(list []string) {
	for _, l := range list {
		pair := strings.SplitN(l, "=", 2)
		if len(pair) != 2 {
			continue
		}

		selector.addLabel(pair[0], pair[1])
	}
}

func (selector metricSelector) addMetricMeta(metric MetricMeta) {
	if len(metric.Name) > 0 {
		selector.addLabel(model.MetricNameLabel, metric.Name)
	}
	if len(metric.Endpoind) > 0 {
		selector.addLabel(LabelEndpoint, metric.Endpoind)
	}
	if len(metric.Nid) > 0 {
		selector.addLabel(LabelNid, metric.Nid)
	}

	for k, v := range metric.Tags {
		if len(v) > 0 {
			selector.addLabel(k, v)
		}
	}
}

func (selector metricSelector) String() string {
	var pairs []string
	for name, values := range selector {
		pairs = append(pairs, fmt.Sprintf("%s=~\"%s\"", name, values))
	}

	return fmt.Sprintf("{%s}", strings.Join(pairs, ","))
}

func (values *metricSelectorValues) dedup() metricSelectorValues {
	vm := make(map[string]struct{})
	for _, v := range *values {
		if len(v) > 0 {
			vm[v] = struct{}{}
		}
	}

	var nv metricSelectorValues
	for v := range vm {
		nv = append(nv, v)
	}

	*values = nv
	return nv
}

func (values metricSelectorValues) String() string {
	values.dedup()

	return strings.Join(values, "|")
}

type replacePair struct {
	n9e  string
	prom string
}

var (
	ErrInvalidName   = errors.New("invalid name")
	ErrWrongResponse = errors.New("http request got error")

	replaceList = []replacePair{
		{n9e: ".", prom: "__dot__"},
		{n9e: "-", prom: "__sub__"},
	}
)

func (prom *PromDataSource) convertN9eMetricName(name string) (string, error) {
	new := prom.Section.Prefix + convertN9eName(name)
	if !model.MetricNameRE.MatchString(new) {
		return new, fmt.Errorf("metric name: %s get: %w", name, ErrInvalidName)
	}
	return new, nil
}

func (prom *PromDataSource) convertPromMetricName(name string) string {
	if !strings.HasPrefix(name, prom.Section.Prefix) {
		return ""
	}
	name = strings.Replace(name, prom.Section.Prefix, "", 1)
	return convertPromName(name)
}

func convertN9eLabelName(name string) (string, error) {
	new := convertN9eName(name)
	if !model.LabelNameRE.MatchString(new) {
		return new, fmt.Errorf("label name: %s get: %w", name, ErrInvalidName)
	}
	return new, nil
}

func convertN9eName(name string) string {
	for _, pair := range replaceList {
		name = strings.ReplaceAll(name, pair.n9e, pair.prom)
	}

	return name
}

func convertPromName(name string) string {
	for _, pair := range replaceList {
		name = strings.ReplaceAll(name, pair.prom, pair.n9e)
	}

	return name
}
