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

// allValueDeepEqualOmitOrder 判断两个字符串切片是否相等，不考虑顺序
func allValueDeepEqualOmitOrder(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	slices.Sort(got)
	slices.Sort(want)
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func Test_removeVal(t *testing.T) {
	type args struct {
		promql string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		// TODO: Add test cases.
		{
			name: "removeVal1",
			args: args{
				promql: "mem{test1=\"$test1\",test2=\"$test2\",test3=\"$test3\"} > $val",
			},
			want: "mem{} > $val",
		},
		{
			name: "removeVal2",
			args: args{
				promql: "mem{test1=\"test1\",test2=\"$test2\",test3=\"$test3\"} > $val",
			},
			want: "mem{test1=\"test1\"} > $val",
		},
		{
			name: "removeVal3",
			args: args{
				promql: "mem{test1=\"$test1\",test2=\"test2\",test3=\"$test3\"} > $val",
			},
			want: "mem{test2=\"test2\"} > $val",
		},
		{
			name: "removeVal4",
			args: args{
				promql: "mem{test1=\"$test1\",test2=\"$test2\",test3=\"test3\"} > $val",
			},
			want: "mem{test3=\"test3\"} > $val",
		},
		{
			name: "removeVal5",
			args: args{
				promql: "mem{test1=\"$test1\",test2=\"test2\",test3=\"test3\"} > $val",
			},
			want: "mem{test2=\"test2\",test3=\"test3\"} > $val",
		},
		{
			name: "removeVal6",
			args: args{
				promql: "mem{test1=\"test1\",test2=\"$test2\",test3=\"test3\"} > $val",
			},
			want: "mem{test1=\"test1\",test3=\"test3\"} > $val",
		},
		{
			name: "removeVal7",
			args: args{
				promql: "mem{test1=\"test1\",test2=\"test2\",test3='$test3'} > $val",
			},
			want: "mem{test1=\"test1\",test2=\"test2\"} > $val",
		},
		{
			name: "removeVal8",
			args: args{
				promql: "mem{test1=\"test1\",test2=\"test2\",test3=\"test3\"} > $val",
			},
			want: "mem{test1=\"test1\",test2=\"test2\",test3=\"test3\"} > $val",
		},
		{
			name: "removeVal9",
			args: args{
				promql: "mem{test1=\"$test1\",test2=\"test2\"} > $val1 and mem{test3=\"test3\",test4=\"test4\"} > $val2",
			},
			want: "mem{test2=\"test2\"} > $val1 and mem{test3=\"test3\",test4=\"test4\"} > $val2",
		},
		{
			name: "removeVal10",
			args: args{
				promql: "mem{test1=\"test1\",test2='$test2'} > $val1 and mem{test3=\"test3\",test4=\"test4\"} > $val2",
			},
			want: "mem{test1=\"test1\"} > $val1 and mem{test3=\"test3\",test4=\"test4\"} > $val2",
		},
		{
			name: "removeVal11",
			args: args{
				promql: "mem{test1='test1',test2=\"test2\"} > $val1 and mem{test3=\"$test3\",test4=\"test4\"} > $val2",
			},
			want: "mem{test1='test1',test2=\"test2\"} > $val1 and mem{test4=\"test4\"} > $val2",
		},
		{
			name: "removeVal12",
			args: args{
				promql: "mem{test1=\"test1\",test2=\"test2\"} > $val1 and mem{test3=\"test3\",test4=\"$test4\"} > $val2",
			},
			want: "mem{test1=\"test1\",test2=\"test2\"} > $val1 and mem{test3=\"test3\"} > $val2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := removeVal(tt.args.promql); got != tt.want {
				t.Errorf("removeVal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractVarMapping(t *testing.T) {
	tests := []struct {
		name   string
		promql string
		want   map[string]string
	}{
		{
			name:   "单个花括号单个变量",
			promql: `mem_used_percent{host="$my_host"} > $val`,
			want:   map[string]string{"my_host": "host"},
		},
		{
			name:   "单个花括号多个变量",
			promql: `mem_used_percent{host="$my_host",region="$region",env="prod"} > $val`,
			want:   map[string]string{"my_host": "host", "region": "region"},
		},
		{
			name:   "多个花括号多个变量",
			promql: `sum(rate(mem_used_percent{host="$my_host"})) by (instance) + avg(node_load1{region="$region"}) > $val`,
			want:   map[string]string{"my_host": "host", "region": "region"},
		},
		{
			name:   "相同变量出现多次",
			promql: `sum(rate(mem_used_percent{host="$my_host"})) + avg(node_load1{host="$my_host"}) > $val`,
			want:   map[string]string{"my_host": "host"},
		},
		{
			name:   "没有变量",
			promql: `mem_used_percent{host="localhost",region="cn"} > 80`,
			want:   map[string]string{},
		},
		{
			name:   "没有花括号",
			promql: `80 > $val`,
			want:   map[string]string{},
		},
		{
			name:   "格式不规范的标签",
			promql: `mem_used_percent{host=$my_host,region = $region} > $val`,
			want:   map[string]string{"my_host": "host", "region": "region"},
		},
		{
			name:   "空花括号",
			promql: `mem_used_percent{} > $val`,
			want:   map[string]string{},
		},
		{
			name:   "不完整的花括号",
			promql: `mem_used_percent{host="$my_host"`,
			want:   map[string]string{},
		},
		{
			name:   "复杂表达式",
			promql: `sum(rate(http_requests_total{handler="$handler",code="$code"}[5m])) by (handler) / sum(rate(http_requests_total{handler="$handler"}[5m])) by (handler) * 100 > $threshold`,
			want:   map[string]string{"handler": "handler", "code": "code"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractVarMapping(tt.promql)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractVarMapping() = %v, want %v", got, tt.want)
			}
		})
	}
}
