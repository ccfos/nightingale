package parser

import (
	"regexp"
	"strings"

	"github.com/expr-lang/expr"
	"github.com/toolkits/pkg/logger"
)

func MathCalc(s string, data map[string]float64) (float64, error) {
	m := make(map[string]float64)
	for k, v := range data {
		m[cleanStr(k)] = v
	}

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
	} else {
		return 0, nil
	}
}

func Calc(s string, data map[string]float64) bool {
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
