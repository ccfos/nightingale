// @Author: Ciusyan 5/17/24

package sqlbase

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/types"
)

func TestFormatMetricValues(t *testing.T) {
	tests := []struct {
		name string
		keys types.Keys
		rows []map[string]interface{}
		want []types.MetricValues
	}{
		{
			name: "cases1",
			keys: types.Keys{
				ValueKey:   "grade a_grade",
				LabelKey:   "id student_name",
				TimeKey:    "update_time",
				TimeFormat: "2006-01-02 15:04:05",
			},
			rows: []map[string]interface{}{
				{
					"id":           "10007",
					"grade":        20003,
					"student_name": "邵子韬",
					"a_grade":      69,
					"update_time":  "2024-05-14 10:00:00",
				},
				{
					"id":           "10007",
					"grade":        20003,
					"student_name": "邵子韬",
					"a_grade":      69,
					"update_time":  "2024-05-14 10:05:00",
				},
				{
					"id":           "10007",
					"grade":        20003,
					"student_name": "邵子韬",
					"a_grade":      69,
					"update_time":  "2024-05-14 10:10:00",
				},
				{
					"id":           "10008",
					"grade":        20004,
					"student_name": "Ciusyan",
					"a_grade":      100,
					"update_time":  "2024-05-14 12:00:00",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatMetricValues(tt.keys, tt.rows)
			for _, g := range got {
				t.Log(g)
			}
		})
	}
}

func TestParseFloat64Value(t *testing.T) {

	ptr := func(val float64) *float64 {
		return &val
	}

	tests := []struct {
		name    string
		input   interface{}
		want    float64
		wantErr bool
	}{
		{"float64", 1.23, 1.23, false},
		{"float32", float32(1.23), float64(float32(1.23)), false},
		{"int", 123, 123, false},
		{"int64", int64(123), 123, false},
		{"uint", uint(123), 123, false},
		{"uint64", uint64(123), 123, false},
		{"string", "1.23", 1.23, false},
		{"[]byte", []byte("1.23"), 1.23, false},
		{"json.Number", json.Number("1.23"), 1.23, false},
		{"interface", interface{}(1.23), 1.23, false},
		{"pointer", ptr(1.23), 1.23, false},
		{"invalid string", "abc", 0, true},
		{"invalid type", struct{}{}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFloat64Value(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFloat64Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseFloat64Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseTime(t *testing.T) {

	ptrTime := func(t time.Time) *time.Time {
		return &t
	}

	tests := []struct {
		name    string
		input   interface{}
		format  string
		want    time.Time
		wantErr bool
	}{
		{"RFC3339", "2024-05-14T12:34:56Z", "", time.Date(2024, 5, 14, 12, 34, 56, 0, time.UTC), false},
		{"RFC3339Nano", "2024-05-14T12:34:56.789Z", "", time.Date(2024, 5, 14, 12, 34, 56, 789000000, time.UTC), false},
		{"Unix timestamp int", int64(1715642135), "", time.Unix(1715642135, 0), false},
		{"Unix timestamp float64", 1715642135.0, "", time.Unix(int64(1715642135), 0), false},
		{"custom format", "14/05/2024", "02/01/2006", time.Date(2024, 5, 14, 0, 0, 0, 0, time.UTC), false},
		{"slice", []byte("2024-05-14T12:34:56Z"), "", time.Date(2024, 5, 14, 12, 34, 56, 0, time.UTC), false},
		{"interface", interface{}("2024-05-14T12:34:56Z"), "", time.Date(2024, 5, 14, 12, 34, 56, 0, time.UTC), false},
		{"pointer", ptrTime(time.Date(2024, 5, 14, 12, 34, 56, 0, time.UTC)), "", time.Date(2024, 5, 14, 12, 34, 56, 0, time.UTC), false},
		{"invalid format", "14-05-2024", "02/01/2006", time.Time{}, true},
		{"invalid type", struct{}{}, "", time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTime(tt.input, tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseTime() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !got.Equal(tt.want) {
				t.Errorf("ParseTime() = %v, want %v", got, tt.want)
			}
		})
	}
}
