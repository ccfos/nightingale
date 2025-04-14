package flashduty

import (
	"errors"
	"strconv"
	"strings"

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
	userList, err := PostFlashDutyWithResp[Data]("/member/list", appKey, req)
	if err != nil {
		return err
	}

	total := userList.Total
	items := []Item{}
	for i := 0; i < total/100+1; i++ {
		req["p"] = i
		req["limit"] = 100
		resp, err := PostFlashDutyWithResp[Data]("/member/list", appKey, req)
		if err != nil {
			return err
		}
		items = append(items, resp.Items...)
	}

	dutyUsers := make(map[int64]*models.User, len(items))
	for i := range items {
		if items[i].RefID != "" {
			id, _ := strconv.ParseInt(items[i].RefID, 10, 64)
			user := &models.User{
				Username: items[i].MemberName,
				Email:    items[i].Email,
				Phone:    items[i].Phone,
				Id:       id,
			}
			dutyUsers[id] = user
		}

	}

	dbUsersHas := sliceToMap(dbUsers)

	delUsers := diffMap(dutyUsers, dbUsersHas)
	fdDelUsers(appKey, delUsers)

	addUsers := diffMap(dbUsersHas, dutyUsers)
	if err := fdAddUsers(appKey, addUsers); err != nil {
		return err
	}
	updateUser(appKey, dbUsersHas, dutyUsers)
	return nil
}

func sliceToMap(dbUsers []*models.User) map[int64]*models.User {
	m := make(map[int64]*models.User, len(dbUsers))
	for _, user := range dbUsers {
		m[user.Id] = user
	}
	return m
}

// in m1 and not in m2
func diffMap(m1, m2 map[int64]*models.User) []models.User {
	var diff []models.User
	for i := range m1 {
		if _, ok := m2[i]; !ok {
			diff = append(diff, *m1[i])
		}
	}
	return diff
}
func updateUser(appKey string, m1, m2 map[int64]*models.User) {
	for i := range m1 {
		if _, ok := m2[i]; ok {
			if m1[i].Email != m2[i].Email || m1[i].Phone != m2[i].Phone || m1[i].Username != m2[i].Username {
				var flashdutyUser User

				flashdutyUser = User{
					RefID: strconv.FormatInt(m1[i].Id, 10),
				}
				flashdutyUser.Updates = Updates{
					Phone:      m1[i].Phone,
					Email:      m1[i].Email,
					MemberName: m1[i].Username,
					RefID:      strconv.FormatInt(m1[i].Id, 10),
				}
				err := flashdutyUser.UpdateMember(appKey)
				if err != nil {
					logger.Errorf("failed to update user: %v", err)
				}
			}
		}
	}
}

type User struct {
	Email      string  `json:"email,omitempty"`
	Phone      string  `json:"phone,omitempty"`
	MemberName string  `json:"member_name,omitempty"`
	RefID      string  `json:"ref_id,omitempty"`
	Updates    Updates `json:"updates,omitempty"`
}

type Updates struct {
	RefID       string `json:"ref_id,omitempty"`
	Email       string `json:"email,omitempty"`
	Phone       string `json:"phone,omitempty"`
	MemberName  string `json:"member_name,omitempty"`
	CountryCode string `json:"country_code,omitempty"`
}

func (user *User) delMember(appKey string) error {
	if user.RefID == "" {
		return errors.New("refID must not be empty")
	}
	userDel := &User{RefID: user.RefID}
	return PostFlashDuty("/member/delete", appKey, userDel)
}

func (user *User) UpdateMember(appKey string) error {

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
		if user.RefID == "" || (user.Phone == "" && user.Email == "") {
			logger.Errorf("user(%+v) refID must not be none, Email or Phone can not be none", user)
		} else {
			validUsers = append(validUsers, user)
		}
	}
	if len(validUsers) == 0 {
		return nil
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
			RefID:      strconv.FormatInt(users[i].Id, 10),
			Phone:      users[i].Phone,
			Email:      users[i].Email,
			MemberName: users[i].Username,
		})

	}
	return fdUsers
}

func UpdateUser(ctx *ctx.Context, target models.User, email, phone string) {
	//contact := target.FindSameContact(email, phone)
	if target.Id == 0 {
		logger.Errorf("user not found: %s", target.Username)
		return
	}
	if email == "" && phone == "" {
		logger.Errorf("email and phone are both empty: %s", target.Username)
		return
	}
	var flashdutyUser User
	refID := strconv.FormatInt(target.Id, 10)

	flashdutyUser = User{
		RefID: refID,
	}
	flashdutyUser.Updates = Updates{
		Phone:      phone,
		Email:      email,
		MemberName: target.Username,
		RefID:      refID,
	}
	appKey, err := models.ConfigsGetFlashDutyAppKey(ctx)
	if err != nil {
		logger.Errorf("failed to get flashduty app key: %v", err)
		return
	}
	err = flashdutyUser.UpdateMember(appKey)
	if err != nil && strings.Contains(err.Error(), "no member found") {
		// 如果没有找到成员，说明需要新建成员
		NewUser := &User{
			Phone:      phone,
			Email:      email,
			MemberName: target.Username,
			RefID:      refID,
		}
		err = PostFlashDuty("/member/invite", appKey, NewUser)
		if err != nil {
			logger.Errorf("failed to update user: %v", err)
		}
		return

	}
	if err != nil {
		logger.Errorf("failed to update user: %v", err)
	}
}
