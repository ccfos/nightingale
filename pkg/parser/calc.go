package parser

import (
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/toolkits/pkg/logger"
)

var defaultFuncMap = map[string]interface{}{
	"between": between,
}

// MathProgram 是编译好的表达式，可以对多组同形状的输入反复求值。同一表达式在
// 成百上千个时间点上求值时，逐点走 MathCalc 会把 expr.Compile 也重复成百上千次，
// 而编译才是这里的大头。
//
// 非并发安全：Run 复用同一个 env map 以省掉每次求值的分配与 key 规范化。
type MathProgram struct {
	program *vm.Program
	// env 的 key 已经过 cleanStr 规范化，Run 只覆盖其中的变量值。
	env map[string]interface{}
	// keys 记录调用方原始 key 到规范化 key 的映射，免得每次 Run 重跑正则。
	keys map[string]string
}

// CompileMath 用 data 的形状做类型检查并编译表达式。后续 Run 传入的 data 必须与
// 这里的 key 集合和值类型一致，否则求值会报类型错误。
func CompileMath(s string, data map[string]interface{}) (*MathProgram, error) {
	p := &MathProgram{
		env:  make(map[string]interface{}, len(data)+len(defaultFuncMap)),
		keys: make(map[string]string, len(data)),
	}
	for k, v := range data {
		clean := cleanStr(k)
		p.keys[k] = clean
		p.env[clean] = v
	}

	for k, v := range defaultFuncMap {
		p.env[k] = v
	}

	// 表达式要求类型一致，否则此处编译会报错
	program, err := expr.Compile(cleanStr(s), expr.Env(p.env))
	if err != nil {
		return nil, err
	}
	p.program = program
	return p, nil
}

// Run 用一组新的变量值求值。data 里出现的新 key 会现场规范化，因此与编译期形状
// 不一致时表现为求值报错，而不是被静默忽略。
func (p *MathProgram) Run(data map[string]interface{}) (float64, error) {
	for k, v := range data {
		clean, ok := p.keys[k]
		if !ok {
			clean = cleanStr(k)
			p.keys[k] = clean
		}
		p.env[clean] = v
	}

	output, err := expr.Run(p.program, p.env)
	if err != nil {
		return 0, err
	}

	return toFloat64(output), nil
}

func MathCalc(s string, data map[string]interface{}) (float64, error) {
	program, err := CompileMath(s, data)
	if err != nil {
		return 0, err
	}

	return program.Run(data)
}

func toFloat64(output interface{}) float64 {
	if result, ok := output.(float64); ok {
		return result
	} else if result, ok := output.(bool); ok {
		if result {
			return 1
		} else {
			return 0
		}
	} else if result, ok := output.(int); ok {
		return float64(result)
	} else {
		return 0
	}
}

// ValidateExp 只做表达式的语法编译检查（不校验变量存在性与类型），
// 用于配置期校验告警判断条件；运行期求值请用 Calc/MathCalc。
// 不带 Env 编译时 expr 对未知变量走运行期动态解析，因此这里只会报语法错误。
func ValidateExp(s string) error {
	_, err := expr.Compile(cleanStr(s))
	return err
}

func Calc(s string, data map[string]interface{}) bool {
	v, err := MathCalc(s, data)
	if err != nil {
		logger.Errorf("Calc exp:%s data:%v error: %v", s, data, err)
		return false
	}

	return v > 0
}

func cleanStr(s string) string {
	s = replaceDollarSigns(s)
	s = strings.ReplaceAll(s, "$.", "")
	return s
}

// 包级编译：cleanStr 会被逐点求值的热路径调用，MustCompile 放在函数里等于每次
// 调用重新编译一遍正则。
var dollarRefPattern = regexp.MustCompile(`\$([A-Z])\.`)

func replaceDollarSigns(s string) string {
	return dollarRefPattern.ReplaceAllString(s, "${1}_")
}

// 自定义 expr 函数
// between 函数，判断 target 是否在 arr[0] 和 arr[1] 之间
func between(target float64, arr []interface{}) bool {
	if len(arr) != 2 {
		return false
	}

	var min, max float64
	switch arr[0].(type) {
	case float64:
		min = arr[0].(float64)
	case int:
		min = float64(arr[0].(int))
	default:
		return false
	}

	switch arr[1].(type) {
	case float64:
		max = arr[1].(float64)
	case int:
		max = float64(arr[1].(int))
	default:
		return false
	}

	return target >= min && target <= max
}

func CalcWithRid(s string, data map[string]interface{}, rid int64) bool {
	v, err := MathCalc(s, data)
	if err != nil {
		logger.Errorf("rid:%d exp:%s data:%v error: %v", rid, s, data, err)
		return false
	}

	return v > 0
}
