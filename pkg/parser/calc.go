package parser

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/expr-lang/expr"
	exprparser "github.com/expr-lang/expr/parser"
	"github.com/toolkits/pkg/logger"
)

var defaultFuncMap = map[string]interface{}{
	"between": between,
}

// buildEnv 构造 expr 求值环境：清洗后的数据变量 + 内置函数。
func buildEnv(data map[string]interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(data)+len(defaultFuncMap))
	for k, v := range data {
		m[cleanStr(k)] = v
	}
	for k, v := range defaultFuncMap {
		m[k] = v
	}
	return m
}

func MathCalc(s string, data map[string]interface{}) (float64, error) {
	m := buildEnv(data)

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
	res, err := evalBoolExpr(s, data)
	if err != nil {
		logger.Errorf("Calc exp:%s data:%v error: %v", s, data, err)
		return false
	}

	return res == triTrue
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

func CalcWithRid(s string, data map[string]interface{}, rid int64) bool {
	res, err := evalBoolExpr(s, data)
	if err != nil {
		logger.Errorf("rid:%d exp:%s data:%v error: %v", rid, s, data, err)
		return false
	}

	return res == triTrue
}

// triState 表示子条件的三值逻辑结果。triError 表示子条件因变量缺失/无数据求值报错，
// 在布尔运算中按 false 处理，但保留该状态以便正确处理取反（! 作用在无数据子条件上时保守地不触发）。
type triState int

const (
	triFalse triState = iota
	triTrue
	triError
)

// evalBoolExpr 按布尔运算符优先级（! 紧于 && 紧于 ||，括号优先）解析并求值高级表达式。
// 与直接交给 expr 整体求值不同，这里把表达式拆成若干子条件分别求值再按布尔逻辑合并，
// 使得某个子条件因变量缺失/无数据报错时仅把该子条件降级为 false（取反时按报错传播），
// 而不会让整条表达式直接判为不满足。子条件内部仍由 MathCalc 求值，因此有数据时结果与改造前一致。
// 返回的 error 仅代表表达式语法/类型等硬错误，维持原有行为：判 false 并由调用方记录日志。
func evalBoolExpr(s string, data map[string]interface{}) (triState, error) {
	// 求值前做一次硬错误校验：维持改造前 MathCalc 整体编译的行为，让语法/类型/未知函数等错误
	// 整条判 false 并记录日志，避免被短路跳过的分支或字符串切分掩盖配置错误。仅变量缺失/无数据
	// 允许在随后的三值短路求值中按布尔逻辑降级；运行期错误（如下标越界）不在此拦截，仍由短路求值决定。
	if err := validateExpr(s, data); err != nil {
		return triFalse, err
	}
	return evalOr(s, data)
}

// validateExpr 在降级求值前校验表达式的硬错误：
//  1. 先对整条表达式做语法解析，捕获按顶层运算符切分会绕过的整体语法错误
//     （如未加括号混用 ?? 与 &&/||、括号不匹配等）。
//  2. 再逐个叶子编译，未知函数/类型错误按硬错误返回，变量缺失/无数据（isNoDataErr）放行后续降级。
func validateExpr(s string, data map[string]interface{}) error {
	if _, err := exprparser.Parse(cleanStr(s)); err != nil {
		return err
	}

	env := buildEnv(data)
	return forEachLeaf(s, func(leaf string) error {
		if _, err := expr.Compile(cleanStr(leaf), expr.Env(env)); err != nil && !isNoDataErr(err, leaf) {
			return err
		}
		return nil
	})
}

// forEachLeaf 以与求值（evalOr/evalAnd/evalUnary/evalPrimary）完全一致的方式拆出所有叶子子条件，
// 对每个叶子调用 fn 并返回首个非 nil 错误。调整拆分逻辑时务必同步两处，确保校验与求值看到相同的叶子。
func forEachLeaf(s string, fn func(leaf string) error) error {
	s = strings.TrimSpace(s)
	if s == "" || hasTopLevelTernary(s) {
		return fn(s)
	}
	if parts := splitTopLevel(s, "||", "or"); len(parts) > 1 {
		return forEachLeafParts(parts, fn)
	}
	if parts := splitTopLevel(s, "&&", "and"); len(parts) > 1 {
		return forEachLeafParts(parts, fn)
	}
	if rest, ok := trimNotPrefix(s); ok {
		return forEachLeaf(rest, fn)
	}
	if isFullyWrapped(s) {
		return forEachLeaf(s[1:len(s)-1], fn)
	}
	return fn(s)
}

func forEachLeafParts(parts []string, fn func(leaf string) error) error {
	for _, p := range parts {
		if err := forEachLeaf(p, fn); err != nil {
			return err
		}
	}
	return nil
}

func evalOr(s string, data map[string]interface{}) (triState, error) {
	// 三元运算符 ? : 的优先级低于 || && 且不增加括号深度，含顶层三元时根节点是三元表达式
	// 而非逻辑运算，整体作为子条件交给 MathCalc 求值，避免误把分支/条件里的 || && 当成顶层逻辑。
	if hasTopLevelTernary(s) {
		return evalLeaf(strings.TrimSpace(s), data)
	}

	parts := splitTopLevel(s, "||", "or")
	if len(parts) == 1 {
		return evalAnd(parts[0], data)
	}

	// 从左到右求值并短路：遇到真立即返回真，保留 expr 原本的短路语义，
	// 避免求值本不该执行的右侧子条件（否则 $.A > 0 || $.Arr[0] > 0 在左侧已为真时
	// 仍会触发右侧的运行期错误）。报错项（无数据）只记未知态后继续，寻找能决定结果的后续项。
	anyError := false
	for _, p := range parts {
		r, err := evalAnd(p, data)
		if err != nil {
			return triFalse, err
		}
		switch r {
		case triTrue:
			return triTrue, nil
		case triError:
			anyError = true
		}
	}

	// 无项为真：任一报错则按 Kleene OR 返回报错（顶层视为不触发），否则为假。
	if anyError {
		return triError, nil
	}
	return triFalse, nil
}

func evalAnd(s string, data map[string]interface{}) (triState, error) {
	parts := splitTopLevel(s, "&&", "and")
	if len(parts) == 1 {
		return evalUnary(parts[0], data)
	}

	// 从左到右求值并短路：遇到假立即返回假。报错项（无数据）只记未知态后继续，寻找能决定结果的后续项。
	anyError := false
	for _, p := range parts {
		r, err := evalUnary(p, data)
		if err != nil {
			return triFalse, err
		}
		switch r {
		case triFalse:
			return triFalse, nil
		case triError:
			anyError = true
		}
	}

	// 无项为假：任一报错则按 Kleene AND 返回报错（顶层视为不触发），否则为真。
	if anyError {
		return triError, nil
	}
	return triTrue, nil
}

func evalUnary(s string, data map[string]interface{}) (triState, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return triFalse, fmt.Errorf("empty sub-expression")
	}

	if rest, ok := trimNotPrefix(s); ok {
		r, err := evalUnary(rest, data)
		if err != nil {
			return triFalse, err
		}
		return notTri(r), nil
	}

	return evalPrimary(s, data)
}

func evalPrimary(s string, data map[string]interface{}) (triState, error) {
	s = strings.TrimSpace(s)
	if isFullyWrapped(s) {
		return evalOr(s[1:len(s)-1], data)
	}
	return evalLeaf(s, data)
}

// evalLeaf 求值一个不含顶层布尔运算符的子条件。
// 变量缺失/无数据导致的报错降级为 triError；其余报错（语法/类型错误）作为硬错误返回。
func evalLeaf(s string, data map[string]interface{}) (triState, error) {
	v, err := MathCalc(s, data)
	if err != nil {
		if isNoDataErr(err, s) {
			return triError, nil
		}
		return triFalse, err
	}
	if v > 0 {
		return triTrue, nil
	}
	return triFalse, nil
}

func notTri(r triState) triState {
	switch r {
	case triTrue:
		return triFalse
	case triFalse:
		return triTrue
	default:
		return triError
	}
}

// isNoDataErr 判断 expr 报错是否为子条件引用的指标变量/字段缺失（即无数据），这是需要降级的情况。
// expr 对缺失变量和未知函数都报 "unknown name X"，二者仅能从语法上区分：函数名后紧跟左括号。
// 因此当 X 作为函数调用出现时（如拼错的 betwen(...)）按硬错误处理，维持原有「判 false 并记录日志」
// 的行为，避免把配置错误悄悄降级、掩盖问题甚至误触发。
func isNoDataErr(err error, s string) bool {
	if err == nil {
		return false
	}

	const marker = "unknown name "
	idx := strings.Index(err.Error(), marker)
	if idx < 0 {
		return false
	}

	// 取出未知名字，expr 报错形如 "unknown name betwen (1:1)"，名字到首个空白或左括号为止。
	name := err.Error()[idx+len(marker):]
	if end := strings.IndexAny(name, " \t\n("); end >= 0 {
		name = name[:end]
	}
	if name == "" {
		return false
	}

	// 名字在表达式中作为函数调用出现（后跟左括号）则是未知函数而非无数据变量。
	return !isFuncCall(cleanStr(s), name)
}

// isFuncCall 判断 name 是否在 expr 中以函数调用形式出现（名字后紧跟左括号，允许中间有空白）。
func isFuncCall(expr, name string) bool {
	for from := 0; from < len(expr); {
		i := strings.Index(expr[from:], name)
		if i < 0 {
			return false
		}
		j := from + i + len(name)
		for j < len(expr) && (expr[j] == ' ' || expr[j] == '\t') {
			j++
		}
		if j < len(expr) && expr[j] == '(' {
			return true
		}
		from = from + i + len(name)
	}
	return false
}

// trimNotPrefix 识别作为一元前缀的取反运算符（! 或独立单词 not），返回去掉前缀后的剩余表达式。
// 不会误伤比较运算符 != 以及二元运算符 not in / not contains 等（它们左侧有操作数，不在表达式开头）。
func trimNotPrefix(s string) (string, bool) {
	if s[0] == '!' {
		if len(s) > 1 && s[1] == '=' {
			return "", false
		}
		return s[1:], true
	}
	if strings.HasPrefix(s, "not") {
		rest := s[len("not"):]
		if rest == "" {
			return "", false
		}
		if r, _ := utf8.DecodeRuneInString(rest); !isIdentRune(r) {
			return rest, true
		}
	}
	return "", false
}

// splitTopLevel 按指定布尔运算符在最外层切分表达式：symbol 为符号形式（如 "||"），
// word 为单词形式（如 "or"）。跳过括号/中括号/花括号及字符串字面量内部的运算符，
// word 形式额外要求两侧为单词边界，避免误切 error、$A.or 等标识符。
func splitTopLevel(s, symbol, word string) []string {
	var parts []string
	depth := 0
	start := 0
	inStr := false
	var quote byte

	for i := 0; i < len(s); {
		c := s[i]
		if inStr {
			// 反引号为原始字符串，不处理转义
			if quote != '`' && c == '\\' {
				i += 2
				continue
			}
			if c == quote {
				inStr = false
			}
			i++
			continue
		}

		switch c {
		case '"', '\'', '`':
			inStr = true
			quote = c
			i++
			continue
		case '(', '[', '{':
			depth++
			i++
			continue
		case ')', ']', '}':
			depth--
			i++
			continue
		}

		if depth == 0 {
			if strings.HasPrefix(s[i:], symbol) {
				parts = append(parts, s[start:i])
				i += len(symbol)
				start = i
				continue
			}
			if strings.HasPrefix(s[i:], word) && isWordBoundary(s, i, len(word)) {
				parts = append(parts, s[start:i])
				i += len(word)
				start = i
				continue
			}
		}
		i++
	}

	parts = append(parts, s[start:])
	return parts
}

// isWordBoundary 判断 s 中 [i, i+n) 处的单词运算符两侧是否均为单词边界。
func isWordBoundary(s string, i, n int) bool {
	if i > 0 {
		if r, _ := utf8.DecodeLastRuneInString(s[:i]); isIdentRune(r) {
			return false
		}
	}
	if j := i + n; j < len(s) {
		if r, _ := utf8.DecodeRuneInString(s[j:]); isIdentRune(r) {
			return false
		}
	}
	return true
}

func isIdentRune(r rune) bool {
	return r == '_' || r == '.' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// isFullyWrapped 判断表达式是否被一对最外层括号完整包裹，如 (a && b)，
// 而非 (a) || (b) 这种首尾虽为括号但并非整体包裹的情况。
func isFullyWrapped(s string) bool {
	if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
		return false
	}

	depth := 0
	inStr := false
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if quote != '`' && c == '\\' {
				i++
				continue
			}
			if c == quote {
				inStr = false
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = true
			quote = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(s)-1 {
				return false
			}
		}
	}
	return depth == 0
}

// hasTopLevelTernary 判断表达式最外层是否含三元运算符 ? :（不含括号/字符串内部）。
// 需排除可选链 ?. 与空值合并 ??，二者不是三元运算。
func hasTopLevelTernary(s string) bool {
	depth := 0
	inStr := false
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if quote != '`' && c == '\\' {
				i++
				continue
			}
			if c == quote {
				inStr = false
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			inStr = true
			quote = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '?':
			if depth != 0 {
				continue
			}
			if i+1 < len(s) && (s[i+1] == '.' || s[i+1] == '?') {
				i++ // 跳过 ?. 或 ?? 的第二个字符
				continue
			}
			return true
		}
	}
	return false
}
