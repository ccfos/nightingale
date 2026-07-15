package router

import "testing"

func TestGroupsSubset(t *testing.T) {
	cases := []struct {
		name       string
		super, sub []int64
		want       bool
	}{
		{"empty sub is subset", []int64{1}, nil, true},
		{"single member", []int64{1, 2}, []int64{1}, true},
		{"all members", []int64{1, 2, 3}, []int64{1, 3}, true},
		{"one outsider rejected", []int64{1}, []int64{1, 999}, false},
		{"non-member of empty super", nil, []int64{1}, false},
	}
	for _, tc := range cases {
		if got := groupsSubset(tc.super, tc.sub); got != tc.want {
			t.Errorf("%s: groupsSubset(%v,%v)=%v want %v", tc.name, tc.super, tc.sub, got, tc.want)
		}
	}
}

func TestAddedGroups(t *testing.T) {
	if got := addedGroups([]int64{1, 2}, []int64{1, 2, 3}); len(got) != 1 || got[0] != 3 {
		t.Fatalf("addedGroups: got %v want [3]", got)
	}
	if got := addedGroups([]int64{1, 2}, []int64{1}); len(got) != 0 {
		t.Fatalf("removing a team adds nothing: got %v", got)
	}
	if got := addedGroups(nil, []int64{5, 6}); len(got) != 2 {
		t.Fatalf("all teams new when prev empty: got %v", got)
	}
	if got := addedGroups([]int64{1}, nil); len(got) != 0 {
		t.Fatalf("empty next adds nothing: got %v", got)
	}
}
