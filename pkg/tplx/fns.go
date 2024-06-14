package tplx

import (
	"errors"
	"fmt"
	"html/template"
	"math"
	"net"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/util/strutil"
)

var (
	errNaNOrInf = errors.New("value is NaN or Inf")
)

type sample struct {
	Labels map[string]string
	Value  float64
}

type queryResult []*sample

type queryResultByLabelSorter struct {
	results queryResult
	by      string
}

func (q queryResultByLabelSorter) Len() int {
	return len(q.results)
}

func (q queryResultByLabelSorter) Less(i, j int) bool {
	return q.results[i].Labels[q.by] < q.results[j].Labels[q.by]
}

func (q queryResultByLabelSorter) Swap(i, j int) {
	q.results[i], q.results[j] = q.results[j], q.results[i]
}

func Unescaped(str string) interface{} {
	return template.HTML(str)
}

func Urlconvert(str string) interface{} {
	return template.URL(str)
}

func Timeformat(ts int64, pattern ...string) string {
	defp := "2006-01-02 15:04:05"
	if len(pattern) > 0 {
		defp = pattern[0]
	}
	return time.Unix(ts, 0).Format(defp)
}

func Timestamp(pattern ...string) string {
	defp := "2006-01-02 15:04:05"
	if len(pattern) > 0 {
		defp = pattern[0]
	}
	return time.Now().Format(defp)
}

func Now() time.Time {
	return time.Now()
}

func Args(args ...interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for i, a := range args {
		result[fmt.Sprintf("arg%d", i)] = a
	}
	return result
}

func ReReplaceAll(pattern, repl, text string) string {
	re := regexp.MustCompile(pattern)
	return re.ReplaceAllString(text, repl)
}

func Humanize(s string) string {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	if v == 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Sprintf("%.2f", v)
	}
	if math.Abs(v) >= 1 {
		prefix := ""
		for _, p := range []string{"k", "M", "G", "T", "P", "E", "Z", "Y"} {
			if math.Abs(v) < 1000 {
				break
			}
			prefix = p
			v /= 1000
		}
		return fmt.Sprintf("%.2f%s", v, prefix)
	}
	prefix := ""
	for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
		if math.Abs(v) >= 1 {
			break
		}
		prefix = p
		v *= 1000
	}
	return fmt.Sprintf("%.2f%s", v, prefix)
}

func Humanize1024(s string) string {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	if math.Abs(v) <= 1 || math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Sprintf("%.4g", v)
	}
	prefix := ""
	for _, p := range []string{"ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi", "Yi"} {
		if math.Abs(v) < 1024 {
			break
		}
		prefix = p
		v /= 1024
	}
	return fmt.Sprintf("%.4g%s", v, prefix)
}

func ToString(v interface{}) string {
	return fmt.Sprint(v)
}

func HumanizeDuration(s string) string {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	return HumanizeDurationFloat64(v)
}

func HumanizeDurationInterface(i interface{}) string {
	f, err := ToFloat64(i)
	if err != nil {
		return ToString(i)
	}
	return HumanizeDurationFloat64(f)
}

func HumanizeDurationFloat64(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Sprintf("%.4g", v)
	}
	if v == 0 {
		return fmt.Sprintf("%.4gs", v)
	}
	if math.Abs(v) >= 1 {
		sign := ""
		if v < 0 {
			sign = "-"
			v = -v
		}
		seconds := int64(v) % 60
		minutes := (int64(v) / 60) % 60
		hours := (int64(v) / 60 / 60) % 24
		days := int64(v) / 60 / 60 / 24
		// For days to minutes, we display seconds as an integer.
		if days != 0 {
			return fmt.Sprintf("%s%dd %dh %dm %ds", sign, days, hours, minutes, seconds)
		}
		if hours != 0 {
			return fmt.Sprintf("%s%dh %dm %ds", sign, hours, minutes, seconds)
		}
		if minutes != 0 {
			return fmt.Sprintf("%s%dm %ds", sign, minutes, seconds)
		}
		// For seconds, we display 4 significant digits.
		return fmt.Sprintf("%s%.4gs", sign, v)
	}
	prefix := ""
	for _, p := range []string{"m", "u", "n", "p", "f", "a", "z", "y"} {
		if math.Abs(v) >= 1 {
			break
		}
		prefix = p
		v *= 1000
	}
	return fmt.Sprintf("%.4g%ss", v, prefix)
}

func HumanizePercentage(s string) string {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	return fmt.Sprintf("%.2f%%", v*100)
}

func HumanizePercentageH(s string) string {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	return fmt.Sprintf("%.2f%%", v)
}

func HumanizeTimestamp(i interface{}) (string, error) {
	v, err := convertToFloat(i)
	if err != nil {
		return "", err
	}

	tm, err := floatToTime(v)
	switch {
	case errors.Is(err, errNaNOrInf):
		return fmt.Sprintf("%.4g", v), nil
	case err != nil:
		return "", err
	}

	return fmt.Sprint(tm), nil
}

// Add returns the sum of a and b.
func Add(a, b interface{}) (interface{}, error) {
	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)

	switch av.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Int() + bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Int() + int64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Int()) + bv.Float(), nil
		default:
			return nil, fmt.Errorf("add: unknown type for %q (%T)", bv, b)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int64(av.Uint()) + bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Uint() + bv.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Uint()) + bv.Float(), nil
		default:
			return nil, fmt.Errorf("add: unknown type for %q (%T)", bv, b)
		}
	case reflect.Float32, reflect.Float64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Float() + float64(bv.Int()), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Float() + float64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return av.Float() + bv.Float(), nil
		default:
			return nil, fmt.Errorf("add: unknown type for %q (%T)", bv, b)
		}
	default:
		return nil, fmt.Errorf("add: unknown type for %q (%T)", av, a)
	}
}

// Subtract returns the difference of b from a.
func Subtract(a, b interface{}) (interface{}, error) {
	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)

	switch av.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Int() - bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Int() - int64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Int()) - bv.Float(), nil
		default:
			return nil, fmt.Errorf("subtract: unknown type for %q (%T)", bv, b)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int64(av.Uint()) - bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Uint() - bv.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Uint()) - bv.Float(), nil
		default:
			return nil, fmt.Errorf("subtract: unknown type for %q (%T)", bv, b)
		}
	case reflect.Float32, reflect.Float64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Float() - float64(bv.Int()), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Float() - float64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return av.Float() - bv.Float(), nil
		default:
			return nil, fmt.Errorf("subtract: unknown type for %q (%T)", bv, b)
		}
	default:
		return nil, fmt.Errorf("subtract: unknown type for %q (%T)", av, a)
	}
}

// Multiply returns the product of a and b.
func Multiply(a, b interface{}) (interface{}, error) {
	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)

	switch av.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Int() * bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Int() * int64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Int()) * bv.Float(), nil
		default:
			return nil, fmt.Errorf("multiply: unknown type for %q (%T)", bv, b)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int64(av.Uint()) * bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Uint() * bv.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Uint()) * bv.Float(), nil
		default:
			return nil, fmt.Errorf("multiply: unknown type for %q (%T)", bv, b)
		}
	case reflect.Float32, reflect.Float64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Float() * float64(bv.Int()), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Float() * float64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return av.Float() * bv.Float(), nil
		default:
			return nil, fmt.Errorf("multiply: unknown type for %q (%T)", bv, b)
		}
	default:
		return nil, fmt.Errorf("multiply: unknown type for %q (%T)", av, a)
	}
}

// Divide returns the division of b from a.
func Divide(a, b interface{}) (interface{}, error) {
	av := reflect.ValueOf(a)
	bv := reflect.ValueOf(b)

	switch av.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Int() / bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Int() / int64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Int()) / bv.Float(), nil
		default:
			return nil, fmt.Errorf("divide: unknown type for %q (%T)", bv, b)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return int64(av.Uint()) / bv.Int(), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Uint() / bv.Uint(), nil
		case reflect.Float32, reflect.Float64:
			return float64(av.Uint()) / bv.Float(), nil
		default:
			return nil, fmt.Errorf("divide: unknown type for %q (%T)", bv, b)
		}
	case reflect.Float32, reflect.Float64:
		switch bv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return av.Float() / float64(bv.Int()), nil
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return av.Float() / float64(bv.Uint()), nil
		case reflect.Float32, reflect.Float64:
			return av.Float() / bv.Float(), nil
		default:
			return nil, fmt.Errorf("divide: unknown type for %q (%T)", bv, b)
		}
	default:
		return nil, fmt.Errorf("divide: unknown type for %q (%T)", av, a)
	}
}

func FormatDecimal(s string, n int) string {
	num, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}

	format := fmt.Sprintf("%%.%df", n)
	return fmt.Sprintf(format, num)
}

func First(v queryResult) (*sample, error) {
	if len(v) > 0 {
		return v[0], nil
	}
	return nil, errors.New("first() called on vector with no elements")
}

func Label(label string, s *sample) string {
	return s.Labels[label]
}

func Value(s *sample) float64 {
	return s.Value
}

func StrValue(s *sample) string {
	return s.Labels["__value__"]
}

func SafeHtml(text string) template.HTML {
	return template.HTML(text)
}

func Match(pattern, s string) (bool, error) {
	return regexp.MatchString(pattern, s)
}
func Title(s string) string {
	return strings.Title(s)
}

func ToUpper(s string) string {
	return strings.ToUpper(s)
}

func ToLower(s string) string {
	return strings.ToLower(s)
}

func GraphLink(expr string) string {
	return strutil.GraphLinkForExpression(expr)
}

func TableLink(expr string) string {
	return strutil.TableLinkForExpression(expr)
}

func SortByLabel(label string, v queryResult) queryResult {
	sorter := queryResultByLabelSorter{v[:], label}
	sort.Stable(sorter)
	return v
}

func StripPort(hostPort string) string {
	host, _, err := net.SplitHostPort(hostPort)
	if err != nil {
		return hostPort
	}
	return host
}

func StripDomain(hostPort string) string {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		host = hostPort
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return hostPort
	}
	host = strings.Split(host, ".")[0]
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	return host
}

func ToTime(i interface{}) (*time.Time, error) {
	v, err := convertToFloat(i)
	if err != nil {
		return nil, err
	}
	return floatToTime(v)
}

func PathPrefix(externalURL *url.URL) string {
	return externalURL.Path
}

func ExternalURL(externalURL *url.URL) string {
	return externalURL.String()
}

func ParseDuration(d string) (float64, error) {
	v, err := model.ParseDuration(d)
	if err != nil {
		return 0, err
	}
	return float64(time.Duration(v)) / float64(time.Second), nil
}

func Printf(format string, value interface{}) string {
	valType := reflect.TypeOf(value).Kind()

	switch valType {
	case reflect.String:
		// Try converting string to float
		if floatValue, err := strconv.ParseFloat(value.(string), 64); err == nil {
			return fmt.Sprintf(format, floatValue)
		}
		return fmt.Sprintf(format, value)
	case reflect.Float64, reflect.Float32:
		return fmt.Sprintf(format, value)
	default:
		// Handle other types as per requirement
		return fmt.Sprintf(format, value)
	}
}

func floatToTime(v float64) (*time.Time, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, errNaNOrInf
	}
	timestamp := v * 1e9
	if timestamp > math.MaxInt64 || timestamp < math.MinInt64 {
		return nil, fmt.Errorf("%v cannot be represented as a nanoseconds timestamp since it overflows int64", v)
	}
	t := model.TimeFromUnixNano(int64(timestamp)).Time().UTC()
	return &t, nil
}

func convertToFloat(i interface{}) (float64, error) {
	switch v := i.(type) {
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	case int:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("can't convert %T to float", v)
	}
}
