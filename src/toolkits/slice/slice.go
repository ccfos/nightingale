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
