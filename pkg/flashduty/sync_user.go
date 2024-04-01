package flashduty

import (
	"errors"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/logger"
)

func SyncUsersChange(ctx *ctx.Context, dbUsers []*models.User) error {
	if !ctx.IsCenter {
		return nil
	}

	appKey, err := models.ConfigsGetFlashDutyAppKey(ctx)
	if err != nil {
		return err
	}

	req := make(map[string]interface{})
	req["limit"] = 100
	userList, err := PostFlashDutyWithResp("/member/list", appKey, req)
	if err != nil {
		return err
	}

	total := userList.Total
	items := []Item{}
	for i := 0; i < total/100+1; i++ {
		req["p"] = i
		req["limit"] = 100
		resp, err := PostFlashDutyWithResp("/member/list", appKey, req)
		if err != nil {
			return err
		}
		items = append(items, resp.Items...)
	}

	dutyUsers := make(map[string]*models.User, len(items))
	for i := range items {
		user := &models.User{
			Username: items[i].MemberName,
			Email:    items[i].Email,
			Phone:    items[i].Phone,
		}
		dutyUsers[user.Username+user.Email] = user
	}

	dbUsersHas := sliceToMap(dbUsers)

	delUsers := diffMap(dutyUsers, dbUsersHas)
	fdDelUsers(appKey, delUsers)

	addUsers := diffMap(dbUsersHas, dutyUsers)
	if err := fdAddUsers(appKey, addUsers); err != nil {
		return err
	}

	return nil
}

func sliceToMap(dbUsers []*models.User) map[string]*models.User {
	m := make(map[string]*models.User, len(dbUsers))
	for _, user := range dbUsers {
		m[user.Username+user.Email] = user
	}
	return m
}

// in m1 and not in m2
func diffMap(m1, m2 map[string]*models.User) []models.User {
	var diff []models.User
	for i := range m1 {
		if _, ok := m2[i]; !ok {
			diff = append(diff, *m1[i])
		}
	}
	return diff
}

type User struct {
	Email      string  `json:"email,omitempty"`
	Phone      string  `json:"phone,omitempty"`
	MemberName string  `json:"member_name,omitempty"`
	Updates    Updates `json:"updates,omitempty"`
}

type Updates struct {
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	MemberName  string `json:"member_name,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
}

func (user *User) delMember(appKey string) error {
	if user.Email == "" && user.Phone == "" {
		return errors.New("phones and email must be selected one of two")
	}
	return PostFlashDuty("/member/delete", appKey, user)
}

func (user *User) UpdateMember(ctx *ctx.Context) error {
	appKey, err := models.ConfigsGetFlashDutyAppKey(ctx)
	if err != nil {
		return err
	}

	return PostFlashDuty("/member/info/reset", appKey, user)
}

type Members struct {
	Users []User `json:"members"`
}

func (m *Members) addMembers(appKey string) error {
	if len(m.Users) == 0 {
		return nil
	}
	validUsers := make([]User, 0, len(m.Users))
	for _, user := range m.Users {
		if user.Email == "" && (user.Phone != "" && user.MemberName == "" || user.Phone == "") {
			logger.Errorf("user(%+v) phone and email must be selected one of two, and the member_name must be added when selecting the phone", user)
		} else {
			validUsers = append(validUsers, user)
		}
	}
	m.Users = validUsers
	return PostFlashDuty("/member/invite", appKey, m)
}

func fdAddUsers(appKey string, users []models.User) error {
	fdUsers := usersToFdUsers(users)
	members := &Members{
		Users: fdUsers,
	}
	return members.addMembers(appKey)
}

func fdDelUsers(appKey string, users []models.User) {
	fdUsers := usersToFdUsers(users)
	for _, fdUser := range fdUsers {
		if err := fdUser.delMember(appKey); err != nil {
			logger.Errorf("failed to delete user: %v", err)
		}
	}
}

func usersToFdUsers(users []models.User) []User {
	fdUsers := make([]User, 0, len(users))
	for i := range users {
		fdUsers = append(fdUsers, User{
			Email:      users[i].Email,
			Phone:      users[i].Phone,
			MemberName: users[i].Username,
		})
	}
	return fdUsers
}

func UpdateUser(ctx *ctx.Context, target models.User, email, phone string) {
	contact := target.FindSameContact(email, phone)
	var flashdutyUser User
	var needSync bool
	switch contact {
	case "email":
		flashdutyUser = User{
			Email: target.Email,
		}
		if target.Phone != phone {
			needSync = true
			flashdutyUser.Updates = Updates{
				Phone:       phone,
				MemberName:  target.Username,
				CountryCode: "CN",
			}
		}
	case "phone":
		flashdutyUser = User{
			Phone: target.Phone,
		}

		if target.Email != email {
			needSync = true
			flashdutyUser.Updates = Updates{
				Email:      email,
				MemberName: target.Username,
			}
		}
	default:
		flashdutyUser = User{
			MemberName: target.Username,
		}
		if target.Email != email {
			needSync = true
			flashdutyUser.Updates.Email = email
		}
		if target.Phone != phone {
			needSync = true
			flashdutyUser.Updates.Phone = phone
			flashdutyUser.Updates.CountryCode = "CN"
		}
	}

	if needSync {
		err := flashdutyUser.UpdateMember(ctx)
		if err != nil {
			logger.Errorf("failed to update user: %v", err)
		}
	}
}
