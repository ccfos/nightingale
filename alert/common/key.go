package common

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

func RuleKey(datasourceId, id int64) string {
	return fmt.Sprintf("alert-%d-%d", datasourceId, id)
}

func MatchTags(eventTagsMap map[string]string, itags []models.TagFilter) bool {
	for _, filter := range itags {
		// target_group in和not in优先特殊处理：匹配通过则继续下一个 filter，匹配失败则整组不匹配
		if filter.Key == "target_group" {
			// target 字段从 event.JsonTagsAndValue() 中获取的
			v, ok := eventTagsMap["target"]
			if !ok {
				return false
			}
			if !targetGroupMatch(v, filter) {
				return false
			}
			continue
		}

		// 普通标签按原逻辑处理
		value, has := eventTagsMap[filter.Key]
		if !has {
			return false
		}
		if !matchTag(value, filter) {
			return false
		}
	}
	return true
}
func MatchGroupsName(groupName string, groupFilter []models.TagFilter) bool {
	for _, filter := range groupFilter {
		if !matchTag(groupName, filter) {
			return false
		}
	}
	return true
}

func matchTag(value string, filter models.TagFilter) bool {
	switch filter.Func {
	case "==":
		return strings.TrimSpace(fmt.Sprintf("%v", filter.Value)) == strings.TrimSpace(value)
	case "!=":
		return strings.TrimSpace(fmt.Sprintf("%v", filter.Value)) != strings.TrimSpace(value)
	case "in":
		_, has := filter.Vset[value]
		return has
	case "not in":
		_, has := filter.Vset[value]
		return !has
	case "=~":
		return filter.Regexp.MatchString(value)
	case "!~":
		return !filter.Regexp.MatchString(value)
	}
	// unexpected func
	return false
}

// targetGroupMatch 处理 target_group 的特殊匹配逻辑
func targetGroupMatch(value string, filter models.TagFilter) bool {
	var valueMap map[string]interface{}
	if err := json.Unmarshal([]byte(value), &valueMap); err != nil {
		return false
	}
	switch filter.Func {
	case "in", "not in":
		filterValueIds, ok := filter.Value.([]interface{})
		if !ok {
			return false
		}
		groupIds, ok := valueMap["group_ids"].([]interface{})
		if !ok {
			return false
		}
		// in 只要 groupIds 中有一个在 filterGroupIds 中出现，就返回 true
		// not in 则相反
		found := false
		for _, gid := range groupIds {
			for _, fgid := range filterValueIds {
				if fmt.Sprintf("%v", gid) == fmt.Sprintf("%v", fgid) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if filter.Func == "in" {
			return found
		}
		// filter.Func == "not in"
		return !found

	case "=~", "!~":
		// 正则满足一个就认为 matched
		groupNames, ok := valueMap["group_names"].([]interface{})
		if !ok {
			return false
		}
		matched := false
		for _, gname := range groupNames {
			if filter.Regexp.MatchString(fmt.Sprintf("%v", gname)) {
				matched = true
				break
			}
		}
		if filter.Func == "=~" {
			return matched
		}
		// "!~": 只要有一个匹配就返回 false，否则返回 true
		return !matched
	default:
		return false
	}
}
