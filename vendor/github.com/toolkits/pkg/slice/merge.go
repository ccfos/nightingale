package slice

// SliceMerge merges interface slices to one slice.
func Merge(slice1, slice2 []interface{}) (c []interface{}) {
	c = append(slice1, slice2...)
	return
}

func MergeInt(slice1, slice2 []int) (c []int) {
	c = append(slice1, slice2...)
	return
}

func MergeInt64(slice1, slice2 []int64) (c []int64) {
	c = append(slice1, slice2...)
	return
}

func MergeString(slice1, slice2 []string) (c []string) {
	c = append(slice1, slice2...)
	return
}
