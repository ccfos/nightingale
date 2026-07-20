package unit

import (
	"math"
	"testing"
)

func TestValueFormatter(t *testing.T) {
	tests := []struct {
		name     string
		unit     string
		decimals int
		value    float64
		want     FormattedValue
	}{
		// 字节测试
		{
			name:     "IEC字节测试",
			unit:     "bytes(IEC)",
			decimals: 2,
			value:    1024 * 1024,
			want:     FormattedValue{Value: 1, Unit: "MiB", Text: "1.00MiB", Stat: 1024 * 1024},
		},
		{
			name:     "SI字节测试",
			unit:     "bytes(SI)",
			decimals: 2,
			value:    1000 * 1000,
			want:     FormattedValue{Value: 1, Unit: "MB", Text: "1.00MB", Stat: 1000 * 1000},
		},
		// 时间单位测试
		{
			name:     "毫秒转秒",
			unit:     "ms",
			decimals: 2,
			value:    1500,
			want: FormattedValue{
				Value: 1.50,
				Unit:  "s",
				Text:  "1.50 s",
				Stat:  1500,
			},
		},
		{
			name:     "秒转分钟",
			unit:     "s",
			decimals: 1,
			value:    150,
			want: FormattedValue{
				Value: 2.5,
				Unit:  "min",
				Text:  "2.5 min",
				Stat:  150,
			},
		},
		// 百分比测试
		{
			name:     "百分比",
			unit:     "percent",
			decimals: 2,
			value:    0.9555,
			want: FormattedValue{
				Value: 0.96,
				Unit:  "%",
				Text:  "0.96%",
				Stat:  0.9555,
			},
		},
		{
			name:     "百分比单位",
			unit:     "percentUnit",
			decimals: 1,
			value:    0.95,
			want: FormattedValue{
				Value: 95.0,
				Unit:  "%",
				Text:  "95.0%",
				Stat:  95.0,
			},
		},
		// SI格式测试
		{
			name:     "SI格式",
			unit:     "sishort",
			decimals: 2,
			value:    1500,
			want: FormattedValue{
				Value: 1.50,
				Unit:  "k",
				Text:  "1.50k",
				Stat:  1500,
			},
		},
		// 时间戳测试
		{
			name:     "时间戳 s",
			unit:     "datetimeSeconds",
			decimals: 0,
			value:    1683518400,
			want: FormattedValue{
				Value: 1683518400,
				Unit:  "",
				Text:  "2023-05-08 12:00:00",
				Stat:  1683518400,
			},
		},
		{
			name:     "时间戳 ms",
			unit:     "datetimeMilliseconds",
			decimals: 0,
			value:    1683518400000,
			want: FormattedValue{
				Value: 1683518400000,
				Unit:  "",
				Text:  "2023-05-08 12:00:00",
				Stat:  1683518400000,
			},
		},
		// 补充时间单位测试
		{
			name:     "纳秒测试",
			unit:     "ns",
			decimals: 2,
			value:    1500,
			want: FormattedValue{
				Value: 1.50,
				Unit:  "µs",
				Text:  "1.50 µs",
				Stat:  1500,
			},
		},
		{
			name:     "微秒测试",
			unit:     "µs",
			decimals: 2,
			value:    1500,
			want: FormattedValue{
				Value: 1.50,
				Unit:  "ms",
				Text:  "1.50 ms",
				Stat:  1500,
			},
		},
		{
			name:     "小时测试",
			unit:     "h",
			decimals: 1,
			value:    2.5,
			want: FormattedValue{
				Value: 2.5,
				Unit:  "h",
				Text:  "2.5 h",
				Stat:  2.5,
			},
		},
		{
			name:     "天数测试",
			unit:     "d",
			decimals: 1,
			value:    1.5,
			want: FormattedValue{
				Value: 1.5,
				Unit:  "d",
				Text:  "1.5 d",
				Stat:  1.5,
			},
		},
		{
			name:     "周数测试",
			unit:     "w",
			decimals: 1,
			value:    1.5,
			want: FormattedValue{
				Value: 1.5,
				Unit:  "w",
				Text:  "1.5 w",
				Stat:  1.5,
			},
		},
		// 补充字节速率测试
		{
			name:     "IEC字节每秒",
			unit:     "bytesSecIEC",
			decimals: 2,
			value:    1024 * 1024,
			want: FormattedValue{
				Value: 1,
				Unit:  "MiB/s",
				Text:  "1.00MiB/s",
				Stat:  1024 * 1024,
			},
		},
		{
			name:     "IEC比特每秒",
			unit:     "bitsSecIEC",
			decimals: 2,
			value:    1024 * 1024,
			want: FormattedValue{
				Value: 1,
				Unit:  "Mib/s",
				Text:  "1.00Mib/s",
				Stat:  1024 * 1024,
			},
		},
		{
			name:     "SI字节每秒",
			unit:     "bytesSecSI",
			decimals: 2,
			value:    1000 * 1000,
			want: FormattedValue{
				Value: 1,
				Unit:  "MB/s",
				Text:  "1.00MB/s",
				Stat:  1000 * 1000,
			},
		},
		{
			name:     "SI比特每秒",
			unit:     "bitsSecSI",
			decimals: 2,
			value:    1000 * 1000,
			want: FormattedValue{
				Value: 1,
				Unit:  "Mb/s",
				Text:  "1.00Mb/s",
				Stat:  1000 * 1000,
			},
		},
		// none 类型测试
		{
			name:     "无单位测试",
			unit:     "none",
			decimals: 2,
			value:    1234.5678,
			want: FormattedValue{
				Value: 1234.5678,
				Unit:  "",
				Text:  "1234.57",
				Stat:  1234.5678,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValueFormatter(tt.unit, tt.decimals, tt.value)
			if !compareFormattedValues(got, tt.want) {
				t.Errorf("ValueFormatter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		unit     string
		decimals int
		value    float64
		wantNil  bool
	}{
		{
			name:     "NaN值",
			unit:     "bytes",
			decimals: 2,
			value:    math.NaN(),
			wantNil:  false,
		},
		{
			name:     "零值",
			unit:     "bytes",
			decimals: 2,
			value:    0,
			wantNil:  false,
		},
		{
			name:     "极小值",
			unit:     "bytes",
			decimals: 2,
			value:    0.0000001,
			wantNil:  false,
		},
		{
			name:     "极大值",
			unit:     "bytes",
			decimals: 2,
			value:    1e30,
			wantNil:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValueFormatter(tt.unit, tt.decimals, tt.value)
			if (got == FormattedValue{}) == !tt.wantNil {
				t.Errorf("ValueFormatter() got = %v, wantNil = %v", got, tt.wantNil)
			}
		})
	}
}

// TestFrontendUnits 覆盖前端 UnitPicker 里可选的每一个单位。
// 期望值按前端 valueFormatter 的换算结果编写，文本沿用后端固定小数位的写法。
func TestFrontendUnits(t *testing.T) {
	tests := []struct {
		unit  string
		value float64
		want  string
	}{
		// Misc
		{"none", 1234.5678, "1234.57"},
		{"short", 1500, "1.50k"},
		{"sishort", 1500, "1.50k"},
		{"count", 1500, "1500.00 count"},

		// Data(SI)
		{"bitsSI", 1500, "1.50kb"},
		{"bytesSI", 1500, "1.50kB"},
		{"kilobytes", 1500, "1.50MB"},
		{"megabytes", 1500, "1.50GB"},
		{"gigabytes", 1500, "1.50TB"},
		{"terabytes", 1500, "1.50PB"},
		{"petabytes", 1500, "1.50EB"},

		// Data(IEC)
		{"bitsIEC", 1500, "1.46Kib"},
		{"bytesIEC", 1500, "1.46KiB"},
		{"kibibytes", 1500, "1.46MiB"},
		{"mebibytes", 1500, "1.46GiB"},
		{"gibibytes", 1500, "1.46TiB"},
		{"tebibytes", 1500, "1.46PiB"},
		{"pebibytes", 1500, "1.46EiB"},

		// Data rate(SI)
		{"packetsSec", 1500, "1.50kp/s"},
		{"bitsSecSI", 1500, "1.50kb/s"},
		{"bytesSecSI", 1500, "1.50kB/s"},
		{"kilobitsSec", 1500, "1.50Mb/s"},
		{"kilobytesSec", 1500, "1.50MB/s"},
		{"megabitsSec", 1500, "1.50Gb/s"},
		{"megabytesSec", 1500, "1.50GB/s"},
		{"gigabitsSec", 1500, "1.50Tb/s"},
		{"gigabytesSec", 1500, "1.50TB/s"},
		{"terabitsSec", 1500, "1.50Pb/s"},
		{"terabytesSec", 1500, "1.50PB/s"},
		{"petabitsSec", 1500, "1.50Eb/s"},
		{"petabytesSec", 1500, "1.50EB/s"},

		// Data rate(IEC)
		{"bitsSecIEC", 1500, "1.46Kib/s"},
		{"bytesSecIEC", 1500, "1.46KiB/s"},
		{"kibibitsSec", 1500, "1.46Mib/s"},
		{"kibibytesSec", 1500, "1.46MiB/s"},
		{"mebibitsSec", 1500, "1.46Gib/s"},
		{"mebibytesSec", 1500, "1.46GiB/s"},
		{"gibibitsSec", 1500, "1.46Tib/s"},
		{"gibibytesSec", 1500, "1.46TiB/s"},
		{"tebibitsSec", 1500, "1.46Pib/s"},
		{"tebibytesSec", 1500, "1.46PiB/s"},
		{"pebibitsSec", 1500, "1.46Eib/s"},
		{"pebibytesSec", 1500, "1.46EiB/s"},

		// Throughput
		{"cps", 1500, "1.50k c/s"},
		{"ops", 1500, "1.50k ops/s"},
		{"reqps", 1500, "1.50k req/s"},
		{"rps", 1500, "1.50k rd/s"},
		{"wps", 1500, "1.50k wr/s"},
		{"iops", 1500, "1.50k io/s"},
		{"eps", 1500, "1.50k evt/s"},
		{"mps", 1500, "1.50k msg/s"},
		{"recps", 1500, "1.50k rec/s"},
		{"rowsps", 1500, "1.50k rows/s"},
		{"cpm", 1500, "1.50k c/m"},
		{"opm", 1500, "1.50k ops/m"},
		{"reqpm", 1500, "1.50k req/m"},
		{"rpm", 1500, "1.50k rd/m"},
		{"wpm", 1500, "1.50k wr/m"},
		{"epm", 1500, "1.50k evts/m"},
		{"mpm", 1500, "1.50k msgs/m"},
		{"recpm", 1500, "1.50k rec/m"},
		{"rowspm", 1500, "1.50k rows/m"},

		// Energy
		{"dBm", 160, "160.00dBm"},

		// Percent
		{"percent", 95.5, "95.50%"},
		{"percentUnit", 0.955, "95.50%"},

		// Time
		{"seconds", 150, "2.50 min"},
		{"milliseconds", 1500, "1.50 s"},
		{"microseconds", 1500, "1.50 ms"},
		{"nanoseconds", 1500, "1.50 µs"},
		// UnitPicker 已不再提供，仅为兼容旧配置
		{"humantimeSeconds", 150, "2.50 min"},
		{"humantimeMilliseconds", 1500, "1.50 s"},

		// Temperature
		{"celsius", 25, "25.00 °C"},
		{"fahrenheit", 77, "77.00 °F"},
		{"kelvin", 298, "298.00 K"},

		// Length
		{"millimeter", 1500, "1.50m"},
		{"meter", 1500, "1.50km"},
		{"kilometer", 1500, "1.50Mm"},
		{"inch", 12, "12.00 in"},
		{"foot", 3, "3.00 ft"},
		{"mile", 2, "2.00 mi"},
	}

	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			if got := ValueFormatter(tt.unit, 2, tt.value); got.Text != tt.want {
				t.Errorf("ValueFormatter(%q, 2, %v).Text = %q, want %q", tt.unit, tt.value, got.Text, tt.want)
			}
		})
	}
}

// TestNegativeDuration 守住负持续时间的选档：按绝对值挑单位，符号只体现在数值上
func TestNegativeDuration(t *testing.T) {
	tests := []struct {
		unit  string
		value float64
		want  string
	}{
		{"humantimeSeconds", -150, "-2.50 min"},
		{"humantimeMilliseconds", -1500, "-1.50 s"},
		{"milliseconds", -1500, "-1.50 s"},
		{"seconds", -0.0015, "-1.50 ms"},
	}

	for _, tt := range tests {
		t.Run(tt.unit, func(t *testing.T) {
			if got := ValueFormatter(tt.unit, 2, tt.value); got.Text != tt.want {
				t.Errorf("ValueFormatter(%q, 2, %v).Text = %q, want %q", tt.unit, tt.value, got.Text, tt.want)
			}
		})
	}
}

func TestScaledUnitBoundary(t *testing.T) {
	tests := []struct {
		name  string
		unit  string
		value float64
		want  FormattedValue
	}{
		{
			// 用户配置 160 MiB/s 的阈值，事件里就该看到 160.00MiB/s 而不是裸数字
			name:  "阈值原样展示",
			unit:  "mebibytesSec",
			value: 160,
			want:  FormattedValue{Value: 160, Unit: "MiB/s", Text: "160.00MiB/s", Stat: 160},
		},
		{
			name:  "零值",
			unit:  "mebibytesSec",
			value: 0,
			want:  FormattedValue{Value: 0, Unit: "MiB/s", Text: "0.00MiB/s", Stat: 0},
		},
		{
			name:  "负值",
			unit:  "mebibytesSec",
			value: -1500,
			want:  FormattedValue{Value: -1.46, Unit: "GiB/s", Text: "-1.46GiB/s", Stat: -1500},
		},
		{
			name:  "向下跨档",
			unit:  "mebibytesSec",
			value: 0.0005,
			want:  FormattedValue{Value: 524.29, Unit: "B/s", Text: "524.29B/s", Stat: 0.0005},
		},
		{
			// 整幂次容易被对数的浮点误差算低一档，这里守住 1e15 字节 = 1PB
			name:  "SI 整幂次",
			unit:  "bytesSI",
			value: 1e15,
			want:  FormattedValue{Value: 1, Unit: "PB", Text: "1.00PB", Stat: 1e15},
		},
		{
			name:  "档位到顶后不再换算",
			unit:  "pebibytesSec",
			value: 1e15,
			want:  FormattedValue{Value: 931322.57, Unit: "YiB/s", Text: "931322.57YiB/s", Stat: 1e15},
		},
		{
			name:  "未知单位保留单位文案",
			unit:  "myUnit",
			value: 1500,
			want:  FormattedValue{Value: 1500, Unit: "myUnit", Text: "1500.00 myUnit", Stat: 1500},
		},
		{
			name:  "空单位",
			unit:  "",
			value: 1500,
			want:  FormattedValue{Value: 1500, Unit: "", Text: "1500.00", Stat: 1500},
		},
		{
			name:  "NaN",
			unit:  "mebibytesSec",
			value: math.NaN(),
			want:  FormattedValue{Value: 0, Unit: "", Text: "NaN", Stat: 0},
		},
		{
			name:  "正无穷",
			unit:  "mebibytesSec",
			value: math.Inf(1),
			want:  FormattedValue{Value: 9999999999, Unit: "", Text: "+Inf", Stat: 9999999999},
		},
		{
			name:  "负无穷",
			unit:  "mebibytesSec",
			value: math.Inf(-1),
			want:  FormattedValue{Value: -9999999999, Unit: "", Text: "-Inf", Stat: -9999999999},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValueFormatter(tt.unit, 2, tt.value); !compareFormattedValues(got, tt.want) {
				t.Errorf("ValueFormatter() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// compareFormattedValues 比较两个FormattedValue是否相等
func compareFormattedValues(a, b FormattedValue) bool {
	const epsilon = 0.0001
	if math.Abs(a.Value-b.Value) > epsilon {
		return false
	}
	if math.Abs(a.Stat-b.Stat) > epsilon {
		return false
	}
	if a.Unit != b.Unit {
		return false
	}
	if a.Text != b.Text {
		return false
	}
	return true
}
