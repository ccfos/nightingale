package slice

func HaveIntersection[T comparable](slice1, slice2 []T) bool {
	elemMap := make(map[T]bool)
	for _, val := range slice1 {
		elemMap[val] = true
	}

	for _, val := range slice2 {
		if elemMap[val] {
			return true
		}
	}
	return false
}
