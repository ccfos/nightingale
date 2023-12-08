package pconf

import (
	"log"
	"regexp"

	"github.com/ccfos/nightingale/v6/pkg/tlsx"

	"github.com/prometheus/common/model"
)

type Pushgw struct {
	BusiGroupLabelKey   string
	IdentMetrics        []string
	IdentStatsThreshold int
	WriteConcurrency    int
	LabelRewrite        bool
	ForceUseServerTS    bool
	DebugSample         map[string]string
	DropSample          []map[string]string
	WriterOpt           WriterGlobalOpt
	Writers             []WriterOptions
}

type WriterGlobalOpt struct {
	QueueMaxSize int
	QueuePopSize int
}

type WriterOptions struct {
	Url           string
	BasicAuthUser string
	BasicAuthPass string

	Timeout               int64
	DialTimeout           int64
	TLSHandshakeTimeout   int64
	ExpectContinueTimeout int64
	IdleConnTimeout       int64
	KeepAlive             int64

	MaxConnsPerHost     int
	MaxIdleConns        int
	MaxIdleConnsPerHost int

	Headers []string

	WriteRelabels []*RelabelConfig

	tlsx.ClientConfig
}

type RelabelConfig struct {
	SourceLabels  model.LabelNames
	Separator     string
	Regex         string
	RegexCompiled *regexp.Regexp
	Modulus       uint64
	TargetLabel   string
	Replacement   string
	Action        string
}

func (p *Pushgw) PreCheck() {
	if p.BusiGroupLabelKey == "" {
		p.BusiGroupLabelKey = "busigroup"
	}

	if p.WriterOpt.QueueMaxSize <= 0 {
		p.WriterOpt.QueueMaxSize = 10000000
	}

	if p.WriterOpt.QueuePopSize <= 0 {
		p.WriterOpt.QueuePopSize = 1000
	}

	if p.WriteConcurrency <= 0 {
		p.WriteConcurrency = 5000
	}

	if p.IdentStatsThreshold <= 0 {
		p.IdentStatsThreshold = 400
	}

	for _, writer := range p.Writers {
		for _, relabel := range writer.WriteRelabels {
			if relabel.Regex == "" {
				relabel.Regex = "(.*)"
			}

			regex, err := regexp.Compile("^(?:" + relabel.Regex + ")$")
			if err != nil {
				log.Fatalln("failed to compile regexp:", relabel.Regex, "error:", err)
			}

			relabel.RegexCompiled = regex

			if relabel.Separator == "" {
				relabel.Separator = ";"
			}

			if relabel.Action == "" {
				relabel.Action = "replace"
			}

			if relabel.Replacement == "" {
				relabel.Replacement = "$1"
			}
		}
	}
}
