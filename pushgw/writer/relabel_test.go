// @Author: Ciusyan 6/19/24

package writer

import (
	"crypto/md5"
	"fmt"
	"reflect"
	"regexp"
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
					Action:        "replace",
					SourceLabels:  model.LabelNames{"instance"},
					Regex:         "([^:]+):.+",
					TargetLabel:   "instance",
					Replacement:   "$1",
					RegexCompiled: regexp.MustCompile("([^:]+):.+"),
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
					Regex:        ".*:([0-9]+)$",
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
					Action:        "replace",
					SourceLabels:  model.LabelNames{"__name__"},
					Regex:         "(.+)_suffix",
					TargetLabel:   "__name__",
					Replacement:   "prefix_$1",
					RegexCompiled: regexp.MustCompile("(.+)_suffix"),
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
					Action:        "labeldrop",
					Regex:         "foo.+",
					RegexCompiled: regexp.MustCompile("foo.+"),
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
					Action:        "labelkeep",
					Regex:         "foo.+",
					RegexCompiled: regexp.MustCompile("foo.+"),
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
					Action:        "replace",
					SourceLabels:  model.LabelNames{"foo"},
					Regex:         "bar",
					TargetLabel:   "foo",
					Replacement:   "",
					RegexCompiled: regexp.MustCompile("bar"),
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
					Action:        "drop",
					SourceLabels:  model.LabelNames{"instance"},
					Regex:         "foobar.+",
					RegexCompiled: regexp.MustCompile("foobar.+"),
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
					Action:        "drop",
					SourceLabels:  model.LabelNames{"instance"},
					Regex:         "foobar.+",
					RegexCompiled: regexp.MustCompile("foobar.+"),
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
					Action:        "drop",
					SourceLabels:  model.LabelNames{"instance"},
					Regex:         "foobar.+",
					RegexCompiled: regexp.MustCompile("foobar.+"),
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
					Action:        "drop",
					SourceLabels:  model.LabelNames{"instance"},
					Regex:         "foobar.+",
					RegexCompiled: regexp.MustCompile("foobar.+"),
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
					Action:        "drop",
					SourceLabels:  model.LabelNames{"job", "instance"},
					Regex:         "foo;bar",
					Separator:     ";",
					RegexCompiled: regexp.MustCompile("foo;bar"),
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
					Action:        "labelmap",
					Regex:         "foo_(.+)",
					Replacement:   "bar_$1",
					RegexCompiled: regexp.MustCompile("foo_(.+)"),
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
					Action:        "replace",
					SourceLabels:  model.LabelNames{"instance"},
					Regex:         "([^:]+):.*",
					TargetLabel:   "instance",
					Replacement:   "$1",
					RegexCompiled: regexp.MustCompile("([^:]+):.*"),
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

func TestHandleDrop(t *testing.T) {
	tests := []struct {
		name     string
		lb       *LabelBuilder
		regx     *regexp.Regexp
		val      string
		expected []prompb.Label
	}{
		{
			name:     "Drop matching label",
			lb:       newBuilder([]prompb.Label{{Name: "test", Value: "drop"}}),
			regx:     regexp.MustCompile("^drop$"),
			val:      "drop",
			expected: nil,
		},
		{
			name:     "Keep non-matching label",
			lb:       newBuilder([]prompb.Label{{Name: "test", Value: "keep"}}),
			regx:     regexp.MustCompile("^drop$"),
			val:      "keep",
			expected: []prompb.Label{{Name: "test", Value: "keep"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleDrop(tt.lb, tt.regx, tt.val)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("handleDrop() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandleKeep(t *testing.T) {
	tests := []struct {
		name     string
		lb       *LabelBuilder
		regx     *regexp.Regexp
		val      string
		expected []prompb.Label
	}{
		{
			name:     "Keep matching label",
			lb:       newBuilder([]prompb.Label{{Name: "test", Value: "keep"}}),
			regx:     regexp.MustCompile("^keep$"),
			val:      "keep",
			expected: []prompb.Label{{Name: "test", Value: "keep"}},
		},
		{
			name:     "Drop non-matching label",
			lb:       newBuilder([]prompb.Label{{Name: "test", Value: "drop"}}),
			regx:     regexp.MustCompile("^keep$"),
			val:      "drop",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleKeep(tt.lb, tt.regx, tt.val)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("handleKeep() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandleReplace(t *testing.T) {
	tests := []struct {
		name     string
		lb       *LabelBuilder
		regx     *regexp.Regexp
		cfg      *pconf.RelabelConfig
		val      string
		lset     []prompb.Label
		expected []prompb.Label
	}{
		{
			name:     "Add label directly",
			lb:       newBuilder([]prompb.Label{}),
			regx:     nil,
			cfg:      &pconf.RelabelConfig{TargetLabel: "test", Replacement: "replaced"},
			val:      "original",
			lset:     []prompb.Label{},
			expected: []prompb.Label{{Name: "test", Value: "replaced"}},
		},
		{
			name: "Set label directly",
			lb:   newBuilder([]prompb.Label{{Name: "test", Value: "original"}}),
			regx: nil,
			cfg: &pconf.RelabelConfig{
				TargetLabel: "test",
				Replacement: "replaced",
			},
			val:      "original",
			lset:     []prompb.Label{{Name: "test", Value: "original"}},
			expected: []prompb.Label{{Name: "test", Value: "replaced"}},
		},
		{
			name:     "If condition matches",
			lb:       newBuilder([]prompb.Label{{Name: "test", Value: "original"}}),
			regx:     regexp.MustCompile("^original$"),
			cfg:      &pconf.RelabelConfig{TargetLabel: "test", Replacement: "replaced", If: `test="original"`, IfRegex: compileRegex(`test="original"`)},
			val:      "original",
			lset:     []prompb.Label{{Name: "test", Value: "original"}},
			expected: []prompb.Label{{Name: "test", Value: "replaced"}},
		},
		{
			name:     "If condition does not match",
			lb:       newBuilder([]prompb.Label{{Name: "test", Value: "original"}}),
			regx:     regexp.MustCompile("^original$"),
			cfg:      &pconf.RelabelConfig{TargetLabel: "test", Replacement: "replaced", If: `test="nomatch"`, IfRegex: compileRegex(`test="nomatch"`)},
			val:      "original",
			lset:     []prompb.Label{{Name: "test", Value: "original"}},
			expected: []prompb.Label{{Name: "test", Value: "original"}},
		},
		{
			name: "Replace by concatenation",
			lb: newBuilder([]prompb.Label{
				{Name: "host", Value: "hostname"},
				{Name: "port", Value: "9090"},
			}),
			regx: nil,
			cfg: &pconf.RelabelConfig{
				TargetLabel:  "address",
				SourceLabels: model.LabelNames{"host", "port"},
				Separator:    ":",
			},
			val: "hostname:9090",
			lset: []prompb.Label{
				{Name: "host", Value: "hostname"},
				{Name: "port", Value: "9090"},
			},
			expected: []prompb.Label{
				{Name: "host", Value: "hostname"},
				{Name: "port", Value: "9090"},
				{Name: "address", Value: "hostname:9090"},
			},
		},
		{
			name: "Replace by partial match",
			lb: newBuilder([]prompb.Label{
				{Name: "instance", Value: "bar:123"},
			}),
			regx: regexp.MustCompile("([^:]+):.+"),
			cfg: &pconf.RelabelConfig{
				TargetLabel:  "instance",
				SourceLabels: model.LabelNames{"instance"},
				Regex:        "([^:]+):.+",
				Replacement:  "$1",
			},
			val: "bar:123",
			lset: []prompb.Label{
				{Name: "instance", Value: "bar:123"},
			},
			expected: []prompb.Label{
				{Name: "instance", Value: "bar"},
			},
		},
		{
			name: "Conditional replace with regex",
			lb: newBuilder([]prompb.Label{
				{Name: "metric", Value: "x"},
				{Name: "foo", Value: "original"},
			}),
			regx: nil,
			cfg: &pconf.RelabelConfig{
				TargetLabel: "foo",
				Replacement: "bar",
				If:          `metric="x|y"`,
				IfRegex:     compileRegex(`metric="x|y"`),
			},
			val: "original",
			lset: []prompb.Label{
				{Name: "metric", Value: "x"},
				{Name: "foo", Value: "original"},
			},
			expected: []prompb.Label{
				{Name: "metric", Value: "x"},
				{Name: "foo", Value: "bar"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleReplace(tt.lb, tt.regx, tt.cfg, tt.val, tt.lset)
			// Sort the slices before comparison
			sort.Slice(got, func(i, j int) bool {
				return got[i].Name < got[j].Name
			})
			sort.Slice(tt.expected, func(i, j int) bool {
				return tt.expected[i].Name < tt.expected[j].Name
			})
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("handleReplace() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandleLowercase(t *testing.T) {
	tests := []struct {
		name     string
		lb       *LabelBuilder
		cfg      *pconf.RelabelConfig
		val      string
		expected []prompb.Label
	}{
		{
			name:     "Lowercase label",
			lb:       newBuilder([]prompb.Label{{Name: "test", Value: "UPPERCASE"}}),
			cfg:      &pconf.RelabelConfig{TargetLabel: "test"},
			val:      "UPPERCASE",
			expected: []prompb.Label{{Name: "test", Value: "uppercase"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleLowercase(tt.lb, tt.cfg, tt.val)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("handleLowercase() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandleUppercase(t *testing.T) {
	tests := []struct {
		name     string
		lb       *LabelBuilder
		cfg      *pconf.RelabelConfig
		val      string
		expected []prompb.Label
	}{
		{
			name:     "Uppercase label",
			lb:       newBuilder([]prompb.Label{{Name: "test", Value: "lowercase"}}),
			cfg:      &pconf.RelabelConfig{TargetLabel: "test"},
			val:      "lowercase",
			expected: []prompb.Label{{Name: "test", Value: "LOWERCASE"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleUppercase(tt.lb, tt.cfg, tt.val)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("handleUppercase() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandleHashMod(t *testing.T) {
	tests := []struct {
		name     string
		lb       *LabelBuilder
		cfg      *pconf.RelabelConfig
		val      string
		expected []prompb.Label
	}{
		{
			name:     "HashMod label",
			lb:       newBuilder([]prompb.Label{{Name: "test", Value: "value"}}),
			cfg:      &pconf.RelabelConfig{TargetLabel: "test", Modulus: 100},
			val:      "value",
			expected: []prompb.Label{{Name: "test", Value: fmt.Sprintf("%d", sum64(md5.Sum([]byte("value")))%100)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleHashMod(tt.lb, tt.cfg, tt.val)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("handleHashMod() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandleLabelMap(t *testing.T) {
	tests := []struct {
		name     string
		lb       *LabelBuilder
		regx     *regexp.Regexp
		lset     []prompb.Label
		cfg      *pconf.RelabelConfig
		expected []prompb.Label
	}{
		{
			name:     "LabelMap",
			lb:       newBuilder([]prompb.Label{{Name: "foo_test", Value: "value"}}),
			regx:     regexp.MustCompile("^foo_(.+)$"),
			lset:     []prompb.Label{{Name: "foo_test", Value: "value"}},
			cfg:      &pconf.RelabelConfig{Replacement: "bar_$1"},
			expected: []prompb.Label{{Name: "foo_test", Value: "value"}, {Name: "bar_test", Value: "value"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleLabelMap(tt.lb, tt.regx, tt.lset, tt.cfg)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("handleLabelMap() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandleLabelDrop(t *testing.T) {
	tests := []struct {
		name     string
		lb       *LabelBuilder
		regx     *regexp.Regexp
		lset     []prompb.Label
		expected []prompb.Label
	}{
		{
			name:     "LabelDrop",
			lb:       newBuilder([]prompb.Label{{Name: "drop_me", Value: "value"}}),
			regx:     regexp.MustCompile("^drop_me$"),
			lset:     []prompb.Label{{Name: "drop_me", Value: "value"}},
			expected: []prompb.Label{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleLabelDrop(tt.lb, tt.regx, tt.lset)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("handleLabelDrop() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandleLabelKeep(t *testing.T) {
	tests := []struct {
		name     string
		lb       *LabelBuilder
		regx     *regexp.Regexp
		lset     []prompb.Label
		expected []prompb.Label
	}{
		{
			name:     "LabelKeep",
			lb:       newBuilder([]prompb.Label{{Name: "keep_me", Value: "value"}}),
			regx:     regexp.MustCompile("^keep_me$"),
			lset:     []prompb.Label{{Name: "keep_me", Value: "value"}, {Name: "drop_me", Value: "value"}},
			expected: []prompb.Label{{Name: "keep_me", Value: "value"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleLabelKeep(tt.lb, tt.regx, tt.lset)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("handleLabelKeep() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHandleDropIfEqual(t *testing.T) {
	tests := []struct {
		name     string
		lb       *LabelBuilder
		cfg      *pconf.RelabelConfig
		lset     []prompb.Label
		expected []prompb.Label
	}{
		{
			name:     "DropIfEqual equal",
			lb:       newBuilder([]prompb.Label{{Name: "a", Value: "1"}, {Name: "b", Value: "1"}}),
			cfg:      &pconf.RelabelConfig{SourceLabels: []model.LabelName{"a", "b"}},
			lset:     []prompb.Label{{Name: "a", Value: "1"}, {Name: "b", Value: "1"}},
			expected: nil,
		},
		{
			name:     "DropIfEqual not equal",
			lb:       newBuilder([]prompb.Label{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}}),
			cfg:      &pconf.RelabelConfig{SourceLabels: []model.LabelName{"a", "b"}},
			lset:     []prompb.Label{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}},
			expected: []prompb.Label{{Name: "a", Value: "1"}, {Name: "b", Value: "2"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := handleDropIfEqual(tt.lb, tt.cfg, tt.lset)
			// Sort the slices before comparison
			sort.Slice(got, func(i, j int) bool {
				return got[i].Name < got[j].Name
			})
			sort.Slice(tt.expected, func(i, j int) bool {
				return tt.expected[i].Name < tt.expected[j].Name
			})
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("handleDropIfEqual() = %v, want %v", got, tt.expected)
			}

			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("handleDropIfEqual() = %v, want %v", got, tt.expected)
			}
		})
	}
}
