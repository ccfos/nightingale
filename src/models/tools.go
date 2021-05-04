package models

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/toolkits/pkg/str"
)

// CryptoPass crypto password use salt
func CryptoPass(raw string) (string, error) {
	if raw == "" {
		return "", _e("Password is not set")
	}
	salt, err := ConfigsGet("salt")
	if err != nil {
		return "", _e("query salt from mysql fail: %v", err)
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

func parseConditions(conditions string) (string, []interface{}) {
	conditions = strings.TrimSpace(conditions)
	if conditions == "" {
		return "", []interface{}{}
	}

	var (
		where []string
		args  []interface{}
	)

	arr := strings.Split(conditions, ",")
	cnt := len(arr)
	for i := 0; i < cnt; i++ {
		if strings.Contains(arr[i], "~=") {
			pair := strings.Split(arr[i], "~=")
			if WarningStr(pair[0]) {
				continue
			}
			if strings.Contains(pair[0], "|") {
				keys := strings.Split(pair[0], "|")
				str := "("
				for i, k := range keys {
					if i < len(keys)-1 {
						str += fmt.Sprintf("%s like ? OR ", k)
					} else {
						str += fmt.Sprintf("%s like ?)", k)
					}
					args = append(args, "%"+pair[1]+"%")
				}
				where = append(where, str)
			} else {
				where = append(where, pair[0]+" like ?")
				args = append(args, "%"+pair[1]+"%")
			}
			continue
		}

		if strings.Contains(arr[i], "!=") {
			pair := strings.Split(arr[i], "!=")
			if WarningStr(pair[0]) {
				continue
			}
			where = append(where, pair[0]+" != ?")
			args = append(args, pair[1])
			continue
		}

		if strings.Contains(arr[i], ">=") {
			pair := strings.Split(arr[i], ">=")
			if WarningStr(pair[0]) {
				continue
			}
			where = append(where, pair[0]+" >= ?")
			args = append(args, pair[1])
			continue
		}

		if strings.Contains(arr[i], "<=") {
			pair := strings.Split(arr[i], "<=")
			if WarningStr(pair[0]) {
				continue
			}
			where = append(where, pair[0]+" <= ?")
			args = append(args, pair[1])
			continue
		}

		if strings.Contains(arr[i], "=") {
			pair := strings.Split(arr[i], "=")
			if WarningStr(pair[0]) {
				continue
			}
			where = append(where, pair[0]+" = ?")
			args = append(args, pair[1])
			continue
		}

		if strings.Contains(arr[i], ">") {
			pair := strings.Split(arr[i], ">")
			if WarningStr(pair[0]) {
				continue
			}
			where = append(where, pair[0]+" > ?")
			args = append(args, pair[1])
			continue
		}

		if strings.Contains(arr[i], "<") {
			pair := strings.Split(arr[i], "<")
			if WarningStr(pair[0]) {
				continue
			}
			where = append(where, pair[0]+" < ?")
			args = append(args, pair[1])
			continue
		}

		if strings.Contains(arr[i], "^^") {
			pair := strings.Split(arr[i], "^^")
			if WarningStr(pair[0]) {
				continue
			}
			where = append(where, pair[0]+" in ("+strings.Join(strings.Split(pair[1], "|"), ",")+")")
			continue
		}
	}

	return strings.Join(where, " and "), args
}

var dbfieldPattern = regexp.MustCompile("^[a-z][a-z0-9\\|_A-Z]*$")

func WarningStr(s string) bool {
	if dbfieldPattern.MatchString(s) || s == "" {
		return false
	}

	return true
}
