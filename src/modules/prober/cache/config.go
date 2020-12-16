package cache

import (
	"bytes"
	"fmt"
	"go/scanner"
	"go/token"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/didi/nightingale/src/modules/monapi/collector"
	"github.com/didi/nightingale/src/modules/prober/config"
	"github.com/influxdata/telegraf"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/yaml.v2"
)

type MetricConfig struct {
	Name      string    `yaml:"name"`
	Type      string    `yaml:"type"`
	Comment   string    `yaml:"comment"`
	Expr      string    `yaml:"expr"`
	notations Notations `yaml:"-"`
}

type PluginConfig struct {
	Metrics []MetricConfig `metrics`
}

var (
	metricsConfig map[string]MetricConfig
	metricsExpr   map[string]map[string]MetricConfig
	ignoreConfig  bool
)

func InitPluginsConfig(cf *config.ConfYaml) {
	metricsConfig = make(map[string]MetricConfig)
	metricsExpr = make(map[string]map[string]MetricConfig)
	ignoreConfig = cf.IgnoreConfig
	plugins := collector.GetRemoteCollectors()
	for _, plugin := range plugins {
		metricsExpr[plugin] = make(map[string]MetricConfig)
		pluginConfig := PluginConfig{}

		file := filepath.Join(cf.PluginsConfig, plugin+".yml")
		b, err := ioutil.ReadFile(file)
		if err != nil {
			logger.Debugf("readfile %s err %s", plugin, err)
			continue
		}

		if err := yaml.Unmarshal(b, &pluginConfig); err != nil {
			logger.Warningf("yaml.Unmarshal %s err %s", plugin, err)
			continue
		}

		for _, v := range pluginConfig.Metrics {
			if _, ok := metricsConfig[v.Name]; ok {
				panic(fmt.Sprintf("plugin %s metrics %s is already exists", plugin, v.Name))
			}
			if v.Expr == "" {
				// nomore
				metricsConfig[v.Name] = v
			} else {
				err := v.parse()
				if err != nil {
					panic(fmt.Sprintf("plugin %s metrics %s expr %s parse err %s",
						plugin, v.Name, v.Expr, err))
				}
				metricsExpr[plugin][v.Name] = v

			}
		}
		logger.Infof("loaded plugin config %s", file)
	}
}

func (p *MetricConfig) parse() (err error) {
	p.notations, err = rePolish([]byte(p.Expr))
	return
}

func (p *MetricConfig) Calc(vars map[string]float64) (float64, error) {
	return calc(p.notations, vars)
}

func Metric(metric string, typ telegraf.ValueType) (c MetricConfig, ok bool) {
	c, ok = metricsConfig[metric]
	if !ok && !ignoreConfig {
		return
	}

	if c.Type == "" {
		c.Type = metricType(typ)
	}

	return
}

func GetMetricExprs(pluginName string) (c map[string]MetricConfig, ok bool) {
	c, ok = metricsExpr[pluginName]
	return
}

func metricType(typ telegraf.ValueType) string {
	switch typ {
	case telegraf.Counter:
		return "COUNTER"
	case telegraf.Gauge:
		return "GAUGE"
	case telegraf.Untyped:
		return "GAUGE"
	case telegraf.Summary: // TODO
		return "SUMMARY"
	case telegraf.Histogram: // TODO
		return "HISTOGRAM"
	default:
		return "GAUGE"
	}
}

type tokenType int

const (
	tokenOperator tokenType = iota
	tokenVar
	tokenConst
)

type TokenNotation struct {
	tokenType tokenType
	o         token.Token // operator
	v         string      // variable
	c         float64     // const
}

type Notations []*TokenNotation

func (s *Notations) Push(tn *TokenNotation) { *s = append(*s, tn) }
func (s *Notations) Pop() *TokenNotation    { n := (*s)[len(*s)-1]; *s = (*s)[:len(*s)-1]; return n }
func (s *Notations) Top() *TokenNotation    { return (*s)[len(*s)-1] }
func (s *Notations) Len() int               { return len(*s) }

func (s Notations) String() string {
	out := bytes.NewBuffer(nil)
	for i := 0; i < len(s); i++ {
		tn := s[i]
		switch tn.tokenType {
		case tokenOperator:
			out.WriteString(tn.o.String() + " ")
		case tokenVar:
			out.WriteString(tn.v + " ")
		case tokenConst:
			out.WriteString(fmt.Sprintf("%.0f ", tn.c))
		}
	}
	return out.String()
}

type StackOp []token.Token

func (s *StackOp) Push(t token.Token) { *s = append(*s, t) }
func (s *StackOp) Pop() token.Token   { n := (*s)[len(*s)-1]; *s = (*s)[:len(*s)-1]; return n }
func (s *StackOp) Top() token.Token   { return (*s)[len(*s)-1] }
func (s *StackOp) Len() int           { return len(*s) }

type StackFloat []float64

func (s *StackFloat) Push(f float64) { *s = append(*s, f) }
func (s *StackFloat) Pop() float64   { n := (*s)[len(*s)-1]; *s = (*s)[:len(*s)-1]; return n }
func (s *StackFloat) Len() int       { return len(*s) }

func priority(tok token.Token) int {
	switch tok {
	case token.ADD, token.SUB:
		return 1
	case token.MUL, token.QUO:
		return 2
	default:
		return 0
	}
}

func calc(rpn Notations, vars map[string]float64) (float64, error) {
	var s StackFloat
	for i := 0; i < rpn.Len(); i++ {
		tn := rpn[i]
		switch tn.tokenType {
		case tokenVar:
			if v, ok := vars[tn.v]; !ok {
				return 0, fmt.Errorf("variable %s is not set", tn.v)
			} else {
				s.Push(v)
			}
		case tokenConst:
			s.Push(tn.c)
		case tokenOperator:
			op1 := s.Pop()
			op2 := s.Pop()
			switch tn.o {
			case token.ADD:
				s.Push(op1 + op2)
			case token.SUB:
				s.Push(op1 - op2)
			case token.MUL:
				s.Push(op1 * op2)
			case token.QUO:
				s.Push(op1 / op2)
			}
		}
	}
	if s.Len() == 1 {
		return s[0], nil
	}

	return 0, fmt.Errorf("invalid calc, stack len %d expect 1", s.Len())
}

// return reverse polish notation stack
func rePolish(src []byte) (output Notations, err error) {
	var scan scanner.Scanner
	var s StackOp

	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(src))

	scan.Init(file, src, errorHandler, scanner.ScanComments)
	var (
		pos token.Pos
		tok token.Token
		lit string
	)
	for {
		pos, tok, lit = scan.Scan()

		switch tok {
		case token.EOF, token.SEMICOLON:
			goto out
		case token.INT, token.FLOAT:
			c, err := strconv.ParseFloat(lit, 64)
			if err != nil {
				return nil, fmt.Errorf("parseFloat error %s\t%s\t%q",
					fset.Position(pos), tok, lit)
			}
			output.Push(&TokenNotation{tokenType: tokenConst, c: c})
		case token.IDENT:
			output.Push(&TokenNotation{tokenType: tokenVar, v: lit})
		case token.LPAREN: // (
			s.Push(tok)
		case token.ADD, token.SUB, token.MUL, token.QUO: // + - * /
		opRetry:
			if s.Len() == 0 {
				s.Push(tok)
			} else if op := s.Top(); op == token.LPAREN || priority(tok) > priority(op) {
				s.Push(tok)
			} else {
				output.Push(&TokenNotation{tokenType: tokenOperator, o: s.Pop()})
				goto opRetry
			}
		case token.RPAREN: // )
			for s.Len() > 0 {
				if op := s.Pop(); op == token.LPAREN {
					break
				} else {
					output.Push(&TokenNotation{tokenType: tokenOperator, o: op})
				}
			}
		default:
			return nil, fmt.Errorf("unsupport token %s", tok)
		}
	}

out:
	for i, l := 0, s.Len(); i < l; i++ {
		output.Push(&TokenNotation{tokenType: tokenOperator, o: s.Pop()})
	}
	return
}

func errorHandler(pos token.Position, msg string) {
	fmt.Fprintf(os.Stderr, "error %s\t%s\n", pos, msg)
}
