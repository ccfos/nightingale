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
		// target_group 优先特殊处理：匹配通过则继续下一个 filter，匹配失败则整组不匹配
		if filter.Key == "target_group" {
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
	filterValueMap, ok := filter.Value.(map[string]interface{})
	if !ok {
		return false
	}

	switch filter.Func {
	case "in":
		groupIds, ok := valueMap["group_ids"].([]interface{})
		if !ok {
			return false
		}

		filterGroupIds, ok := filterValueMap["ids"].([]interface{})
		if !ok {
			return false
		}
		// 只要 groupIds 中有一个在 filterGroupIds 中出现，就返回 true
		for _, gid := range groupIds {
			for _, fgid := range filterGroupIds {
				if fmt.Sprintf("%v", gid) == fmt.Sprintf("%v", fgid) {
					return true
				}
			}
		}
		return false
	case "=~":
		// 正则满足一个就返回 true
		groupNames, ok := valueMap["group_names"].([]interface{})
		if !ok {
			return false
		}
		for _, gname := range groupNames {
			if filter.Regexp.MatchString(fmt.Sprintf("%v", gname)) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
