package str

import (
	"strings"

	"github.com/toolkits/pkg/str"
)

func Checksum(endpoint string, metric string, tags string) string {
	return str.MD5(PK(endpoint, metric, tags))
}

func GetKey(filename string) string {
	arr := strings.Split(filename, "/")
	if len(arr) < 2 {
		return ""
	}
	a := strings.Split(arr[1], "_")
	if len(a) > 1 {
		return a[0]
	}
	return ""
}
