package models

import (
	"crypto/md5"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
)

const (
	Replace   Action = "replace"
	Keep      Action = "keep"
	Drop      Action = "drop"
	HashMod   Action = "hashmod"
	LabelMap  Action = "labelmap"
	LabelDrop Action = "labeldrop"
	LabelKeep Action = "labelkeep"
	Lowercase Action = "lowercase"
	Uppercase Action = "uppercase"
)

type Action string

type Regexp struct {
	*regexp.Regexp
}

type RelabelConfig struct {
	SourceLabels model.LabelNames
	Separator    string
	Regex        interface{}
	Modulus      uint64
	TargetLabel  string
	Replacement  string
	Action       Action
}

func Process(labels []*prompb.Label, cfgs ...*RelabelConfig) []*prompb.Label {
	for _, cfg := range cfgs {
		labels = relabel(labels, cfg)
		if labels == nil {
			return nil
		}
	}
	return labels
}

func getValue(ls []*prompb.Label, name model.LabelName) string {
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

func newBuilder(ls []*prompb.Label) *LabelBuilder {
	lset := make(map[string]string, len(ls))
	for _, l := range ls {
		lset[l.Name] = l.Value
	}
	return &LabelBuilder{LabelSet: lset}
}

func (l *LabelBuilder) set(k, v string) *LabelBuilder {
	if v == "" {
		return l.del(k)
	}

	l.LabelSet[k] = v
	return l
}

func (l *LabelBuilder) del(ns ...string) *LabelBuilder {
	for _, n := range ns {
		delete(l.LabelSet, n)
	}
	return l
}

func (l *LabelBuilder) labels() []*prompb.Label {
	ls := make([]*prompb.Label, 0, len(l.LabelSet))
	if len(l.LabelSet) == 0 {
		return ls
	}

	for k, v := range l.LabelSet {
		ls = append(ls, &prompb.Label{
			Name:  k,
			Value: v,
		})
	}

	sort.Slice(ls, func(i, j int) bool {
		return ls[i].Name > ls[j].Name
	})
	return ls
}

func relabel(lset []*prompb.Label, cfg *RelabelConfig) []*prompb.Label {
	values := make([]string, 0, len(cfg.SourceLabels))
	for _, ln := range cfg.SourceLabels {
		values = append(values, getValue(lset, ln))
	}

	regx := cfg.Regex.(Regexp)

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
		indexes := regx.FindStringSubmatchIndex(val)
		if indexes == nil {
			break
		}
		target := model.LabelName(regx.ExpandString([]byte{}, cfg.TargetLabel, val, indexes))
		if !target.IsValid() {
			lb.del(cfg.TargetLabel)
			break
		}
		res := regx.ExpandString([]byte{}, cfg.Replacement, val, indexes)
		if len(res) == 0 {
			lb.del(cfg.TargetLabel)
			break
		}
		lb.set(string(target), string(res))
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
	default:
		panic(fmt.Errorf("relabel: unknown relabel action type %q", cfg.Action))
	}

	return lb.labels()
}

func sum64(hash [md5.Size]byte) uint64 {
	var s uint64

	for i, b := range hash {
		shift := uint64((md5.Size - i - 1) * 8)

		s |= uint64(b) << shift
	}
	return s
}

func NewRegexp(s string) (Regexp, error) {
	regex, err := regexp.Compile("^(?:" + s + ")$")
	return Regexp{Regexp: regex}, err
}

func MustNewRegexp(s string) Regexp {
	re, err := NewRegexp(s)
	if err != nil {
		panic(err)
	}
	return re
}
