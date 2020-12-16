// +build gofuzz

package stalecucumber

import (
	"bytes"
)

func Fuzz(data []byte) int {
	if _, err := Unpickle(bytes.NewReader(data)); err != nil {
		return 0
	}
	return 1
}
