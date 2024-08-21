package eval

import (
	"reflect"
	"testing"

	"golang.org/x/exp/slices"
)

var (
	reHashTagIndex1 = map[uint64][][]uint64{
		1: {
			{1, 2}, {3, 4},
		},
		2: {
			{5, 6}, {7, 8},
		},
	}
	reHashTagIndex2 = map[uint64][][]uint64{
		1: {
			{9, 10}, {11, 12},
		},
		3: {
			{13, 14}, {15, 16},
		},
	}
	seriesTagIndex1 = map[uint64][]uint64{
		1: {1, 2, 3, 4},
		2: {5, 6, 7, 8},
	}
	seriesTagIndex2 = map[uint64][]uint64{
		1: {9, 10, 11, 12},
		3: {13, 14, 15, 16},
	}
)

func Test_originalJoin(t *testing.T) {
	type args struct {
		seriesTagIndex1 map[uint64][]uint64
		seriesTagIndex2 map[uint64][]uint64
	}
	tests := []struct {
		name string
		args args
		want map[uint64][]uint64
	}{
		{
			name: "original join",
			args: args{
				seriesTagIndex1: map[uint64][]uint64{
					1: {1, 2, 3, 4},
					2: {5, 6, 7, 8},
				},
				seriesTagIndex2: map[uint64][]uint64{
					1: {9, 10, 11, 12},
					3: {13, 14, 15, 16},
				},
			},
			want: map[uint64][]uint64{
				1: {1, 2, 3, 4, 9, 10, 11, 12},
				2: {5, 6, 7, 8},
				3: {13, 14, 15, 16},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := originalJoin(tt.args.seriesTagIndex1, tt.args.seriesTagIndex2); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("originalJoin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_exclude(t *testing.T) {
	type args struct {
		reHashTagIndex1 map[uint64][][]uint64
		reHashTagIndex2 map[uint64][][]uint64
	}
	tests := []struct {
		name string
		args args
		want map[uint64][]uint64
	}{
		{
			name: "left exclude",
			args: args{
				reHashTagIndex1: reHashTagIndex1,
				reHashTagIndex2: reHashTagIndex2,
			},
			want: map[uint64][]uint64{
				0: {5, 6},
				1: {7, 8},
			},
		},
		{
			name: "right exclude",
			args: args{
				reHashTagIndex1: reHashTagIndex2,
				reHashTagIndex2: reHashTagIndex1,
			},
			want: map[uint64][]uint64{
				3: {13, 14},
				4: {15, 16},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := exclude(tt.args.reHashTagIndex1, tt.args.reHashTagIndex2); !allValueDeepEqual(flatten(got), tt.want) {
				t.Errorf("exclude() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_noneJoin(t *testing.T) {
	type args struct {
		seriesTagIndex1 map[uint64][]uint64
		seriesTagIndex2 map[uint64][]uint64
	}
	tests := []struct {
		name string
		args args
		want map[uint64][]uint64
	}{
		{
			name: "none join, direct splicing",
			args: args{
				seriesTagIndex1: seriesTagIndex1,
				seriesTagIndex2: seriesTagIndex2,
			},
			want: map[uint64][]uint64{
				0: {1, 2, 3, 4},
				1: {5, 6, 7, 8},
				2: {9, 10, 11, 12},
				3: {13, 14, 15, 16},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := noneJoin(tt.args.seriesTagIndex1, tt.args.seriesTagIndex2); !allValueDeepEqual(got, tt.want) {
				t.Errorf("noneJoin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_cartesianJoin(t *testing.T) {
	type args struct {
		seriesTagIndex1 map[uint64][]uint64
		seriesTagIndex2 map[uint64][]uint64
	}
	tests := []struct {
		name string
		args args
		want map[uint64][]uint64
	}{
		{
			name: "cartesian join",
			args: args{
				seriesTagIndex1: seriesTagIndex1,
				seriesTagIndex2: seriesTagIndex2,
			},
			want: map[uint64][]uint64{
				0: {1, 2, 3, 4, 9, 10, 11, 12},
				1: {5, 6, 7, 8, 9, 10, 11, 12},
				2: {5, 6, 7, 8, 13, 14, 15, 16},
				3: {1, 2, 3, 4, 13, 14, 15, 16},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cartesianJoin(tt.args.seriesTagIndex1, tt.args.seriesTagIndex2); !allValueDeepEqual(got, tt.want) {
				t.Errorf("cartesianJoin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_onJoin(t *testing.T) {
	type args struct {
		reHashTagIndex1 map[uint64][][]uint64
		reHashTagIndex2 map[uint64][][]uint64
		joinType        JoinType
	}
	tests := []struct {
		name string
		args args
		want map[uint64][]uint64
	}{
		{
			name: "left join",
			args: args{
				reHashTagIndex1: reHashTagIndex1,
				reHashTagIndex2: reHashTagIndex2,
				joinType:        Left,
			},
			want: map[uint64][]uint64{
				1: {1, 2, 9, 10},
				2: {3, 4, 9, 10},
				3: {1, 2, 11, 12},
				4: {3, 4, 11, 12},
				5: {5, 6},
				6: {7, 8},
			},
		},
		{
			name: "right join",
			args: args{
				reHashTagIndex1: reHashTagIndex2,
				reHashTagIndex2: reHashTagIndex1,
				joinType:        Right,
			},
			want: map[uint64][]uint64{
				1: {1, 2, 9, 10},
				2: {3, 4, 9, 10},
				3: {1, 2, 11, 12},
				4: {3, 4, 11, 12},
				5: {13, 14},
				6: {15, 16},
			},
		},

		{
			name: "inner join",
			args: args{
				reHashTagIndex1: reHashTagIndex1,
				reHashTagIndex2: reHashTagIndex2,
				joinType:        Inner,
			},
			want: map[uint64][]uint64{
				1: {1, 2, 9, 10},
				2: {3, 4, 9, 10},
				3: {1, 2, 11, 12},
				4: {3, 4, 11, 12},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := onJoin(tt.args.reHashTagIndex1, tt.args.reHashTagIndex2, tt.args.joinType); !allValueDeepEqual(flatten(got), tt.want) {
				t.Errorf("onJoin() = %v, want %v", got, tt.want)
			}
		})
	}
}

// allValueDeepEqual 判断 map 的 value 是否相同，不考虑 key
func allValueDeepEqual(got, want map[uint64][]uint64) bool {
	if len(got) != len(want) {
		return false
	}
	for _, v1 := range got {
		curEqual := false
		slices.Sort(v1)
		for _, v2 := range want {
			slices.Sort(v2)
			if reflect.DeepEqual(v1, v2) {
				curEqual = true
				break
			}
		}
		if !curEqual {
			return false
		}
	}
	return true
}
