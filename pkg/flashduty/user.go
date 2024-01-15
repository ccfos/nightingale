package flashduty

import (
	"errors"
	"github.com/ccfos/nightingale/v6/pkg/poster"
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

func (m *Members) AddMembers(appKey string) error {
	if len(m.Users) == 0 {
		return nil
	}
	for _, user := range m.Users {
		if user.Email == "" && (user.Phone == "" || user.MemberName == "") {
			return errors.New("phones and email must be selected one of two, and the member_name must be added when selecting the phone")
		}
	}
	_, _, err := poster.PostFlashDuty("/member/invite", appKey, m)
	return err
}

func (user *User) DelMember(appKey string) error {
	if user.Email == "" && user.Phone == "" {
		return errors.New("phones and email must be selected one of two")
	}
	_, _, err := poster.PostFlashDuty("/member/delete", appKey, user)
	return err
}
