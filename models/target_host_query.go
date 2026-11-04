package models

import (
	"encoding/json"
	"regexp"
	"strings"
)

// FilterTargetsByHostQueries 在内存中按主机筛选条件过滤 targets，
// 语义与 GetHostsQuery + TargetFilterQueryBuild 生成的 SQL 保持一致，
// 供 edge 等不连 DB 的场景用本地 TargetCache 替代 /v1/n9e/targets-of-host-query。
//
// 对齐要点：
//   - 同一个 HostQuery 内的多个条件为 OR，多个 HostQuery 之间为 AND；
//   - SQL 对 target_busi_group 做 LEFT JOIN，条件按 (target, 某个业务组) 行求值，
//     任意一行满足全部条件即命中；未归组的 target 对应一行 group 为 NULL 的记录；
//   - LIKE 按 MySQL 默认排序规则处理：大小写不敏感，% 与 _ 为通配符，\ 为转义。
func FilterTargetsByHostQueries(targets []*Target, queries []HostQuery) []*Target {
	filters := compileHostQueries(queries)
	res := make([]*Target, 0, len(targets))
	for _, t := range targets {
		if t != nil && targetMatchFilters(t, filters) {
			res = append(res, t)
		}
	}
	return res
}

// hostFilter 对一行 (target, group) 求值，group 为 nil 表示未归组的那一行
type hostFilter func(t *targetView, group *int64) bool

type targetView struct {
	*Target
	tags     string // 对应 DB 的 tags 列
	hostTags *string
	groupSet map[int64]struct{}
}

func newTargetView(t *Target) *targetView {
	v := &targetView{Target: t, tags: t.Tags}
	// edge 从中心同步的 target 不带 Tags（json:"-"），用 TagsJSON 还原
	if v.tags == "" && len(t.TagsJSON) > 0 {
		v.tags = strings.Join(t.TagsJSON, " ")
	}
	// host_tags 在 DB 中按 json 序列化存储，nil 存为 NULL
	if t.HostTags != nil {
		if b, err := json.Marshal(t.HostTags); err == nil {
			s := string(b)
			v.hostTags = &s
		}
	}
	v.groupSet = make(map[int64]struct{}, len(t.GroupIds))
	for _, id := range t.GroupIds {
		v.groupSet[id] = struct{}{}
	}
	return v
}

func (v *targetView) inAnyGroup(ids []int64) bool {
	for _, id := range ids {
		if _, ok := v.groupSet[id]; ok {
			return true
		}
	}
	return false
}

func targetMatchFilters(t *Target, filters []hostFilter) bool {
	if len(filters) == 0 {
		return true
	}
	v := newTargetView(t)

	rows := make([]*int64, 0, len(t.GroupIds))
	for i := range t.GroupIds {
		rows = append(rows, &t.GroupIds[i])
	}
	if len(rows) == 0 {
		rows = append(rows, nil)
	}

	for _, row := range rows {
		ok := true
		for _, f := range filters {
			if !f(v, row) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func compileHostQueries(queries []HostQuery) []hostFilter {
	var filters []hostFilter
	for _, q := range queries {
		switch q.Key {
		case "group_ids":
			if f := compileGroupFilter(q); f != nil {
				filters = append(filters, f)
			}
		case "tags":
			if f := compileTagsFilter(q); f != nil {
				filters = append(filters, f)
			}
		case "hosts":
			if f := compileHostsFilter(q); f != nil {
				filters = append(filters, f)
			}
		}
		// 其他 key 在 SQL 中生成空条件，不做过滤
	}
	return filters
}

func compileGroupFilter(q HostQuery) hostFilter {
	ids := ParseInt64(q.Values)
	if len(ids) == 0 {
		return nil
	}
	hasZero := false
	nonZeroIds := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			hasZero = true
		} else {
			nonZeroIds = append(nonZeroIds, id)
		}
	}
	inIds := func(group *int64) bool {
		if group == nil {
			return false
		}
		for _, id := range nonZeroIds {
			if *group == id {
				return true
			}
		}
		return false
	}

	if q.Op == "==" {
		switch {
		case hasZero && len(nonZeroIds) == 0:
			return func(_ *targetView, group *int64) bool { return group == nil }
		case hasZero:
			return func(_ *targetView, group *int64) bool { return group == nil || inIds(group) }
		default:
			return func(_ *targetView, group *int64) bool { return inIds(group) }
		}
	}

	switch {
	case hasZero && len(nonZeroIds) == 0:
		return func(_ *targetView, group *int64) bool { return group != nil }
	case hasZero:
		return func(v *targetView, group *int64) bool { return group != nil && !v.inAnyGroup(nonZeroIds) }
	default:
		return func(v *targetView, _ *int64) bool { return !v.inAnyGroup(nonZeroIds) }
	}
}

func compileTagsFilter(q HostQuery) hostFilter {
	lst := stringValues(q.Values)
	if len(lst) == 0 {
		return nil
	}
	patterns := make([]*regexp.Regexp, 0, len(lst))
	for _, tag := range lst {
		patterns = append(patterns, likeToRegexp("%"+tag+"%"))
	}

	if q.Op == "==" {
		// tags like ? OR host_tags like ? ...
		return func(v *targetView, _ *int64) bool {
			for _, p := range patterns {
				if p.MatchString(v.tags) || (v.hostTags != nil && p.MatchString(*v.hostTags)) {
					return true
				}
			}
			return false
		}
	}

	// tags not like ? and (host_tags not like ? or host_tags is null) ...
	return func(v *targetView, _ *int64) bool {
		for _, p := range patterns {
			if p.MatchString(v.tags) || (v.hostTags != nil && p.MatchString(*v.hostTags)) {
				return false
			}
		}
		return true
	}
}

func compileHostsFilter(q HostQuery) hostFilter {
	lst := stringValues(q.Values)
	switch q.Op {
	case "==":
		// 空列表在 SQL 中是 ident IN (NULL)，不命中任何 target
		return func(v *targetView, _ *int64) bool { return containsFold(lst, v.Ident) }
	case "!=":
		// 空列表在 SQL 中是 ident NOT IN (NULL)，同样不命中任何 target
		return func(v *targetView, _ *int64) bool { return len(lst) > 0 && !containsFold(lst, v.Ident) }
	case "=~", "!~":
		if len(lst) == 0 {
			return nil
		}
		patterns := make([]*regexp.Regexp, 0, len(lst))
		for _, host := range lst {
			patterns = append(patterns, likeToRegexp(strings.ReplaceAll(host, "*", "%")))
		}
		matchAny := func(ident string) bool {
			for _, p := range patterns {
				if p.MatchString(ident) {
					return true
				}
			}
			return false
		}
		if q.Op == "=~" {
			return func(v *targetView, _ *int64) bool { return matchAny(v.Ident) }
		}
		return func(v *targetView, _ *int64) bool { return !matchAny(v.Ident) }
	}
	return nil
}

func stringValues(values []interface{}) []string {
	lst := make([]string, 0, len(values))
	for _, v := range values {
		if s, ok := v.(string); ok {
			lst = append(lst, s)
		}
	}
	return lst
}

func containsFold(lst []string, s string) bool {
	for _, item := range lst {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}

// likeToRegexp 把 SQL LIKE 模式转换为等价的正则：% 匹配任意串，_ 匹配单个字符，\ 转义下一个字符
func likeToRegexp(pattern string) *regexp.Regexp {
	var sb strings.Builder
	sb.WriteString("(?is)^")
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch c := runes[i]; c {
		case '%':
			sb.WriteString(".*")
		case '_':
			sb.WriteString(".")
		case '\\':
			if i+1 < len(runes) {
				i++
				sb.WriteString(regexp.QuoteMeta(string(runes[i])))
			} else {
				sb.WriteString(regexp.QuoteMeta(`\`))
			}
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	sb.WriteString("$")
	return regexp.MustCompile(sb.String())
}
