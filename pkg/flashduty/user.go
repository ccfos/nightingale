package flashduty

import (
	"errors"
	"github.com/ccfos/nightingale/v6/center/cconf"
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

func AddUsers(fdConf *cconf.FlashDuty, appKey string, users []User) error {
	members := &Members{
		Users: users,
	}
	return members.AddMembers(fdConf, appKey)
}

func DelUsers(fdConf *cconf.FlashDuty, appKey string, users []User) {
	for _, user := range users {
		if err := user.DelMember(fdConf, appKey); err != nil {
			logger.Error("failed to delete user: %v", err)
		}
	}
}

func (m *Members) AddMembers(fdConf *cconf.FlashDuty, appKey string) error {
	if len(m.Users) == 0 {
		return nil
	}
	for i, user := range m.Users {
		if user.Email == "" && (user.Phone == "" || user.MemberName == "") {
			logger.Error("phones and email must be selected one of two, and the member_name must be added when selecting the phone")
			m.Users = append(m.Users[:i], m.Users[i+1:]...)
		}
	}
	_, _, err := PostFlashDuty(fdConf.Api, "/member/invite", fdConf.Timeout, appKey, m)
	return err
}

func (user *User) DelMember(fdConf *cconf.FlashDuty, appKey string) error {
	if user.Email == "" && user.Phone == "" {
		return errors.New("phones and email must be selected one of two")
	}
	_, _, err := PostFlashDuty(fdConf.Api, "/member/delete", fdConf.Timeout, appKey, user)
	return err
}
