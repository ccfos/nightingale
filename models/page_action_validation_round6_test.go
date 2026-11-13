package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizePageActionSchema_RejectsInvalidPropertyDeclarations(t *testing.T) {
	cases := []struct {
		name   string
		schema string
		want   string
	}{
		{name: "malformed", schema: "{", want: "invalid page action schema"},
		{name: "null", schema: "null", want: "schema must be an object"},
		{name: "wrong root type", schema: `{"type":"array"}`, want: "schema type must be object"},
		{name: "wrong properties type", schema: `{"properties":[]}`, want: "properties must be an object"},
		{name: "scalar property", schema: `{"properties":{"x":1}}`, want: `property "x" must be an object`},
		{
			name: "unknown property type", schema: `{"properties":{"x":{"type":"date"}}}`,
			want: `property "x" has unsupported type date`,
		},
		{
			name: "numeric property type", schema: `{"properties":{"x":{"type":1}}}`,
			want: `property "x" has unsupported type 1`,
		},
		{
			name: "nontext description", schema: `{"properties":{"x":{"description":1}}}`,
			want: `property "x" description must be a string`,
		},
		{
			name: "empty enum", schema: `{"properties":{"x":{"enum":[]}}}`,
			want: `property "x" enum must be a non-empty array`,
		},
		{
			name: "object enum", schema: `{"properties":{"x":{"enum":[{}]}}}`,
			want: `property "x" enum values must be scalars`,
		},
		{
			name: "invalid nested properties", schema: `{"properties":{"x":{"properties":1}}}`,
			want: `property "x" properties must be an object`,
		},
		{
			name: "invalid nested child", schema: `{"properties":{"x":{"properties":{"y":1}}}}`,
			want: `property "x.y" must be an object`,
		},
		{
			name: "invalid array item", schema: `{"properties":{"x":{"items":1}}}`,
			want: `property "x[]" must be an object`,
		},
		{
			name: "undeclared nested requirement", schema: `{"properties":{"x":{"required":["missing"]}}}`,
			want: `property x requires "missing", which is not in properties`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			clean, err := normalizePageActionSchema(json.RawMessage(tc.schema))

			require.ErrorContains(t, err, tc.want)
			require.Nil(t, clean)
		})
	}
}

func TestNormalizePageActionSchema_PreservesScalarEnums(t *testing.T) {
	input := json.RawMessage(`{"properties":{"choice":{"enum":["cpu",1,true]}}}`)

	clean, err := normalizePageActionSchema(input)

	require.NoError(t, err)
	require.JSONEq(t, `{"type":"object","properties":{"choice":{"enum":["cpu",1,true]}}}`, string(clean))
}
