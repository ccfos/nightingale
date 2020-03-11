package slice

func Contains(sl []interface{}, v interface{}) bool {
	for _, vv := range sl {
		if vv == v {
			return true
		}
	}
	return false
}

func ContainsInt(sl []int, v int) bool {
	for _, vv := range sl {
		if vv == v {
			return true
		}
	}
	return false
}

func ContainsInt64(sl []int64, v int64) bool {
	for _, vv := range sl {
		if vv == v {
			return true
		}
	}
	return false
}

func ContainsString(sl []string, v string) bool {
	for _, vv := range sl {
		if vv == v {
			return true
		}
	}
	return false
}


func ContainsSlice(smallSlice, bigSlice []string) bool {
	for i := 0; i < len(smallSlice); i++ {
		if !ContainsString(bigSlice, smallSlice[i]) {
			return false
		}
	}

	return true
}
