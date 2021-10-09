package slice

import "strings"

func Int64Set(s []int64) (r []int64) {
	c := len(s)
	if c == 0 {
		return r
	}

	m := make(map[int64]struct{}, c)
	for i := 0; i < c; i++ {
		m[s[i]] = struct{}{}
	}

	for k := range m {
		r = append(r, k)
	}

	return r
}

func IntSet(s []int) (r []int) {
	c := len(s)
	if c == 0 {
		return r
	}

	m := make(map[int]struct{}, c)
	for i := 0; i < c; i++ {
		m[s[i]] = struct{}{}
	}

	for k := range m {
		r = append(r, k)
	}

	return r
}

func StringSet(s []string) (r []string) {
	c := len(s)
	if c == 0 {
		return r
	}

	m := make(map[string]struct{}, c)
	for i := 0; i < c; i++ {
		m[s[i]] = struct{}{}
	}

	for k := range m {
		r = append(r, k)
	}

	return r
}

func StringSetWithoutBlank(s []string) (r []string) {
	c := len(s)
	m := make(map[string]struct{}, c)
	for i := 0; i < c; i++ {
		if strings.TrimSpace(s[i]) == "" {
			continue
		}
		m[s[i]] = struct{}{}
	}

	for k := range m {
		r = append(r, k)
	}

	return r
}

func StringIn(val string, slice []string) bool {
	for i := 0; i < len(slice); i++ {
		if slice[i] == val {
			return true
		}
	}

	return false
}

func Int64In(val int64, slice []int64) bool {
	for i := 0; i < len(slice); i++ {
		if slice[i] == val {
			return true
		}
	}

	return false
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

func ArrayInGroupsOf(arr []string, num int64) [][]string {
	max := int64(len(arr))
	//判断数组大小是否小于等于指定分割大小的值，是则把原数组放入二维数组返回
	if max <= num {
		return [][]string{arr}
	}
	//获取应该数组分割为多少份
	var quantity int64
	if max%num == 0 {
		quantity = max / num
	} else {
		quantity = (max / num) + 1
	}
	//声明分割好的二维数组
	var segments = make([][]string, 0)
	//声明分割数组的截止下标
	var start, end, i int64
	for i = 1; i <= quantity; i++ {
		end = i*num
		if i != quantity {
			segments = append(segments, arr[start:end])
		} else {
			segments = append(segments, arr[start:])
		}
		start = i*num
	}
	return segments
}
