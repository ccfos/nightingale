package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/toolkits/pkg/str"
)

// CryptoPass crypto password use salt
func CryptoPass(raw string) (string, error) {
	salt, err := ConfigsGet("salt")
	if err != nil {
		return "", fmt.Errorf("query salt from mysql fail: %v", err)
	}

	return str.MD5(salt + "<-*Uk30^96eY*->" + raw), nil
}

func GenUUIDForUser(username string) string {
	return str.MD5(username + fmt.Sprint(time.Now().UnixNano()))
}

// Paths 把长路径切成多个path，比如：
// cop.sre.falcon.judge.hna被切成：
// cop、cop.sre、cop.sre.falcon、cop.sre.falcon.judge、cop.sre.falcon.judge.hna
func Paths(longPath string) []string {
	names := strings.Split(longPath, ".")
	count := len(names)
	paths := make([]string, 0, count)

	for i := 1; i <= count; i++ {
		paths = append(paths, strings.Join(names[:i], "."))
	}

	return paths
}
