package router

import (
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

func maskPhone(phone string) string {
	countryCode := ""
	if strings.HasPrefix(phone, "+") {
		for size := range 3 {
			end := size + 2
			if len(phone) < end {
				break
			}
			if strings.Contains(internationalCallingCodes, "|"+phone[1:end]+"|") {
				countryCode = phone[:end]
				phone = phone[end:]
				break
			}
		}
	}

	keepStart := 2
	if countryCode == "" || countryCode == "+86" {
		keepStart = 3
	}
	chars := []rune(phone)
	if len(chars) <= keepStart+2 {
		return countryCode + strings.Repeat("*", len(chars))
	}
	return countryCode + string(chars[:keepStart]) + strings.Repeat("*", len(chars)-keepStart-2) + string(chars[len(chars)-2:])
}

func userForResponse(user *models.User) *models.User {
	if user == nil {
		return nil
	}
	result := *user
	result.Phone = maskPhone(user.Phone)
	return &result
}
