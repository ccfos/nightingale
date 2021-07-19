package istr

import (
	"strconv"
	"strings"
)

func SampleKeyInvalid(str string) bool {
	idx := strings.IndexFunc(str, func(r rune) bool {
		return r == '\t' ||
			r == '\r' ||
			r == '\n' ||
			r == ',' ||
			r == ' ' ||
			r == '='
	})

	if idx != -1 {
		return true
	}

	_, err := strconv.ParseFloat(str, 64)
	return err == nil
}
