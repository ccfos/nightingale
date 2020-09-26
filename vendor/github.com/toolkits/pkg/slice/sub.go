package slice

// Sub: a-b=?
func SubInt64(a, b []int64) []int64 {
	blen := len(b)
	if blen == 0 {
		return a
	}

	alen := len(a)
	if alen == 0 {
		return a
	}

	bset := make(map[int64]struct{}, blen)
	for i := 0; i < blen; i++ {
		bset[b[i]] = struct{}{}
	}

	ret := make([]int64, 0, alen)
	for i := 0; i < alen; i++ {
		if _, has := bset[a[i]]; !has {
			ret = append(ret, a[i])
		}
	}

	return ret
}

func SubString(a, b []string) []string {
	blen := len(b)
	if blen == 0 {
		return a
	}

	alen := len(a)
	if alen == 0 {
		return a
	}

	bset := make(map[string]struct{}, blen)
	for i := 0; i < blen; i++ {
		bset[b[i]] = struct{}{}
	}

	ret := make([]string, 0, alen)
	for i := 0; i < alen; i++ {
		if _, has := bset[a[i]]; !has {
			ret = append(ret, a[i])
		}
	}

	return ret
}
