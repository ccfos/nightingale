package models

import (
	"sort"
	"testing"
)

// 精确匹配的 values 兼容三种形态：数字 id、数字字符串、数据源名。
// AI 生成/模板导入/API 直调的规则里可能出现后两种，静默丢弃会让规则解析不出数据源。
func TestGetDatasourceIDsByDatasourceQueries_ExactMatch(t *testing.T) {
	idMap := map[int64]struct{}{3: {}, 4: {}, 5: {}}
	nameMap := map[string]int64{"prometheus": 3, "mysql": 4, "es-log": 5}

	cases := []struct {
		name   string
		values []interface{}
		want   []int64
	}{
		{"numeric id", []interface{}{float64(4)}, []int64{4}},
		{"numeric string", []interface{}{"4"}, []int64{4}},
		{"datasource name", []interface{}{"mysql"}, []int64{4}},
		{"mixed", []interface{}{float64(3), "mysql"}, []int64{3, 4}},
		{"unknown name dropped", []interface{}{"nonexistent"}, []int64{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := GetDatasourceIDsByDatasourceQueries(
				[]DatasourceQuery{{MatchType: 0, Op: "in", Values: c.values}},
				idMap, nameMap,
			)
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

// 数据源名允许是纯数字：此时应按名字解析到它真正的 id，而不是把字符串当成 id
func TestGetDatasourceIDsByDatasourceQueries_NumericNameWins(t *testing.T) {
	idMap := map[int64]struct{}{5: {}, 7: {}}
	nameMap := map[string]int64{"5": 7, "mysql": 5}

	cases := []struct {
		name   string
		values []interface{}
		want   []int64
	}{
		{"numeric name resolves by name", []interface{}{"5"}, []int64{7}},
		{"non-name numeric string falls back to id", []interface{}{"7"}, []int64{7}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := GetDatasourceIDsByDatasourceQueries(
				[]DatasourceQuery{{MatchType: 0, Op: "in", Values: c.values}},
				idMap, nameMap,
			)
			sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}
