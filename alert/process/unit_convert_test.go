package process

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
			unit:     "bytes(ICE)",
			decimals: 2,
			value:    1024 * 1024,
			want:     FormattedValue{Value: 1, Unit: "MB", Text: "1.00MB", Stat: 1024 * 1024},
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
