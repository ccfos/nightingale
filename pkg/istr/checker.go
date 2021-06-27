package istr

import (
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
	return idx != -1
}
