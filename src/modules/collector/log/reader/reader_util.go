package reader

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"syscall"
	"time"
)

func GetNowPath(path string) string {
	return getLogPath(path, true)
}

func GetCurrentPath(path string) string {
	return getLogPath(path, false)
}

func getLogPath(path string, isnext bool) string {
	pat := `(\$\{(%[YmdH][^\/]*)+\})`
	reg := regexp.MustCompile(pat)
	return reg.ReplaceAllStringFunc(path, func(s string) string {
		stringv := strings.TrimFunc(s, func(r rune) bool {
			if r == '$' || r == '{' || r == '}' {
				return true
			}
			return false
		})
		name := strings.Split(strings.TrimLeft(stringv, "%"), "%")
		now := time.Now()
		if isnext {
			switch name[len(name)-1] {
			case "Y", "m", "d":
				if now.Hour() == 23 {
					now = time.Now() //.Add(time.Hour)
				}
			case "H":
				now = time.Now() //.Add(time.Hour)
			}
		}
		for k, v := range name {
			if strings.Contains(v, "Y") {
				if strings.HasPrefix(v, "Y") {
					year := fmt.Sprintf("%d", now.Year())
					name[k] = strings.Replace(v, "Y", year, 1)
				}
			} else if strings.Contains(v, "m") {
				if strings.HasPrefix(v, "m") {
					month := fmt.Sprintf("%02d", now.Month())
					name[k] = strings.Replace(v, "m", month, 1)
				}
			} else if strings.Contains(v, "d") {
				if strings.HasPrefix(v, "d") {
					day := fmt.Sprintf("%02d", now.Day())
					name[k] = strings.Replace(v, "d", day, 1)
				}
			} else if strings.Contains(v, "H") {
				if strings.HasPrefix(v, "H") {
					hour := fmt.Sprintf("%02d", now.Hour())
					name[k] = strings.Replace(v, "H", hour, 1)
				}
			}

		}
		return strings.Join(name, "")
	})

}

func GetFileInodeNum(path string) uint64 {
	s, err := os.Stat(path)
	if err != nil {
		return 0
	}

	stat, ok := s.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return stat.Ino
}
