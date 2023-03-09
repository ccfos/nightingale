package common

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/models"
)

func RuleKey(datasourceId, id int64) string {
	return fmt.Sprintf("alert-%d-%d", datasourceId, id)
}

func MatchTags(eventTagsMap map[string]string, itags []models.TagFilter) bool {
	for _, filter := range itags {
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

func matchTag(value string, filter models.TagFilter) bool {
	switch filter.Func {
	case "==":
		return filter.Value == value
	case "!=":
		return filter.Value != value
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
	// unexpect func
	return false
}
