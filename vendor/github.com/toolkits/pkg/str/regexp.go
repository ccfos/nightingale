package str

import (
	"regexp"
	"strings"
)

var IPReg, _ = regexp.Compile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)
var MailReg, _ = regexp.Compile(`\w[-._\w]*@\w[-._\w]*\.\w+`)

func IsMatch(s, pattern string) bool {
	match, err := regexp.Match(pattern, []byte(s))
	if err != nil {
		return false
	}

	return match
}

func IsIdentifier(s string, pattern ...string) bool {
	defpattern := "^[a-zA-Z0-9\\-\\_\\.]+$"
	if len(pattern) > 0 {
		defpattern = pattern[0]
	}

	return IsMatch(s, defpattern)
}

func IsMail(s string) bool {
	return MailReg.MatchString(s)
}

func IsPhone(s string) bool {
	if strings.HasPrefix(s, "+") {
		return IsMatch(s[1:], `^\d{13}$`)
	} else {
		return IsMatch(s, `^\d{11}$`)
	}
}

func IsIP(s string) bool {
	return IPReg.MatchString(s)
}

func Dangerous(s string) bool {
	if strings.Contains(s, "<") {
		return true
	}

	if strings.Contains(s, ">") {
		return true
	}

	if strings.Contains(s, "&") {
		return true
	}

	if strings.Contains(s, "'") {
		return true
	}

	if strings.Contains(s, "\"") {
		return true
	}

	if strings.Contains(s, "file://") {
		return true
	}

	if strings.Contains(s, "../") {
		return true
	}

	return false
}
