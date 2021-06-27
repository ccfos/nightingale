package ierr

import (
	"fmt"
)

type PageError struct {
	Message string
	Code    int
}

func (p PageError) Error() string {
	return p.Message
}

func (p PageError) String() string {
	return p.Message
}

func Bomb(code int, format string, a ...interface{}) {
	panic(PageError{Code: code, Message: fmt.Sprintf(format, a...)})
}

func Dangerous(v interface{}, code ...int) {
	if v == nil {
		return
	}

	c := 200
	if len(code) > 0 {
		c = code[0]
	}

	switch t := v.(type) {
	case string:
		if t != "" {
			panic(PageError{Code: c, Message: t})
		}
	case error:
		panic(PageError{Code: c, Message: t.Error()})
	}
}
