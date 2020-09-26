package str

import (
	"strings"
)

func ToENSymbol(raw string) string {
	raw = strings.Replace(raw, "，", ",", -1)
	raw = strings.Replace(raw, "（", "(", -1)
	raw = strings.Replace(raw, "）", ")", -1)
	raw = strings.Replace(raw, "：", ":", -1)
	raw = strings.Replace(raw, "。", ".", -1)
	return raw
}
