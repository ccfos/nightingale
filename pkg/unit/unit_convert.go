package unit

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// FormattedValue 格式化后的值的结构
type FormattedValue struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
	Text  string  `json:"text"`
	Stat  float64 `json:"stat"`
}

// FormatOptions 格式化选项
type FormatOptions struct {
	Type     string // "si" 或 "iec"
	Base     string // "bits" 或 "bytes"
	Decimals int    // 小数位数
	Postfix  string // 后缀
}

// 时间相关常量
const (
	NanosecondVal  = 0.000000001
	MicrosecondVal = 0.000001
	MillisecondVal = 0.001
	SecondVal      = 1
	MinuteVal      = 60
	HourVal        = 3600
	DayVal         = 86400
	WeekVal        = 86400 * 7
	YearVal        = 86400 * 365
)

var (
	valueMap = []struct {
		Exp    int
		Si     string
		Iec    string
		IecExp int
	}{
		{0, "", "", 1},
		{3, "k", "K", 10},
		{6, "M", "M", 20},
		{9, "G", "G", 30},
		{12, "T", "T", 40},
		{15, "P", "P", 50},
		{18, "E", "E", 60},
		{21, "Z", "Z", 70},
		{24, "Y", "Y", 80},
	}

	baseUtilMap = map[string]string{
		"bits":  "b",
		"bytes": "B",
	}
)

// ValueFormatter 格式化入口函数
func ValueFormatter(unit string, decimals int, value float64) FormattedValue {
	if math.IsNaN(value) {
		return FormattedValue{
			Value: 0,
			Unit:  "",
			Text:  "NaN",
			Stat:  0,
		}
	}

	// 处理时间单位
	switch unit {
	case "ns", "µs", "ms", "s", "min", "h", "d", "w":
		return formatDuration(value, unit, decimals)
	case "percent":
		return formatPercent(value, decimals, false)
	case "percentUnit":
		return formatPercent(value, decimals, true)
	case "none":
		return formatNone(value, decimals)
	case "bytes(ICE)", "bits(ICE)", "bytes", "bits":
		opts := FormatOptions{
			Type:     "iec",
			Base:     strings.TrimSuffix(unit, "(ICE)"),
			Decimals: decimals,
		}
		return formatBytes(value, opts)
	case "default", "sishort", "bytes(SI)", "bits(SI)":
		opts := FormatOptions{
			Type:     "si",
			Base:     strings.TrimSuffix(unit, "(SI)"),
			Decimals: decimals,
		}
		return formatBytes(value, opts)
	case "datetimeSeconds", "datetimeMilliseconds":
		return formatDateTime(unit, value)
	default:
		return formatNone(value, decimals)
	}
}

// formatDuration 处理时间单位的转换
func formatDuration(originValue float64, unit string, decimals int) FormattedValue {
	var converted float64
	var targetUnit string
	value := originValue
	// 标准化到秒
	switch unit {
	case "ns":
		value *= NanosecondVal
	case "µs":
		value *= MicrosecondVal
	case "ms":
		value *= MillisecondVal
	case "min":
		value *= MinuteVal
	case "h":
		value *= HourVal
	case "d":
		value *= DayVal
	case "w":
		value *= WeekVal
	}

	// 选择合适的单位
	switch {
	case value >= YearVal:
		converted = value / YearVal
		targetUnit = "y"
	case value >= WeekVal:
		converted = value / WeekVal
		targetUnit = "w"
	case value >= DayVal:
		converted = value / DayVal
		targetUnit = "d"
	case value >= HourVal:
		converted = value / HourVal
		targetUnit = "h"
	case value >= MinuteVal:
		converted = value / MinuteVal
		targetUnit = "min"
	case value >= SecondVal:
		converted = value
		targetUnit = "s"
	case value >= MillisecondVal:
		converted = value / MillisecondVal
		targetUnit = "ms"
	case value >= MicrosecondVal:
		converted = value / MicrosecondVal
		targetUnit = "µs"
	default:
		converted = value / NanosecondVal
		targetUnit = "ns"
	}

	return FormattedValue{
		Value: roundFloat(converted, decimals),
		Unit:  targetUnit,
		Text:  fmt.Sprintf("%.*f %s", decimals, converted, targetUnit),
		Stat:  originValue,
	}
}

// formatBytes 处理字节相关的转换
func formatBytes(value float64, opts FormatOptions) FormattedValue {
	if value == 0 {
		baseUtil := baseUtilMap[opts.Base]
		return FormattedValue{
			Value: 0,
			Unit:  baseUtil + opts.Postfix,
			Text:  fmt.Sprintf("0%s%s", baseUtil, opts.Postfix),
			Stat:  0,
		}
	}

	baseUtil := baseUtilMap[opts.Base]
	threshold := 1000.0
	if opts.Type == "iec" {
		threshold = 1024.0
	}

	if math.Abs(value) < threshold {
		return FormattedValue{
			Value: roundFloat(value, opts.Decimals),
			Unit:  baseUtil + opts.Postfix,
			Text:  fmt.Sprintf("%.*f%s%s", opts.Decimals, value, baseUtil, opts.Postfix),
			Stat:  value,
		}
	}

	// 计算指数
	exp := int(math.Floor(math.Log10(math.Abs(value))/3.0)) * 3
	if exp > 24 {
		exp = 24
	}

	var unit string
	var divider float64

	// 查找对应的单位
	for _, v := range valueMap {
		if v.Exp == exp {
			if opts.Type == "iec" {
				unit = v.Iec
				divider = math.Pow(2, float64(v.IecExp))
			} else {
				unit = v.Si
				divider = math.Pow(10, float64(v.Exp))
			}
			break
		}
	}

	newValue := value / divider
	return FormattedValue{
		Value: roundFloat(newValue, opts.Decimals),
		Unit:  unit + baseUtil + opts.Postfix,
		Text:  fmt.Sprintf("%.*f%s%s%s", opts.Decimals, newValue, unit, baseUtil, opts.Postfix),
		Stat:  value,
	}
}

// formatPercent 处理百分比格式化
func formatPercent(value float64, decimals int, isUnit bool) FormattedValue {
	if isUnit {
		value = value * 100
	}
	return FormattedValue{
		Value: roundFloat(value, decimals),
		Unit:  "%",
		Text:  fmt.Sprintf("%.*f%%", decimals, value),
		Stat:  value,
	}
}

// formatNone 处理无单位格式化
func formatNone(value float64, decimals int) FormattedValue {
	return FormattedValue{
		Value: value,
		Unit:  "",
		Text:  fmt.Sprintf("%.*f", decimals, value),
		Stat:  value,
	}
}

// formatDateTime 处理时间戳格式化
func formatDateTime(uint string, value float64) FormattedValue {
	var t time.Time
	switch uint {
	case "datetimeSeconds":
		t = time.Unix(int64(value), 0)
	case "datetimeMilliseconds":
		t = time.Unix(0, int64(value)*int64(time.Millisecond))
	}

	text := t.Format("2006-01-02 15:04:05")
	return FormattedValue{
		Value: value,
		Unit:  "",
		Text:  text,
		Stat:  value,
	}
}

// roundFloat 四舍五入到指定小数位
func roundFloat(val float64, precision int) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}
