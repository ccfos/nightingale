package str

import "strings"

func Contains(smallSlice, bigSlice []string) bool {
	for i := 0; i < len(smallSlice); i++ {
		if !InSlice(smallSlice[i], bigSlice) {
			return false
		}

	}

	return true
}

func InSlice(val string, slice []string) bool {
	for i := 0; i < len(slice); i++ {
		if slice[i] == val {
			return true
		}
	}

	return false
}

// 分割m, 每次n个
func SplitN(m, n int) [][]int {
	var res [][]int

	if n <= 0 {
		return [][]int{{0, m}}
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

// slice去重
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

func SetInt64(s []int64) []int64 {
	m := make(map[int64]interface{})
	for i := 0; i < len(s); i++ {
		m[s[i]] = 1
	}

	s2 := []int64{}
	for k := range m {
		s2 = append(s2, k)
	}

	return s2
}
