package tools

import (
	"reflect"
	"testing"
)

func TestResolveCreationTeamIDs(t *testing.T) {
	cases := []struct {
		name       string
		fromConfig []int64
		params     map[string]string
		want       []int64
	}{
		{
			name:       "config wins over params",
			fromConfig: []int64{7, 8},
			params:     map[string]string{"team_ids": "1,2,3"},
			want:       []int64{7, 8},
		},
		{
			name:       "fallback to params team_ids",
			fromConfig: nil,
			params:     map[string]string{"team_ids": "1,2,3"},
			want:       []int64{1, 2, 3},
		},
		{
			name:       "trims spaces",
			fromConfig: nil,
			params:     map[string]string{"team_ids": " 1 , 2 "},
			want:       []int64{1, 2},
		},
		{
			name:       "skips non-int and non-positive",
			fromConfig: nil,
			params:     map[string]string{"team_ids": "1,abc,0,-3,4"},
			want:       []int64{1, 4},
		},
		{
			name:       "no team_ids param",
			fromConfig: nil,
			params:     map[string]string{},
			want:       nil,
		},
		{
			name:       "empty team_ids param",
			fromConfig: nil,
			params:     map[string]string{"team_ids": ""},
			want:       nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveCreationTeamIDs(tc.fromConfig, tc.params)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("resolveCreationTeamIDs(%v, %v) = %v, want %v", tc.fromConfig, tc.params, got, tc.want)
			}
		})
	}
}
