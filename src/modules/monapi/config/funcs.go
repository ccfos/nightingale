package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/toolkits/pkg/logger"
)

// InitLogger init logger toolkits
func InitLogger() {
	c := Get().Logger

	lb, err := logger.NewFileBackend(c.Dir)
	if err != nil {
		fmt.Println("cannot init logger:", err)
		os.Exit(1)
	}

	lb.SetRotateByHour(true)
	lb.SetKeepHours(c.KeepHours)

	logger.SetLogging(c.Level, lb)
}

// slice set
func Set(s []string) []string {
	m := make(map[string]interface{})
	for i := 0; i < len(s); i++ {
		if strings.TrimSpace(s[i]) == "" {
			continue
		}

		m[s[i]] = 1
	}

	s2 := []string{}
	for k := range m {
		s2 = append(s2, k)
	}

	return s2
}

func InSlice(val string, slice []string) bool {
	for i := 0; i < len(slice); i++ {
		if slice[i] == val {
			return true
		}
	}

	return false
}

func SplitN(m, n int) [][]int {
	var res [][]int

	if n <= 0 {
		return [][]int{[]int{0, m}}
	}

	for i := 0; i < m; i = i + n {
		var start, end int
		start = i
		end = i + n

		if end >= m {
			end = m
		}

		res = append(res, []int{start, end})

	}
	return res
}
