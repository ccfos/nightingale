package macros

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/gtime"
)

const rsIdentifier = `([_a-zA-Z0-9]+)`
const sExpr = `\$` + rsIdentifier + `\(([^\)]*)\)`

var restrictedRegExp = regexp.MustCompile(`(?im)([\s]*show[\s]+grants|[\s,]session_user\([^\)]*\)|[\s,]current_user(\([^\)]*\))?|[\s,]system_user\([^\)]*\)|[\s,]user\([^\)]*\))([\s,;]|$)`)

func ReplaceMacros(sql string, start, end int64) (string, error) {
	return Interpolate(&backend.DataQuery{}, backend.TimeRange{From: time.Unix(start, 0), To: time.Unix(end, 0)}, sql)
}

func Interpolate(query *backend.DataQuery, timeRange backend.TimeRange, sql string) (string, error) {
	matches := restrictedRegExp.FindAllStringSubmatch(sql, 1)
	if len(matches) > 0 {
		return "", fmt.Errorf("invalid query")
	}

	// TODO: Handle error
	rExp, _ := regexp.Compile(sExpr)
	var macroError error

	sql = ReplaceAllStringSubmatchFunc(rExp, sql, func(groups []string) string {
		args := strings.Split(groups[2], ",")
		for i, arg := range args {
			args[i] = strings.Trim(arg, " ")
		}
		res, err := evaluateMacro(timeRange, query, groups[1], args)
		if err != nil && macroError == nil {
			macroError = err
			return "macro_error()"
		}
		return res
	})

	if macroError != nil {
		return "", macroError
	}

	return sql, nil
}

func evaluateMacro(timeRange backend.TimeRange, query *backend.DataQuery, name string, args []string) (string, error) {
	switch name {
	case "__timeEpoch", "__time":
		if len(args) == 0 {
			return "", fmt.Errorf("missing time column argument for macro %v", name)
		}
		return fmt.Sprintf("UNIX_TIMESTAMP(%s) as time_sec", args[0]), nil
	case "__timeFilter":
		if len(args) == 0 {
			return "", fmt.Errorf("missing time column argument for macro %v", name)
		}
		if timeRange.From.UTC().Unix() < 0 {
			return fmt.Sprintf("%s BETWEEN DATE_ADD(FROM_UNIXTIME(0), INTERVAL %d SECOND) AND FROM_UNIXTIME(%d)", args[0], timeRange.From.UTC().Unix(), timeRange.To.UTC().Unix()), nil
		}
		return fmt.Sprintf("%s BETWEEN FROM_UNIXTIME(%d) AND FROM_UNIXTIME(%d)", args[0], timeRange.From.UTC().Unix(), timeRange.To.UTC().Unix()), nil
	case "__timeFrom":
		return fmt.Sprintf("FROM_UNIXTIME(%d)", timeRange.From.UTC().Unix()), nil
	case "__timeTo":
		return fmt.Sprintf("FROM_UNIXTIME(%d)", timeRange.To.UTC().Unix()), nil
	case "__unixEpochFrom":
		return fmt.Sprintf("%d", timeRange.From.UTC().Unix()), nil
	case "__unixEpochTo":
		return fmt.Sprintf("%d", timeRange.To.UTC().Unix()), nil
	case "__timeGroup":
		if len(args) < 2 {
			return "", fmt.Errorf("macro %v needs time column and interval arg:%+v", name, args)
		}
		interval, err := gtime.ParseInterval(strings.Trim(args[1], `'"`))
		if err != nil {
			return "", fmt.Errorf("error parsing interval %v", args[1])
		}
		return fmt.Sprintf("FLOOR(UNIX_TIMESTAMP(%s) DIV %.0f) * %.0f", args[0], interval.Seconds(), interval.Seconds()), nil
	case "__timeGroupAlias":
		tg, err := evaluateMacro(timeRange, query, "__timeGroup", args)
		if err == nil {
			return tg + " AS \"time\"", nil
		}
		return "", err
	case "__unixEpochFilter":
		if len(args) == 0 {
			return "", fmt.Errorf("missing time column argument for macro %v", name)
		}
		return fmt.Sprintf("%s >= %d AND %s <= %d", args[0], timeRange.From.UTC().Unix(), args[0], timeRange.To.UTC().Unix()), nil
	case "__unixEpochNanoFilter":
		if len(args) == 0 {
			return "", fmt.Errorf("missing time column argument for macro %v", name)
		}
		return fmt.Sprintf("%s >= %d AND %s <= %d", args[0], timeRange.From.UTC().UnixNano(), args[0], timeRange.To.UTC().UnixNano()), nil
	case "__unixEpochNanoFrom":
		return fmt.Sprintf("%d", timeRange.From.UTC().UnixNano()), nil
	case "__unixEpochNanoTo":
		return fmt.Sprintf("%d", timeRange.To.UTC().UnixNano()), nil
	case "__unixEpochGroup":
		if len(args) < 2 {
			return "", fmt.Errorf("macro %v needs time column and interval and optional fill value", name)
		}
		interval, err := gtime.ParseInterval(strings.Trim(args[1], `'`))
		if err != nil {
			return "", fmt.Errorf("error parsing interval %v", args[1])
		}
		return fmt.Sprintf("FLOOR(%s DIV %v) * %v", args[0], interval.Seconds(), interval.Seconds()), nil
	case "__unixEpochGroupAlias":
		tg, err := evaluateMacro(timeRange, query, "__unixEpochGroup", args)
		if err == nil {
			return tg + " AS \"time\"", nil
		}
		return "", err
	case "__timeISOFrom":
		return fmt.Sprintf("'%s'", timeRange.From.UTC().Format(time.RFC3339)), nil
	case "__timeISOTo":
		return fmt.Sprintf("'%s'", timeRange.To.UTC().Format(time.RFC3339)), nil
	default:
		return "", fmt.Errorf("unknown macro %v", name)
	}
}

func ReplaceAllStringSubmatchFunc(re *regexp.Regexp, str string, repl func([]string) string) string {
	result := ""
	lastIndex := 0

	for _, v := range re.FindAllStringSubmatchIndex(str, -1) {
		groups := []string{}
		for i := 0; i < len(v); i += 2 {
			groups = append(groups, str[v[i]:v[i+1]])
		}

		result += str[lastIndex:v[0]] + repl(groups)
		lastIndex = v[1]
	}

	return result + str[lastIndex:]
}
