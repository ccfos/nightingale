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
		{3, "k", "Ki", 10},
		{6, "M", "Mi", 20},
		{9, "G", "Gi", 30},
		{12, "T", "Ti", 40},
		{15, "P", "Pi", 50},
		{18, "E", "Ei", 60},
		{21, "Z", "Zi", 70},
		{24, "Y", "Yi", 80},
	}

	baseUtilMap = map[string]string{
		"bits":  "b",
		"bytes": "B",
	}
)

const (
	iecFactor = 1024.0
	siFactor  = 1000.0
	// siBaseIndex 是 siPrefixes 中空前缀（基础单位）的下标
	siBaseIndex = 5
)

// scaledUnit 描述前端 UnitPicker 里带固定档位的单位，语义与前端 binaryPrefix/SIPrefix 一致：
// 数值本身就是 offset 所指档位上的值，比如 mebibytesSec 的 160 即 160MiB/s，而不是 160 字节每秒。
type scaledUnit struct {
	factor float64
	symbol string
	offset int
}

func iecUnit(symbol string, offset int) scaledUnit {
	return scaledUnit{factor: iecFactor, symbol: symbol, offset: offset}
}

func siUnit(symbol string, offset int) scaledUnit {
	return scaledUnit{factor: siFactor, symbol: symbol, offset: siBaseIndex + offset}
}

var (
	iecPrefixes = []string{"", "Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "Zi", "Yi"}
	siPrefixes  = []string{"f", "p", "n", "µ", "m", "", "k", "M", "G", "T", "P", "E", "Z", "Y"}

	// scaledUnitMap 对应前端 UnitPicker 的 Data、Data rate 分组以及 Length 里可换算的长度单位
	scaledUnitMap = map[string]scaledUnit{
		// Data(IEC)
		"bitsIEC":    iecUnit("b", 0),
		"bits(IEC)":  iecUnit("b", 0),
		"bytesIEC":   iecUnit("B", 0),
		"bytes(IEC)": iecUnit("B", 0),
		"kibibytes":  iecUnit("B", 1),
		"mebibytes":  iecUnit("B", 2),
		"gibibytes":  iecUnit("B", 3),
		"tebibytes":  iecUnit("B", 4),
		"pebibytes":  iecUnit("B", 5),
		// Data(SI)
		"bitsSI":    siUnit("b", 0),
		"bits(SI)":  siUnit("b", 0),
		"bytesSI":   siUnit("B", 0),
		"bytes(SI)": siUnit("B", 0),
		"kilobytes": siUnit("B", 1),
		"megabytes": siUnit("B", 2),
		"gigabytes": siUnit("B", 3),
		"terabytes": siUnit("B", 4),
		"petabytes": siUnit("B", 5),
		// Data rate(IEC)
		"bitsSecIEC":   iecUnit("b/s", 0),
		"bytesSecIEC":  iecUnit("B/s", 0),
		"kibibitsSec":  iecUnit("b/s", 1),
		"kibibytesSec": iecUnit("B/s", 1),
		"mebibitsSec":  iecUnit("b/s", 2),
		"mebibytesSec": iecUnit("B/s", 2),
		"gibibitsSec":  iecUnit("b/s", 3),
		"gibibytesSec": iecUnit("B/s", 3),
		"tebibitsSec":  iecUnit("b/s", 4),
		"tebibytesSec": iecUnit("B/s", 4),
		"pebibitsSec":  iecUnit("b/s", 5),
		"pebibytesSec": iecUnit("B/s", 5),
		// Data rate(SI)
		"bitsSecSI":    siUnit("b/s", 0),
		"bytesSecSI":   siUnit("B/s", 0),
		"packetsSec":   siUnit("p/s", 0),
		"kilobitsSec":  siUnit("b/s", 1),
		"kilobytesSec": siUnit("B/s", 1),
		"megabitsSec":  siUnit("b/s", 2),
		"megabytesSec": siUnit("B/s", 2),
		"gigabitsSec":  siUnit("b/s", 3),
		"gigabytesSec": siUnit("B/s", 3),
		"terabitsSec":  siUnit("b/s", 4),
		"terabytesSec": siUnit("B/s", 4),
		"petabitsSec":  siUnit("b/s", 5),
		"petabytesSec": siUnit("B/s", 5),
		// Length
		"millimeter": siUnit("m", -1),
		"meter":      siUnit("m", 0),
		"kilometer":  siUnit("m", 1),
	}

	// siSymbolUnits 对应前端 UnitPicker 的 Throughput、Temperature 分组：
	// 数值按 SI 缩写后再跟上符号
	siSymbolUnits = map[string]string{
		"cps":    "c/s",
		"ops":    "ops/s",
		"reqps":  "req/s",
		"rps":    "rd/s",
		"wps":    "wr/s",
		"iops":   "io/s",
		"eps":    "evt/s",
		"mps":    "msg/s",
		"recps":  "rec/s",
		"rowsps": "rows/s",
		"cpm":    "c/m",
		"opm":    "ops/m",
		"reqpm":  "req/m",
		"rpm":    "rd/m",
		"wpm":    "wr/m",
		"epm":    "evts/m",
		"mpm":    "msgs/m",
		"recpm":  "rec/m",
		"rowspm": "rows/m",

		"celsius":    "°C",
		"fahrenheit": "°F",
		"kelvin":     "K",
	}

	// fixedSymbolUnits 对应前端 Length 分组里不参与换算的单位
	fixedSymbolUnits = map[string]string{
		"inch": "in",
		"foot": "ft",
		"mile": "mi",
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

	// Handle positive and negative infinity
	if math.IsInf(value, 1) {
		return FormattedValue{
			Value: 9999999999,
			Unit:  "",
			Text:  "+Inf",
			Stat:  9999999999,
		}
	}
	if math.IsInf(value, -1) {
		return FormattedValue{
			Value: -9999999999,
			Unit:  "",
			Text:  "-Inf",
			Stat:  -9999999999,
		}
	}

	switch unit {
	case "", "none":
		return formatNone(value, decimals)
	case "ns", "nanoseconds":
		return formatDuration(value, "ns", decimals)
	case "µs", "microseconds":
		return formatDuration(value, "µs", decimals)
	case "ms", "milliseconds":
		return formatDuration(value, "ms", decimals)
	case "s", "seconds":
		return formatDuration(value, "s", decimals)
	case "min", "h", "d", "w":
		return formatDuration(value, unit, decimals)
	case "humantimeSeconds":
		// UnitPicker 已不再提供，仅为兼容旧配置
		return formatDuration(value, "s", decimals)
	case "humantimeMilliseconds":
		return formatDuration(value, "ms", decimals)
	case "percent":
		return formatPercent(value, decimals, false)
	case "percentUnit":
		return formatPercent(value, decimals, true)
	case "default", "short", "sishort":
		// 前端 short 用的是 Intl 紧凑记数（1.5K），后端没有 i18n 语境，统一按 SI 缩写处理
		return formatBytes(value, FormatOptions{Type: "si", Decimals: decimals})
	case "dBm":
		return formatBytes(value, FormatOptions{Type: "si", Decimals: decimals, Postfix: "dBm"})
	case "datetimeSeconds", "datetimeMilliseconds":
		return formatDateTime(unit, value)
	}

	if su, ok := scaledUnitMap[unit]; ok {
		return formatScaled(value, su, decimals)
	}

	// 吞吐量、温度：数值先按 SI 缩写，再跟上符号，符号前的空格只体现在 Text 里
	if symbol, ok := siSymbolUnits[unit]; ok {
		formatted := formatBytes(value, FormatOptions{Type: "si", Decimals: decimals, Postfix: " " + symbol})
		formatted.Unit = strings.TrimSpace(formatted.Unit)
		return formatted
	}

	if symbol, ok := fixedSymbolUnits[unit]; ok {
		return formatSymbol(value, symbol, decimals)
	}

	// 未知单位（含用户自定义单位）与前端一样把单位直接拼在数值后面，
	// 这样前端将来新增单位时后端最多是不换算，而不会把单位整个丢掉
	return formatSymbol(value, unit, decimals)
}

// formatScaled 按 scaledUnit 描述的进制和起始档位换算，与前端 scaledUnits 的算法保持一致
func formatScaled(value float64, su scaledUnit, decimals int) FormattedValue {
	prefixes := siPrefixes
	if su.factor == iecFactor {
		prefixes = iecPrefixes
	}

	step := 0
	if value != 0 {
		abs := math.Abs(value)
		step = int(math.Floor(math.Log10(abs) / math.Log10(su.factor)))
		// 取对数的浮点误差会让 1e15 这类整幂次落到低一档（4.999...），这里纠正回来
		if math.Pow(su.factor, float64(step+1)) <= abs {
			step++
		} else if math.Pow(su.factor, float64(step)) > abs {
			step--
		}
	}

	symbol := prefixes[clamp(su.offset+step, 0, len(prefixes)-1)] + su.symbol
	// 档位到头后不再继续换算，多出来的数量级直接体现在数值上
	scaled := value / math.Pow(su.factor, float64(clamp(step, -su.offset, len(prefixes)-su.offset-1)))

	return FormattedValue{
		Value: roundFloat(scaled, decimals),
		Unit:  symbol,
		Text:  fmt.Sprintf("%.*f%s", decimals, scaled, symbol),
		Stat:  value,
	}
}

// formatSymbol 处理不做换算、直接在数值后拼接符号的单位
func formatSymbol(value float64, symbol string, decimals int) FormattedValue {
	return FormattedValue{
		Value: roundFloat(value, decimals),
		Unit:  symbol,
		Text:  fmt.Sprintf("%.*f %s", decimals, value, symbol),
		Stat:  value,
	}
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
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

	// 选择合适的单位，用绝对值选档，否则负值会跳过所有分支落到 ns 上被放大若干个数量级
	absValue := math.Abs(value)
	switch {
	case absValue >= YearVal:
		converted = value / YearVal
		targetUnit = "y"
	case absValue >= WeekVal:
		converted = value / WeekVal
		targetUnit = "w"
	case absValue >= DayVal:
		converted = value / DayVal
		targetUnit = "d"
	case absValue >= HourVal:
		converted = value / HourVal
		targetUnit = "h"
	case absValue >= MinuteVal:
		converted = value / MinuteVal
		targetUnit = "min"
	case absValue >= SecondVal:
		converted = value
		targetUnit = "s"
	case absValue >= MillisecondVal:
		converted = value / MillisecondVal
		targetUnit = "ms"
	case absValue >= MicrosecondVal:
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
