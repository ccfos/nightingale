package model

import "strings"

func Paths(longPath string) []string {
	names := strings.Split(longPath, ".")
	count := len(names)
	paths := make([]string, 0, count)

	for i := 1; i <= count; i++ {
		paths = append(paths, strings.Join(names[:i], "."))
	}

	return paths
}
