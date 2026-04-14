package ginx

import "github.com/toolkits/pkg/errorx"

func Bomb(code int, format string, a ...interface{}) {
	errorx.Bomb(code, format, a...)
}

func Dangerous(v interface{}, code ...int) {
	errorx.Dangerous(v, code...)
}
