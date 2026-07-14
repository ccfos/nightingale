package tools

import (
	"reflect"
	"testing"
)

func TestResolveCreationPrivate(t *testing.T) {
	cases := []struct {
		name   string
		args   map[string]interface{}
		params map[string]string
		want   int
		wantOK bool
	}{
		// args (tool argument, model-provided from a text reply) wins over params.
		{"arg public wins over param", map[string]interface{}{"private": float64(0)}, map[string]string{"private": "1"}, 0, true},
		{"arg team-scoped", map[string]interface{}{"private": float64(1)}, nil, 1, true},
		{"arg as string", map[string]interface{}{"private": "0"}, nil, 0, true},
		// fall back to params (form submission) when arg absent/blank.
		{"param public", nil, map[string]string{"private": "0"}, 0, true},
		{"param team-scoped", nil, map[string]string{"private": "1"}, 1, true},
		{"param trims spaces", nil, map[string]string{"private": " 1 "}, 1, true},
		{"blank arg falls back to param", map[string]interface{}{"private": ""}, map[string]string{"private": "1"}, 1, true},
		// neither channel → not provided.
		{"missing", nil, map[string]string{}, 0, false},
		{"empty param", nil, map[string]string{"private": ""}, 0, false},
		{"non-int param", nil, map[string]string{"private": "abc"}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveCreationPrivate(tc.args, tc.params)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("resolveCreationPrivate(%v,%v) = (%d,%v), want (%d,%v)", tc.args, tc.params, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

func TestArgInt64Slice(t *testing.T) {
	cases := []struct {
		name string
		args map[string]interface{}
		want []int64
	}{
		{"json number array", map[string]interface{}{"team_ids": []interface{}{float64(1), float64(2)}}, []int64{1, 2}},
		{"quoted numbers in array", map[string]interface{}{"team_ids": []interface{}{"3", "4"}}, []int64{3, 4}},
		{"comma-separated string", map[string]interface{}{"team_ids": "5, 6"}, []int64{5, 6}},
		{"drops non-positive and junk", map[string]interface{}{"team_ids": []interface{}{float64(0), float64(-1), "x", float64(7)}}, []int64{7}},
		{"absent", map[string]interface{}{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := argInt64Slice(tc.args, "team_ids")
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argInt64Slice(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestInt64SliceSubset(t *testing.T) {
	cases := []struct {
		name  string
		super []int64
		sub   []int64
		want  bool
	}{
		{"empty sub is subset", []int64{1, 2}, nil, true},
		{"proper subset", []int64{1, 2, 3}, []int64{1, 3}, true},
		{"equal set", []int64{1, 2}, []int64{2, 1}, true},
		{"not subset", []int64{1, 2}, []int64{2, 3}, false},
		{"empty super non-empty sub", nil, []int64{1}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := int64SliceSubset(tc.super, tc.sub); got != tc.want {
				t.Fatalf("int64SliceSubset(%v,%v) = %v, want %v", tc.super, tc.sub, got, tc.want)
			}
		})
	}
}
