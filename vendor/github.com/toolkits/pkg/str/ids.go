package str

import (
	"fmt"
	"strconv"
	"strings"
)

func IdsInt64(ids string) []int64 {
	if ids == "" {
		return []int64{}
	}

	arr := strings.Split(ids, ",")
	count := len(arr)
	ret := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		if arr[i] != "" {
			id, err := strconv.ParseInt(arr[i], 10, 64)
			if err == nil {
				ret = append(ret, id)
			}
		}
	}

	return ret
}

func IdsString(ids []int64) string {
	count := len(ids)
	arr := make([]string, count)
	for i := 0; i < count; i++ {
		arr[i] = fmt.Sprintf("%d", ids[i])
	}
	return strings.Join(arr, ",")
}
