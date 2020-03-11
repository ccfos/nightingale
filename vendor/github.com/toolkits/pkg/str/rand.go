package str

import (
	"fmt"
	"math/rand"
)

var letters = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
var digits = []rune("0123456789")

const size = 62

func RandLetters(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(size)]
	}

	return fmt.Sprintf("%s", string(b))
}

func RandDigits(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = digits[rand.Intn(10)]
	}

	return fmt.Sprintf("%s", string(b))
}
