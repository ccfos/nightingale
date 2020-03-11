// +build xxhash

package str

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/cespare/xxhash"
)

func Checksum(strs ...string) uint64 {
	ret := bufferPool.Get().(*bytes.Buffer)
	ret.Reset()
	defer bufferPool.Put(ret)
	count := len(strs)
	if count == 0 {
		return 0
	}

	ret.WriteString(strs[0])
	for i := 1; i < count-1; i++ {
		ret.WriteString(SEPERATOR)
		ret.WriteString(strs[i])
	}

	if strs[count-1] != "" {
		ret.WriteString(SEPERATOR)
		ret.WriteString(strs[count-1])
	}

	return xxhash.Sum64(ret.Bytes())
}

func GetKey(filename string) uint64 {
	arr := strings.Split(filename, "/")
	if len(arr) < 2 {
		return 0
	}
	a := strings.Split(arr[1], "_")
	if len(a) > 1 {
		key, _ := strconv.ParseUint(a[0], 10, 64)
		return key
	}
	return 0
}
