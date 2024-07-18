// @Author: Ciusyan 6/19/24

package writer

import (
	"reflect"
	"sort"
	"testing"

	"github.com/ccfos/nightingale/v6/pushgw/pconf"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

func TestProcess(t *testing.T) {
	tests := []struct {
		name     string
		labels   []prompb.Label
		cfgs     []*pconf.RelabelConfig
		expected []prompb.Label
	}{
		// 1. 添加新标签 (Adding new label)
		{
			name:   "Adding new label",
			labels: []prompb.Label{{Name: "job", Value: "aa"}},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:      "replace",
					TargetLabel: "foo",
					Replacement: "bar",
				},
			},
			expected: []prompb.Label{{Name: "job", Value: "aa"}, {Name: "foo", Value: "bar"}},
		},
		// 2. 更新现有标签 (Updating existing label)
		{
			name:   "Updating existing label",
			labels: []prompb.Label{{Name: "foo", Value: "aaaa"}},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:      "replace",
					TargetLabel: "foo",
					Replacement: "bar",
				},
			},
			expected: []prompb.Label{{Name: "foo", Value: "bar"}},
		},
		// 3. 重写现有标签 (Rewriting existing label)
		{
			name:   "Rewriting existing label",
			labels: []prompb.Label{{Name: "instance", Value: "bar:123"}},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "replace",
					SourceLabels: model.LabelNames{"instance"},
					Regex:        "([^:]+):.+",
					TargetLabel:  "instance",
					Replacement:  "$1",
				},
			},
			expected: []prompb.Label{{Name: "instance", Value: "bar"}},
		},
		{
			name:   "Rewriting existing label",
			labels: []prompb.Label{{Name: "instance", Value: "bar:123"}},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "replace",
					SourceLabels: model.LabelNames{"instance"},
					Regex:        ":([0-9]+)$",
					TargetLabel:  "port",
					Replacement:  "$1",
				},
			},
			expected: []prompb.Label{{Name: "port", Value: "123"}, {Name: "instance", Value: "bar:123"}},
		},
		// 4. 更新度量标准名称 (Updating metric name)
		{
			name:   "Updating metric name",
			labels: []prompb.Label{{Name: "__name__", Value: "foo_suffix"}},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "replace",
					SourceLabels: model.LabelNames{"__name__"},
					Regex:        "(.+)_suffix",
					TargetLabel:  "__name__",
					Replacement:  "prefix_$1",
				},
			},
			expected: []prompb.Label{{Name: "__name__", Value: "prefix_foo"}},
		},
		// 5. 删除不需要/保持需要 的标签 (Removing unneeded labels)
		{
			name: "Removing unneeded labels",
			labels: []prompb.Label{
				{Name: "job", Value: "a"},
				{Name: "instance", Value: "xyz"},
				{Name: "foobar", Value: "baz"},
				{Name: "foox", Value: "aaa"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action: "labeldrop",
					Regex:  "foo.+",
				},
			},
			expected: []prompb.Label{
				{Name: "job", Value: "a"},
				{Name: "instance", Value: "xyz"},
			},
		},
		{
			name: "keep needed labels",
			labels: []prompb.Label{
				{Name: "job", Value: "a"},
				{Name: "instance", Value: "xyz"},
				{Name: "foobar", Value: "baz"},
				{Name: "foox", Value: "aaa"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action: "labelkeep",
					Regex:  "foo.+",
				},
			},
			expected: []prompb.Label{
				{Name: "foobar", Value: "baz"},
				{Name: "foox", Value: "aaa"},
			},
		},
		// 6. 删除特定标签值 (Removing the specific label value)
		{
			name: "Removing the specific label value",
			labels: []prompb.Label{
				{Name: "foo", Value: "bar"},
				{Name: "baz", Value: "x"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "replace",
					SourceLabels: model.LabelNames{"foo"},
					Regex:        "bar",
					TargetLabel:  "foo",
					Replacement:  "",
				},
			},
			expected: []prompb.Label{
				{Name: "baz", Value: "x"},
			},
		},
		// 7. 删除不需要的度量标准 (Removing unneeded metrics)
		{
			name: "Removing unneeded metrics",
			labels: []prompb.Label{
				{Name: "instance", Value: "foobar1"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "drop",
					SourceLabels: model.LabelNames{"instance"},
					Regex:        "foobar.+",
				},
			},
			expected: nil,
		},
		{
			name: "Removing unneeded metrics 2",
			labels: []prompb.Label{
				{Name: "instance", Value: "foobar2"},
				{Name: "job", Value: "xxx"},
				{Name: "aaa", Value: "bb"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "drop",
					SourceLabels: model.LabelNames{"instance"},
					Regex:        "foobar.+",
				},
			},
			expected: nil,
		},
		{
			name: "Removing unneeded metrics 3",
			labels: []prompb.Label{
				{Name: "instance", Value: "xxx"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "drop",
					SourceLabels: model.LabelNames{"instance"},
					Regex:        "foobar.+",
				},
			},
			expected: []prompb.Label{
				{Name: "instance", Value: "xxx"},
			},
		},
		{
			name: "Removing unneeded metrics 4",
			labels: []prompb.Label{
				{Name: "instance", Value: "abc"},
				{Name: "job", Value: "xyz"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "drop",
					SourceLabels: model.LabelNames{"instance"},
					Regex:        "foobar.+",
				},
			},
			expected: []prompb.Label{
				{Name: "instance", Value: "abc"},
				{Name: "job", Value: "xyz"},
			},
		},
		{
			name: "Removing unneeded metrics with multiple labels",
			labels: []prompb.Label{
				{Name: "job", Value: "foo"},
				{Name: "instance", Value: "bar"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "drop",
					SourceLabels: model.LabelNames{"job", "instance"},
					Regex:        "foo;bar",
					Separator:    ";",
				},
			},
			expected: nil,
		},
		// 8. 按条件删除度量标准 (Dropping metrics on certain condition)
		{
			name: "Dropping metrics on certain condition",
			labels: []prompb.Label{
				{Name: "real_port", Value: "123"},
				{Name: "needed_port", Value: "123"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "drop_if_equal",
					SourceLabels: model.LabelNames{"real_port", "needed_port"},
				},
			},
			expected: nil,
		},
		{
			name: "Dropping metrics on certain condition 2",
			labels: []prompb.Label{
				{Name: "real_port", Value: "123"},
				{Name: "needed_port", Value: "456"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "drop_if_equal",
					SourceLabels: model.LabelNames{"real_port", "needed_port"},
				},
			},
			expected: []prompb.Label{
				{Name: "real_port", Value: "123"},
				{Name: "needed_port", Value: "456"},
			},
		},
		// 9. 修改标签名称 (Modifying label names)
		{
			name: "Modifying label names",
			labels: []prompb.Label{
				{Name: "foo_xx", Value: "bb"},
				{Name: "job", Value: "qq"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:      "labelmap",
					Regex:       "foo_(.+)",
					Replacement: "bar_$1",
				},
			},
			expected: []prompb.Label{
				{Name: "foo_xx", Value: "bb"},
				{Name: "bar_xx", Value: "bb"},
				{Name: "job", Value: "qq"},
			},
		},
		// 10. 从多个现有标签构建新标签 (Constructing a label from multiple existing labels)
		{
			name: "Constructing a label from multiple existing labels",
			labels: []prompb.Label{
				{Name: "host", Value: "hostname"},
				{Name: "port", Value: "9090"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:       "replace",
					SourceLabels: model.LabelNames{"host", "port"},
					Separator:    ":",
					TargetLabel:  "address",
				},
			},
			expected: []prompb.Label{
				{Name: "host", Value: "hostname"},
				{Name: "port", Value: "9090"},
				{Name: "address", Value: "hostname:9090"},
			},
		},
		// 11. 链式重标记规则 (Chaining relabeling rules)
		{
			name: "Chaining relabeling rules",
			labels: []prompb.Label{
				{Name: "instance", Value: "hostname:9090"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:      "replace",
					TargetLabel: "foo",
					Replacement: "bar",
				},
				{
					Action:       "replace",
					SourceLabels: model.LabelNames{"instance"},
					Regex:        "([^:]+):.*",
					TargetLabel:  "instance",
					Replacement:  "$1",
				},
			},
			expected: []prompb.Label{
				{Name: "instance", Value: "hostname"},
				{Name: "foo", Value: "bar"},
			},
		},
		// 12. 条件重标记 (Conditional relabeling)
		{
			name: "Conditional relabeling matches",
			labels: []prompb.Label{
				{Name: "label", Value: "x"},
				{Name: "foo", Value: "aaa"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:      "replace",
					If:          `label="x|y"`,
					TargetLabel: "foo",
					Replacement: "bar",
					IfRegex:     compileRegex(`label="x|y"`),
				},
			},
			expected: []prompb.Label{
				{Name: "label", Value: "x"},
				{Name: "foo", Value: "bar"},
			},
		},
		{
			name: "Conditional relabeling matches alternative",
			labels: []prompb.Label{
				{Name: "label", Value: "y"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:      "replace",
					If:          `label="x|y"`,
					TargetLabel: "foo",
					Replacement: "bar",
					IfRegex:     compileRegex(`label="x|y"`),
				},
			},
			expected: []prompb.Label{
				{Name: "label", Value: "y"},
				{Name: "foo", Value: "bar"},
			},
		},
		{
			name: "Conditional relabeling does not match",
			labels: []prompb.Label{
				{Name: "label", Value: "z"},
			},
			cfgs: []*pconf.RelabelConfig{
				{
					Action:      "replace",
					If:          `label="x|y"`,
					TargetLabel: "foo",
					Replacement: "bar",
					IfRegex:     compileRegex(`label="x|y"`),
				},
			},
			expected: []prompb.Label{
				{Name: "label", Value: "z"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Process(tt.labels, tt.cfgs...)
			// Sort the slices before comparison
			sort.Slice(got, func(i, j int) bool {
				return got[i].Name < got[j].Name
			})
			sort.Slice(tt.expected, func(i, j int) bool {
				return tt.expected[i].Name < tt.expected[j].Name
			})
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("Process() = %v, want %v", got, tt.expected)
			}
		})
	}
}
