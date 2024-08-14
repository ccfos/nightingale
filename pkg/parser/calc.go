package parser

import (
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/toolkits/pkg/logger"
)

var defaultFuncMap = map[string]interface{}{
	"between": between,
}

func MathCalc(s string, data map[string]interface{}) (float64, error) {
	m := make(map[string]interface{})
	for k, v := range data {
		m[cleanStr(k)] = v
	}

	for k, v := range defaultFuncMap {
		m[k] = v
	}

	// 表达式要求类型一致，否则此处编译会报错
	program, err := expr.Compile(cleanStr(s), expr.Env(m))
	if err != nil {
		return 0, err
	}

	output, err := expr.Run(program, m)
	if err != nil {
		return 0, err
	}

	if result, ok := output.(float64); ok {
		return result, nil
	} else if result, ok := output.(bool); ok {
		if result {
			return 1, nil
		} else {
			return 0, nil
		}
	} else if result, ok := output.(int); ok {
		return float64(result), nil
	} else {
		return 0, nil
	}
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

func replaceDollarSigns(s string) string {
	re := regexp.MustCompile(`\$([A-Z])\.`)
	return re.ReplaceAllString(s, "${1}_")
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
