package writer

import (
	"crypto/md5"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ccfos/nightingale/v6/pushgw/pconf"
	"github.com/toolkits/pkg/logger"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

const (
	Replace     string = "replace"
	Keep        string = "keep"
	Drop        string = "drop"
	HashMod     string = "hashmod"
	LabelMap    string = "labelmap"
	LabelDrop   string = "labeldrop"
	LabelKeep   string = "labelkeep"
	Lowercase   string = "lowercase"
	Uppercase   string = "uppercase"
	DropIfEqual string = "drop_if_equal"
)

func Process(labels []prompb.Label, cfgs ...*pconf.RelabelConfig) []prompb.Label {
	for _, cfg := range cfgs {
		labels = relabel(labels, cfg)
		if labels == nil {
			return nil
		}
	}
	return labels
}

func getValue(ls []prompb.Label, name model.LabelName) string {
	for _, l := range ls {
		if l.Name == string(name) {
			return l.Value
		}
	}
	return ""
}

type LabelBuilder struct {
	LabelSet map[string]string
}

func newBuilder(ls []prompb.Label) *LabelBuilder {
	lset := make(map[string]string, len(ls))
	for _, l := range ls {
		lset[l.Name] = l.Value
	}
	return &LabelBuilder{LabelSet: lset}
}

func (l *LabelBuilder) set(k, v string) *LabelBuilder {
	l.LabelSet[k] = v
	return l
}

func (l *LabelBuilder) del(ns ...string) *LabelBuilder {
	for _, n := range ns {
		delete(l.LabelSet, n)
	}
	return l
}

func (l *LabelBuilder) labels() []prompb.Label {
	ls := make([]prompb.Label, 0, len(l.LabelSet))
	if len(l.LabelSet) == 0 {
		return ls
	}

	for k, v := range l.LabelSet {
		ls = append(ls, prompb.Label{
			Name:  k,
			Value: v,
		})
	}

	sort.Slice(ls, func(i, j int) bool {
		return ls[i].Name > ls[j].Name
	})
	return ls
}

func relabel(lset []prompb.Label, cfg *pconf.RelabelConfig) []prompb.Label {
	values := make([]string, 0, len(cfg.SourceLabels))
	for _, ln := range cfg.SourceLabels {
		values = append(values, getValue(lset, ln))
	}

	regx := cfg.RegexCompiled
	if regx == nil {
		regx = compileRegex(cfg.Regex)
	}

	if regx == nil {
		return lset
	}

	val := strings.Join(values, cfg.Separator)
	lb := newBuilder(lset)

	switch cfg.Action {
	case Drop:
		if regx.MatchString(val) {
			return nil
		}
	case Keep:
		if !regx.MatchString(val) {
			return nil
		}
	case Replace:
		return handleReplace(lb, regx, cfg, val, lset)
	case Lowercase:
		lb.set(cfg.TargetLabel, strings.ToLower(val))
	case Uppercase:
		lb.set(cfg.TargetLabel, strings.ToUpper(val))
	case HashMod:
		mod := sum64(md5.Sum([]byte(val))) % cfg.Modulus
		lb.set(cfg.TargetLabel, fmt.Sprintf("%d", mod))
	case LabelMap:
		for _, l := range lset {
			if regx.MatchString(l.Name) {
				res := regx.ReplaceAllString(l.Name, cfg.Replacement)
				lb.set(res, l.Value)
			}
		}
	case LabelDrop:
		for _, l := range lset {
			if regx.MatchString(l.Name) {
				lb.del(l.Name)
			}
		}
	case LabelKeep:
		for _, l := range lset {
			if !regx.MatchString(l.Name) {
				lb.del(l.Name)
			}
		}
	case DropIfEqual:
		return handleDropIfEqual(lb, cfg, lset)
	default:
		logger.Errorf("relabel: unknown relabel action type %q", cfg.Action)
	}

	return lb.labels()
}

func handleReplace(lb *LabelBuilder, regx *regexp.Regexp, cfg *pconf.RelabelConfig, val string, lset []prompb.Label) []prompb.Label {
	// 如果没有 source_labels，直接设置标签（新增标签）
	if len(cfg.SourceLabels) == 0 {
		lb.set(cfg.TargetLabel, cfg.Replacement)
		return lb.labels()
	}

	// 如果 Replacement 为空, separator 不为空, 则用已有标签构建新标签
	if cfg.Replacement == "" && len(cfg.SourceLabels) > 1 {
		lb.set(cfg.TargetLabel, val)
		return lb.labels()
	}

	// 处理正则表达式替换的情况（修改标签值，正则）
	if regx != nil {
		indexes := regx.FindStringSubmatchIndex(val)
		if indexes == nil {
			return lb.labels()
		}

		target := model.LabelName(cfg.TargetLabel)
		if !target.IsValid() {
			lb.del(cfg.TargetLabel)
			return lb.labels()
		}

		res := regx.ExpandString([]byte{}, cfg.Replacement, val, indexes)
		if len(res) == 0 {
			lb.del(cfg.TargetLabel)
		} else {
			lb.set(string(target), string(res))
		}

		return lb.labels()
	}

	// 默认情况，直接设置目标标签值
	lb.set(cfg.TargetLabel, cfg.Replacement)
	return lb.labels()
}

func handleDropIfEqual(lb *LabelBuilder, cfg *pconf.RelabelConfig, lset []prompb.Label) []prompb.Label {
	if len(cfg.SourceLabels) < 2 {
		return lb.labels()
	}
	firstVal := getValue(lset, cfg.SourceLabels[0])
	equal := true
	for _, label := range cfg.SourceLabels[1:] {
		if getValue(lset, label) != firstVal {
			equal = false
			break
		}
	}
	if equal {
		return nil
	}
	return lb.labels()
}

func compileRegex(expr string) *regexp.Regexp {
	regex, err := regexp.Compile(expr)
	if err != nil {
		logger.Error("failed to compile regexp:", expr, "error:", err)
		return nil
	}

	return regex
}

func sum64(hash [md5.Size]byte) uint64 {
	var s uint64

	for i, b := range hash {
		shift := uint64((md5.Size - i - 1) * 8)

		s |= uint64(b) << shift
	}
	return s
}
