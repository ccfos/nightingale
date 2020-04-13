package cache

import (
	"sort"
)

type TagPair struct {
	Key    string   `json:"tagk"` // json 和变量不一致为了兼容前端
	Values []string `json:"tagv"`
}

type TagPairs []*TagPair

func (t TagPairs) Len() int {
	return len(t)
}

func (t TagPairs) Less(i, j int) bool {
	return t[i].Key > t[j].Key
}

func (t TagPairs) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func getMatchedTags(tagsMap map[string][]string, include, exclude []*TagPair) map[string][]string {
	inMap := make(map[string]map[string]bool)
	exMap := make(map[string]map[string]bool)

	if len(include) > 0 {
		for _, tagPair := range include {
			// include 中的 tagKey 在 tags 列表中不存在
			if _, exists := tagsMap[tagPair.Key]; !exists {
				return nil
			}
			// tagKey 存在，初始化 map
			if _, found := inMap[tagPair.Key]; !found {
				inMap[tagPair.Key] = make(map[string]bool)
			}
			// 对存在的值进行标记
			for _, tagv := range tagPair.Values {
				inMap[tagPair.Key][tagv] = true
			}
		}
	}

	if len(exclude) > 0 {
		for _, tagPair := range exclude {
			if _, found := exMap[tagPair.Key]; !found {
				exMap[tagPair.Key] = make(map[string]bool)
			}
			for _, tagv := range tagPair.Values {
				exMap[tagPair.Key][tagv] = true
			}
		}
	}

	fullMatch := make(map[string][]string)
	for tagk, tagvs := range tagsMap {
		for _, tagv := range tagvs {
			// 排除必须排除的, exclude 的优先级高于 include
			if _, tagkExists := exMap[tagk]; tagkExists {
				if _, tagvExists := exMap[tagk][tagv]; tagvExists {
					continue
				}
			}
			// 包含必须包含的
			if _, tagkExists := inMap[tagk]; tagkExists {
				if _, tagvExists := inMap[tagk][tagv]; tagvExists {
					if _, found := fullMatch[tagk]; !found {
						fullMatch[tagk] = make([]string, 0)
					}
					fullMatch[tagk] = append(fullMatch[tagk], tagv)
				}
				continue
			}
			// 除此之外全都包含
			if _, found := fullMatch[tagk]; !found {
				fullMatch[tagk] = make([]string, 0)
			}
			fullMatch[tagk] = append(fullMatch[tagk], tagv)
		}
	}

	return fullMatch
}

// GetAllCounter returns all possible tags combination.
// But not all of them will be in the CounterMaps.
func GetAllCounter(tags []*TagPair) []string {
	if len(tags) == 0 {
		return []string{}
	}

	head := tags[0]
	firstList := make([]string, len(head.Values))

	for i, v := range head.Values {
		firstList[i] = head.Key + "=" + v
	}

	otherList := GetAllCounter(tags[1:])
	if len(otherList) == 0 {
		return firstList
	}

	rest := make([]string, len(otherList)*len(firstList))
	i := 0
	for _, firstV := range firstList {
		for _, otherV := range otherList {
			rest[i] = firstV + "," + otherV
			i++
		}
	}

	return rest
}

// OverMaxLimit check whether it can over limit or not.
func OverMaxLimit(tagMap map[string][]string, limit int) bool {
	multiRes := 1

	for _, values := range tagMap {
		multiRes = multiRes * len(values)
		if multiRes > limit {
			return true
		}
	}

	return false
}

func TagPairToMap(tagPairs []*TagPair) map[string][]string {
	tagMap := make(map[string][]string)
	for _, tagPair := range tagPairs {
		tagMap[tagPair.Key] = tagPair.Values
	}
	return tagMap
}

func GetSortTags(tagMap map[string][]string) []*TagPair {
	var keys []string
	for key := range tagMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	newTags := make([]*TagPair, len(keys))
	for i, key := range keys {
		newTags[i] = &TagPair{Key: key, Values: tagMap[key]}
	}
	return newTags
}
