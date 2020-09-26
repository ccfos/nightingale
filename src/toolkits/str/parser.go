package str

import "strings"

func TrimStringSlice(raw []string) []string {
	if raw == nil {
		return []string{}
	}

	cnt := len(raw)
	arr := make([]string, 0, cnt)
	for i := 0; i < cnt; i++ {
		item := strings.TrimSpace(raw[i])
		if item == "" {
			continue
		}

		arr = append(arr, item)
	}

	return arr
}
