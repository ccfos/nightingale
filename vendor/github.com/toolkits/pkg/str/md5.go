package str

import (
	"crypto/md5"
	"encoding/hex"
	"io"
)

func MD5(str string) string {
	hasher := md5.New()
	io.WriteString(hasher, str)
	return hex.EncodeToString(hasher.Sum(nil))
}
