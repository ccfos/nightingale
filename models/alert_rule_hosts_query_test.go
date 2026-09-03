package models_test

import (
	"reflect"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
)

func TestGetHostsQuery_GroupIDs(t *testing.T) {
	const (
		sqlEqOnlyZero    = "target_busi_group.target_ident IS NULL"
		sqlEqMixed       = "(target_busi_group.target_ident IS NULL OR target_busi_group.group_id IN (?))"
		sqlEqOnlyReal    = "target_busi_group.group_id in (?)"
		sqlNeOnlyZero    = "target_busi_group.target_ident IS NOT NULL"
		sqlNeMixed       = "target_busi_group.target_ident IS NOT NULL AND NOT EXISTS (SELECT 1 FROM target_busi_group tbg WHERE tbg.target_ident = target.ident AND tbg.group_id IN (?))"
		sqlNeOnlyReal    = "NOT EXISTS (SELECT 1 FROM target_busi_group tbg WHERE tbg.target_ident = target.ident AND tbg.group_id IN (?))"
	)

	cases := []struct {
		name      string
		op        string
		values    []interface{}
		wantKey   string
		wantValue interface{}
	}{
		{
			name:      "== only ungrouped",
			op:        "==",
			values:    []interface{}{float64(0)},
			wantKey:   sqlEqOnlyZero,
			wantValue: nil,
		},
		{
			name:      "== only real groups",
			op:        "==",
			values:    []interface{}{float64(1), float64(2)},
			wantKey:   sqlEqOnlyReal,
			wantValue: []int64{1, 2},
		},
		{
			name:      "== mixed",
			op:        "==",
			values:    []interface{}{float64(0), float64(1), float64(2)},
			wantKey:   sqlEqMixed,
			wantValue: []int64{1, 2},
		},
		{
			name:      "!= only ungrouped",
			op:        "!=",
			values:    []interface{}{float64(0)},
			wantKey:   sqlNeOnlyZero,
			wantValue: nil,
		},
		{
			name:      "!= only real groups",
			op:        "!=",
			values:    []interface{}{float64(1), float64(2)},
			wantKey:   sqlNeOnlyReal,
			wantValue: []int64{1, 2},
		},
		{
			name:      "!= mixed",
			op:        "!=",
			values:    []interface{}{float64(0), float64(1), float64(2)},
			wantKey:   sqlNeMixed,
			wantValue: []int64{1, 2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := models.GetHostsQuery([]models.HostQuery{{
				Key:    "group_ids",
				Op:     tc.op,
				Values: tc.values,
			}})
			if len(out) != 1 {
				t.Fatalf("expected 1 query map, got %d", len(out))
			}
			if len(out[0]) != 1 {
				t.Fatalf("expected exactly one key in query map, got %d (map=%v)", len(out[0]), out[0])
			}
			got, ok := out[0][tc.wantKey]
			if !ok {
				t.Fatalf("expected key %q in map, got %v", tc.wantKey, out[0])
			}
			if !reflect.DeepEqual(got, tc.wantValue) {
				t.Fatalf("for key %q: expected value %#v, got %#v", tc.wantKey, tc.wantValue, got)
			}
		})
	}
}
