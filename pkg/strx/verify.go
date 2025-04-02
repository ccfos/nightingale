package strx

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/errorx"
)

func IsValidURL(url string) bool {
	re := regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)
	return re.MatchString(url)
}

func IdsInt64ForAPI(ids string, sep ...string) []int64 {
	if ids == "" {
		return []int64{}
	}

	s := ","
	if len(sep) > 0 {
		s = sep[0]
	}

	var arr []string

	if s == " " {
		arr = strings.Fields(ids)
	} else {
		arr = strings.Split(ids, s)
	}

	count := len(arr)
	ret := make([]int64, 0, count)
	for i := 0; i < count; i++ {
		if arr[i] != "" {
			id, err := strconv.ParseInt(arr[i], 10, 64)
			if err != nil {
				errorx.Bomb(http.StatusBadRequest, "cannot convert %s to int64", arr[i])
			}

			ret = append(ret, id)
		}
	}

	return ret
}
