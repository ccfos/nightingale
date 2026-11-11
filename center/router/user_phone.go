package router

import (
	"strings"

	"github.com/ccfos/nightingale/v6/models"
)

func maskPhone(phone string) string {
	chars := []rune(phone)
	if len(chars) <= 7 {
		return strings.Repeat("*", len(chars))
	}
	return string(chars[:3]) + strings.Repeat("*", len(chars)-7) + string(chars[len(chars)-4:])
}

func userForResponse(user *models.User) *models.User {
	if user == nil {
		return nil
	}
	result := *user
	result.Phone = maskPhone(user.Phone)
	return &result
}
