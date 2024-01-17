package flashduty

import (
	"errors"

	"github.com/toolkits/pkg/logger"
)

type User struct {
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	CountryCode string `json:"country_code"`
	MemberName  string `json:"member_name"`
	RoleIds     []int  `json:"role_ids"`
}

type Members struct {
	Users []User `json:"members"`
}

func AddUsers(appKey string, users []User) error {
	members := &Members{
		Users: users,
	}
	return members.addMembers(appKey)
}

func DelUsers(appKey string, users []User) {
	for _, user := range users {
		if err := user.delMember(appKey); err != nil {
			logger.Error("failed to delete user: %v", err)
		}
	}
}

func (m *Members) addMembers(appKey string) error {
	if len(m.Users) == 0 {
		return nil
	}
	validUsers := make([]User, 0, len(m.Users))
	for _, user := range m.Users {
		if user.Email == "" && (user.Phone == "" || user.MemberName == "") {
			logger.Error("phones and email must be selected one of two, and the member_name must be added when selecting the phone")
		} else {
			validUsers = append(validUsers, user)
		}
	}
	m.Users = validUsers
	_, _, err := PostFlashDuty("/member/invite", appKey, m)
	return err
}

func (user *User) delMember(appKey string) error {
	if user.Email == "" && user.Phone == "" {
		return errors.New("phones and email must be selected one of two")
	}
	_, _, err := PostFlashDuty("/member/delete", appKey, user)
	return err
}
