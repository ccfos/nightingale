package tplx

import (
	"bytes"
	"html/template"
	"math"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

func TestHumanize_ScalesDecimalUnits(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "1234567", want: "1.23M"},
		{input: "-2500", want: "-2.50k"},
		{input: "0.000002", want: "2.00u"},
		{input: "0", want: "0.00"},
		{input: "NaN", want: "NaN"},
		{input: "+Inf", want: "+Inf"},
		{input: "unavailable", want: "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Humanize(tt.input)

			require.Equal(t, tt.want, got)
		})
	}
}

func TestRenderTemplate_HumanizesBytes(t *testing.T) {
	tpl := template.Must(template.New("bytes").Funcs(NewTemplateFuncMap()).Parse(`{{humanize1024 .}}`))
	tests := []struct {
		input string
		want  string
	}{
		{input: "1048576", want: "1Mi"},
		{input: "-2048", want: "-2ki"},
		{input: "0.5", want: "0.5"},
		{input: "0", want: "0"},
		{input: "unknown", want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var out bytes.Buffer

			err := tpl.Execute(&out, tt.input)

			require.NoError(t, err)
			require.Equal(t, tt.want, out.String())
		})
	}
}

func TestHumanizeDuration_FormatsSeconds(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "90061", want: "1d 1h 1m 1s"},
		{input: "3661", want: "1h 1m 1s"},
		{input: "61", want: "1m 1s"},
		{input: "-1.25", want: "-1.25s"},
		{input: "0.002", want: "2ms"},
		{input: "0", want: "0s"},
		{input: "NaN", want: "NaN"},
		{input: "unknown", want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := HumanizeDuration(tt.input)

			require.Equal(t, tt.want, got)
		})
	}
}

func TestHumanizeDurationInterface_ConvertsNumericInput(t *testing.T) {
	require.Equal(t, "1m 1s", HumanizeDurationInterface(int64(61)))
	require.Equal(t, "unknown", HumanizeDurationInterface("unknown"))
}

func TestRenderTemplate_FormatsPercentagesAndDecimals(t *testing.T) {
	tests := []struct {
		name string
		tpl  string
		want string
	}{
		{name: "fraction", tpl: `{{humanizePercentage "0.125"}}`, want: "12.50%"},
		{name: "percentage", tpl: `{{humanizePercentageH "12.5"}}`, want: "12.50%"},
		{name: "decimal", tpl: `{{formatDecimal "12.345" 2}}`, want: "12.35"},
		{name: "invalid fraction", tpl: `{{humanizePercentage "n/a"}}`, want: "n/a"},
		{name: "invalid percentage", tpl: `{{humanizePercentageH "n/a"}}`, want: "n/a"},
		{name: "invalid decimal", tpl: `{{formatDecimal "n/a" 2}}`, want: "n/a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tpl := template.Must(template.New(tt.name).Funcs(NewTemplateFuncMap()).Parse(tt.tpl))
			var out bytes.Buffer

			err := tpl.Execute(&out, nil)

			require.NoError(t, err)
			require.Equal(t, tt.want, out.String())
		})
	}
}

func TestPrintf_PreservesUnits(t *testing.T) {
	tests := []struct {
		name   string
		format string
		input  interface{}
		want   string
	}{
		{name: "numeric string", format: "%.1f", input: "12.34", want: "12.3"},
		{name: "percentage", format: "%.1f", input: "12.34%", want: "12.34%"},
		{name: "bytes", format: "%.1f", input: "100MB", want: "100MB"},
		{name: "negative", format: "%.1f", input: "-3.14", want: "-3.1"},
		{name: "text", format: "%s", input: "unknown", want: "unknown"},
		{name: "empty", format: "%s", input: "", want: ""},
		{name: "float", format: "%.1f", input: 12.34, want: "12.3"},
		{name: "integer", format: "%d", input: 12, want: "12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Printf(tt.format, tt.input)

			require.Equal(t, tt.want, got)
		})
	}
}

func TestToFloat64_ConvertsNumericAndStatusValues(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
		want  float64
	}{
		{name: "decimal", input: "1.25", want: 1.25},
		{name: "hex", input: "0x10", want: 16},
		{name: "true string", input: "true", want: 1},
		{name: "false string", input: "false", want: 0},
		{name: "up", input: "up", want: 1},
		{name: "down", input: "down", want: 0},
		{name: "true", input: true, want: 1},
		{name: "false", input: false, want: 0},
		{name: "int", input: int(2), want: 2},
		{name: "int8", input: int8(-2), want: -2},
		{name: "int16", input: int16(-3), want: -3},
		{name: "int32", input: int32(-4), want: -4},
		{name: "int64", input: int64(-5), want: -5},
		{name: "uint", input: uint(2), want: 2},
		{name: "uint8", input: uint8(3), want: 3},
		{name: "uint16", input: uint16(4), want: 4},
		{name: "uint32", input: uint32(5), want: 5},
		{name: "uint64", input: uint64(6), want: 6},
		{name: "float32", input: float32(1.5), want: 1.5},
		{name: "float64", input: 2.5, want: 2.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToFloat64(tt.input)

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestToFloat64_RejectsUnparseableInput(t *testing.T) {
	for _, input := range []interface{}{"unknown", struct{}{}, nil} {
		_, err := ToFloat64(input)

		require.Error(t, err)
	}
}

func TestArithmetic_PreservesNumericResultTypes(t *testing.T) {
	operations := []struct {
		name string
		fn   func(interface{}, interface{}) (interface{}, error)
	}{
		{name: "add", fn: Add},
		{name: "subtract", fn: Subtract},
		{name: "multiply", fn: Multiply},
		{name: "divide", fn: Divide},
	}
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want []interface{}
	}{
		{name: "signed signed", a: int64(8), b: int32(2), want: []interface{}{int64(10), int64(6), int64(16), int64(4)}},
		{name: "signed unsigned", a: int64(8), b: uint32(2), want: []interface{}{int64(10), int64(6), int64(16), int64(4)}},
		{name: "signed float", a: int64(8), b: 2.0, want: []interface{}{10.0, 6.0, 16.0, 4.0}},
		{name: "unsigned signed", a: uint64(8), b: int32(2), want: []interface{}{int64(10), int64(6), int64(16), int64(4)}},
		{
			name: "unsigned unsigned", a: uint64(8), b: uint32(2),
			want: []interface{}{uint64(10), uint64(6), uint64(16), uint64(4)},
		},
		{name: "unsigned float", a: uint64(8), b: 2.0, want: []interface{}{10.0, 6.0, 16.0, 4.0}},
		{name: "float signed", a: 8.0, b: int32(2), want: []interface{}{10.0, 6.0, 16.0, 4.0}},
		{name: "float unsigned", a: 8.0, b: uint32(2), want: []interface{}{10.0, 6.0, 16.0, 4.0}},
		{name: "float float", a: 8.0, b: 2.0, want: []interface{}{10.0, 6.0, 16.0, 4.0}},
	}
	for i, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := op.fn(tt.a, tt.b)

					require.NoError(t, err)
					require.Equal(t, tt.want[i], got)
				})
			}
		})
	}
}

func TestArithmetic_RejectsUnsupportedOperands(t *testing.T) {
	for _, fn := range []func(interface{}, interface{}) (interface{}, error){Add, Subtract, Multiply, Divide} {
		for _, input := range []interface{}{int64(8), uint64(8), 8.0, "8"} {
			got, err := fn(input, "2")

			require.Error(t, err)
			require.Nil(t, got)
		}
	}
}

func TestToTime_ConvertsFixedUnixTimestamp(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{name: "float", input: 1700000000.0},
		{name: "string", input: "1700000000"},
		{name: "int", input: int(1700000000)},
		{name: "uint", input: uint(1700000000)},
		{name: "int64", input: int64(1700000000)},
		{name: "uint64", input: uint64(1700000000)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ToTime(tt.input)

			require.NoError(t, err)
			require.Equal(t, time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC), *got)
		})
	}
}

func TestToTime_RejectsInvalidTimestamps(t *testing.T) {
	for _, input := range []interface{}{math.NaN(), math.Inf(1), 1e20, "bad", true} {
		got, err := ToTime(input)

		require.Error(t, err)
		require.Nil(t, got)
	}
}

func TestHumanizeTimestamp_FormatsUTC(t *testing.T) {
	got, err := HumanizeTimestamp("1700000000")

	require.NoError(t, err)
	require.Equal(t, "2023-11-14 22:13:20 +0000 UTC", got)
}

func TestParseDuration_UsesPrometheusUnits(t *testing.T) {
	got, err := ParseDuration("1h30m")

	require.NoError(t, err)
	require.Equal(t, 5400.0, got)

	_, err = ParseDuration("not a duration")

	require.Error(t, err)
}

func TestStripPort_HandlesIPv6AndBareHosts(t *testing.T) {
	require.Equal(t, "2001:db8::1", StripPort("[2001:db8::1]:9090"))
	require.Equal(t, "node.example.com", StripPort("node.example.com:9090"))
	require.Equal(t, "node.example.com", StripPort("node.example.com"))
}

func TestStripDomain_PreservesAddressesAndPorts(t *testing.T) {
	require.Equal(t, "node:9090", StripDomain("node.example.com:9090"))
	require.Equal(t, "node", StripDomain("node.example.com"))
	require.Equal(t, "127.0.0.1:9090", StripDomain("127.0.0.1:9090"))
	require.Equal(t, "[2001:db8::1]:9090", StripDomain("[2001:db8::1]:9090"))
}

func TestSortByLabel_PreservesOrderOfEqualLabels(t *testing.T) {
	input := QueryResult{
		{Labels: map[string]string{"host": "b"}, Value: 1},
		{Labels: map[string]string{"host": "a"}, Value: 2},
		{Labels: map[string]string{"host": "a"}, Value: 3},
	}

	got := SortByLabel("host", input)
	first, err := First(got)

	require.NoError(t, err)
	require.Equal(t, "a", Label("host", first))
	require.Equal(t, 2.0, Value(first))
	require.Equal(t, 3.0, Value(got[1]))
	require.Equal(t, 1.0, Value(got[2]))
}

func TestFirst_RejectsEmptyResults(t *testing.T) {
	got, err := First(nil)

	require.ErrorContains(t, err, "no elements")
	require.Nil(t, got)
}

func TestConvertToQueryResult_SkipsNaNVectorSamples(t *testing.T) {
	input := model.Vector{
		{Metric: model.Metric{"host": "node", "__value__": "up"}, Value: 2.5},
		{Metric: model.Metric{"host": "missing"}, Value: model.SampleValue(math.NaN())},
	}

	got := ConvertToQueryResult(input)

	require.Len(t, got, 1)
	require.Equal(t, map[string]string{"host": "node", "__value__": "up"}, got[0].Labels)
	require.Equal(t, 2.5, Value(got[0]))
	require.Equal(t, "up", StrValue(got[0]))
}

func TestConvertToQueryResult_UsesLastMatrixSample(t *testing.T) {
	input := model.Matrix{
		{Metric: model.Metric{"host": "node"}, Values: []model.SamplePair{{Value: 1}, {Value: 3}}},
		{Metric: model.Metric{"host": "empty"}},
		{Metric: model.Metric{"host": "nan"}, Values: []model.SamplePair{{Value: model.SampleValue(math.NaN())}}},
	}

	got := ConvertToQueryResult(input)

	require.Equal(t, QueryResult{{Labels: map[string]string{"host": "node"}, Value: 3}}, got)
}

func TestConvertToQueryResult_ConvertsScalar(t *testing.T) {
	got := ConvertToQueryResult(&model.Scalar{Value: 4})

	require.Equal(t, QueryResult{{Labels: map[string]string{}, Value: 4}}, got)
}

func TestConvertToQueryResult_RejectsNaNScalar(t *testing.T) {
	got := ConvertToQueryResult(&model.Scalar{Value: model.SampleValue(math.NaN())})

	require.Nil(t, got)
}

func TestConvertToQueryResult_RejectsUnsupportedValues(t *testing.T) {
	require.Nil(t, ConvertToQueryResult(&model.String{Value: "text"}))
	require.Nil(t, ConvertToQueryResult(nil))
}

func TestQuery_ForwardsDatasourceAndExpression(t *testing.T) {
	previous := queryFunc
	t.Cleanup(func() { queryFunc = previous })
	value := &model.Scalar{Value: 42}
	RegisterQueryFunc(func(id int64, expression string) model.Value {
		require.Equal(t, int64(7), id)
		require.Equal(t, "up", expression)
		return value
	})

	got := Query(7, "up")

	require.Same(t, value, got)
}

func TestTagsMapToStr_SortsLabels(t *testing.T) {
	got := TagsMapToStr(map[string]string{"zone": "west", "host": "node"})

	require.Equal(t, "host=node,zone=west", got)
}

func TestMapDifference_RemovesKeysRegardlessOfValues(t *testing.T) {
	got, err := MapDifference(map[string]string{"host": "a", "zone": "west"}, map[string]string{"host": "b"})

	require.NoError(t, err)
	require.Equal(t, map[string]string{"zone": "west"}, got)
}

func TestReplaceTemplate_EscapesOnlyHTML(t *testing.T) {
	data := map[string]string{"Name": "<node>&"}

	html := ReplaceTemplateUseHtml("html", `host={{.Name}}`, data)
	text := ReplaceTemplateUseText("text", `host={{.Name}}`, data)

	require.Equal(t, "host=&lt;node&gt;&amp;", html)
	require.Equal(t, "host=<node>&", text)
}

func TestReplaceTemplate_PreservesInputOnErrors(t *testing.T) {
	for _, fn := range []func(string, string, any) string{ReplaceTemplateUseHtml, ReplaceTemplateUseText} {
		for _, input := range []string{`{{`, `{{.Missing}}`} {
			got := fn("invalid", input, struct{}{})

			require.Equal(t, input, got)
		}
	}
}

func TestNewTemplateFuncMap_IsolatesCallerChanges(t *testing.T) {
	functions := NewTemplateFuncMap()
	delete(functions, "humanize")
	functions["testOnly"] = func() string { return "test" }

	fresh := NewTemplateFuncMap()

	require.Contains(t, fresh, "humanize")
	require.NotContains(t, fresh, "testOnly")
}
