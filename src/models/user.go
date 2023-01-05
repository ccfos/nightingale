package models

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"

	"github.com/didi/nightingale/v5/src/pkg/ldapx"
	"github.com/didi/nightingale/v5/src/pkg/ormx"
	"github.com/didi/nightingale/v5/src/webapi/config"
)

type User struct {
	Id         int64        `json:"id" gorm:"primaryKey"`
	Username   string       `json:"username"`
	Nickname   string       `json:"nickname"`
	Password   string       `json:"-"`
	Phone      string       `json:"phone"`
	Email      string       `json:"email"`
	Portrait   string       `json:"portrait"`
	Roles      string       `json:"-"`              // 这个字段写入数据库
	RolesLst   []string     `json:"roles" gorm:"-"` // 这个字段和前端交互
	Contacts   ormx.JSONObj `json:"contacts"`       // 内容为 map[string]string 结构
	Maintainer int          `json:"maintainer"`     // 是否给管理员发消息 0:not send 1:send
	CreateAt   int64        `json:"create_at"`
	CreateBy   string       `json:"create_by"`
	UpdateAt   int64        `json:"update_at"`
	UpdateBy   string       `json:"update_by"`
	Admin      bool         `json:"admin" gorm:"-"` // 方便前端使用
}

func (u *User) TableName() string {
	return "users"
}

func (u *User) IsAdmin() bool {
	for i := 0; i < len(u.RolesLst); i++ {
		if u.RolesLst[i] == AdminRole {
			return true
		}
	}
	return false
}

func (u *User) Verify() error {
	u.Username = strings.TrimSpace(u.Username)

	if u.Username == "" {
		return errors.New("Username is blank")
	}

	if str.Dangerous(u.Username) {
		return errors.New("Username has invalid characters")
	}

	if str.Dangerous(u.Nickname) {
		return errors.New("Nickname has invalid characters")
	}

	if u.Phone != "" && !str.IsPhone(u.Phone) {
		return errors.New("Phone invalid")
	}

	if u.Email != "" && !str.IsMail(u.Email) {
		return errors.New("Email invalid")
	}

	return nil
}

func (u *User) Add() error {
	user, err := UserGetByUsername(u.Username)
	if err != nil {
		return errors.WithMessage(err, "failed to query user")
	}

	if user != nil {
		return errors.New("Username already exists")
	}

	now := time.Now().Unix()
	u.CreateAt = now
	u.UpdateAt = now
	return Insert(u)
}

func (u *User) Update(selectField interface{}, selectFields ...interface{}) error {
	if err := u.Verify(); err != nil {
		return err
	}

	return DB().Model(u).Select(selectField, selectFields...).Updates(u).Error
}

func (u *User) UpdateAllFields() error {
	if err := u.Verify(); err != nil {
		return err
	}

	u.UpdateAt = time.Now().Unix()
	return DB().Model(u).Select("*").Updates(u).Error
}

func (u *User) UpdatePassword(password, updateBy string) error {
	return DB().Model(u).Updates(map[string]interface{}{
		"password":  password,
		"update_at": time.Now().Unix(),
		"update_by": updateBy,
	}).Error
}

func (u *User) Del() error {
	return DB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id=?", u.Id).Delete(&UserGroupMember{}).Error; err != nil {
			return err
		}

		if err := tx.Where("id=?", u.Id).Delete(&User{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func (u *User) ChangePassword(oldpass, newpass string) error {
	_oldpass, err := CryptoPass(oldpass)
	if err != nil {
		return err
	}

	_newpass, err := CryptoPass(newpass)
	if err != nil {
		return err
	}

	if u.Password != _oldpass {
		return errors.New("Incorrect old password")
	}

	return u.UpdatePassword(_newpass, u.Username)
}

func UserGet(where string, args ...interface{}) (*User, error) {
	var lst []*User
	err := DB().Where(where, args...).Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 {
		return nil, nil
	}

	lst[0].RolesLst = strings.Fields(lst[0].Roles)
	lst[0].Admin = lst[0].IsAdmin()

	return lst[0], nil
}

func UserGetByUsername(username string) (*User, error) {
	return UserGet("username=?", username)
}

func UserGetById(id int64) (*User, error) {
	return UserGet("id=?", id)
}

func InitRoot() {
	user, err := UserGetByUsername("root")
	if err != nil {
		fmt.Println("failed to query user root:", err)
		os.Exit(1)
	}

	if user == nil {
		return
	}

	if len(user.Password) > 31 {
		// already done before
		return
	}

	newPass, err := CryptoPass(user.Password)
	if err != nil {
		fmt.Println("failed to crypto pass:", err)
		os.Exit(1)
	}

	err = DB().Model(user).Update("password", newPass).Error
	if err != nil {
		fmt.Println("failed to update root password:", err)
		os.Exit(1)
	}

	fmt.Println("root password init done")
}

func PassLogin(username, pass string) (*User, error) {
	user, err := UserGetByUsername(username)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return nil, fmt.Errorf("Username or password invalid")
	}

	loginPass, err := CryptoPass(pass)
	if err != nil {
		return nil, err
	}

	if loginPass != user.Password {
		return nil, fmt.Errorf("Username or password invalid")
	}

	return user, nil
}

func LdapLogin(username, pass string) (*User, error) {
	sr, err := ldapx.LdapReq(username, pass)
	if err != nil {
		return nil, err
	}

	user, err := UserGetByUsername(username)
	if err != nil {
		return nil, err
	}

	if user == nil {
		// default user settings
		user = &User{
			Username: username,
			Nickname: username,
		}
	}

	// copy attributes from ldap
	attrs := ldapx.LDAP.Attributes
	if attrs.Nickname != "" {
		user.Nickname = sr.Entries[0].GetAttributeValue(attrs.Nickname)
	}
	if attrs.Email != "" {
		user.Email = sr.Entries[0].GetAttributeValue(attrs.Email)
	}
	if attrs.Phone != "" {
		user.Phone = sr.Entries[0].GetAttributeValue(attrs.Phone)
	}

	if user.Id > 0 {
		if ldapx.LDAP.CoverAttributes {
			err := DB().Updates(user).Error
			if err != nil {
				return nil, errors.WithMessage(err, "failed to update user")
			}
		}
		return user, nil
	}

	now := time.Now().Unix()

	if len(config.C.LDAP.DefaultRoles) == 0 {
		config.C.LDAP.DefaultRoles = []string{"Standard"}
	}

	user.Password = "******"
	user.Portrait = ""
	user.Roles = strings.Join(config.C.LDAP.DefaultRoles, " ")
	user.RolesLst = config.C.LDAP.DefaultRoles
	user.Contacts = []byte("{}")
	user.CreateAt = now
	user.UpdateAt = now
	user.CreateBy = "ldap"
	user.UpdateBy = "ldap"

	err = DB().Create(user).Error
	return user, err
}

func UserTotal(query string) (num int64, err error) {
	if query != "" {
		q := "%" + query + "%"
		num, err = Count(DB().Model(&User{}).Where("username like ? or nickname like ? or phone like ? or email like ?", q, q, q, q))
	} else {
		num, err = Count(DB().Model(&User{}))
	}

	if err != nil {
		return num, errors.WithMessage(err, "failed to count user")
	}

	return num, nil
}

func UserGets(query string, limit, offset int) ([]User, error) {
	session := DB().Limit(limit).Offset(offset).Order("username")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("username like ? or nickname like ? or phone like ? or email like ?", q, q, q, q)
	}

	var users []User
	err := session.Find(&users).Error
	if err != nil {
		return users, errors.WithMessage(err, "failed to query user")
	}

	for i := 0; i < len(users); i++ {
		users[i].RolesLst = strings.Fields(users[i].Roles)
		users[i].Admin = users[i].IsAdmin()
	}

	return users, nil
}

func UserGetAll() ([]*User, error) {
	var lst []*User
	err := DB().Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].RolesLst = strings.Fields(lst[i].Roles)
			lst[i].Admin = lst[i].IsAdmin()
		}
	}
	return lst, err
}

func UserGetsByIds(ids []int64) ([]User, error) {
	if len(ids) == 0 {
		return []User{}, nil
	}

	var lst []User
	err := DB().Where("id in ?", ids).Order("username").Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].RolesLst = strings.Fields(lst[i].Roles)
			lst[i].Admin = lst[i].IsAdmin()
		}
	}

	return lst, err
}

func (u *User) CanModifyUserGroup(ug *UserGroup) (bool, error) {
	// 我是管理员，自然可以
	if u.IsAdmin() {
		return true, nil
	}

	// 我是创建者，自然可以
	if ug.CreateBy == u.Username {
		return true, nil
	}

	// 我是成员，也可以吧，简单搞
	num, err := UserGroupMemberCount("user_id=? and group_id=?", u.Id, ug.Id)
	if err != nil {
		return false, err
	}

	return num > 0, nil
}

func (u *User) CanDoBusiGroup(bg *BusiGroup, permFlag ...string) (bool, error) {
	if u.IsAdmin() {
		return true, nil
	}

	// 我在任意一个UserGroup里，就有权限
	ugids, err := UserGroupIdsOfBusiGroup(bg.Id, permFlag...)
	if err != nil {
		return false, err
	}

	if len(ugids) == 0 {
		return false, nil
	}

	num, err := UserGroupMemberCount("user_id = ? and group_id in ?", u.Id, ugids)
	return num > 0, err
}

func (u *User) CheckPerm(operation string) (bool, error) {
	if u.IsAdmin() {
		return true, nil
	}

	return RoleHasOperation(u.RolesLst, operation)
}

func UserStatistics() (*Statistics, error) {
	session := DB().Model(&User{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func (u *User) NopriIdents(idents []string) ([]string, error) {
	if u.IsAdmin() {
		return []string{}, nil
	}

	ugids, err := MyGroupIds(u.Id)
	if err != nil {
		return []string{}, err
	}

	if len(ugids) == 0 {
		return idents, nil
	}

	bgids, err := BusiGroupIds(ugids, "rw")
	if err != nil {
		return []string{}, err
	}

	if len(bgids) == 0 {
		return idents, nil
	}

	var arr []string
	err = DB().Model(&Target{}).Where("group_id in ?", bgids).Pluck("ident", &arr).Error
	if err != nil {
		return []string{}, err
	}

	return slice.SubString(idents, arr), nil
}

// 我是管理员，返回所有
// 或者我是成员
func (u *User) BusiGroups(limit int, query string, all ...bool) ([]BusiGroup, error) {
	session := DB().Order("name").Limit(limit)

	var lst []BusiGroup
	if u.IsAdmin() || (len(all) > 0 && all[0]) {
		err := session.Where("name like ?", "%"+query+"%").Find(&lst).Error
		if err != nil {
			return lst, err
		}

		if len(lst) == 0 && len(query) > 0 {
			// 隐藏功能，一般人不告诉，哈哈。query可能是给的ident，所以上面的sql没有查到，当做ident来查一下试试
			var t *Target
			t, err = TargetGet("ident=?", query)
			if err != nil {
				return lst, err
			}

			if t == nil {
				return lst, nil
			}

			err = DB().Order("name").Limit(limit).Where("id=?", t.GroupId).Find(&lst).Error
		}

		return lst, err
	}

	userGroupIds, err := MyGroupIds(u.Id)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get MyGroupIds")
	}

	busiGroupIds, err := BusiGroupIds(userGroupIds)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get BusiGroupIds")
	}

	if len(busiGroupIds) == 0 {
		return lst, nil
	}

	err = session.Where("id in ?", busiGroupIds).Where("name like ?", "%"+query+"%").Find(&lst).Error
	if err != nil {
		return nil, err
	}

	if len(lst) == 0 && len(query) > 0 {
		var t *Target
		t, err = TargetGet("ident=?", query)
		if err != nil {
			return lst, err
		}

		if slice.ContainsInt64(busiGroupIds, t.GroupId) {
			err = DB().Order("name").Limit(limit).Where("id=?", t.GroupId).Find(&lst).Error
		}
	}

	return lst, err
}

func (u *User) UserGroups(limit int, query string) ([]UserGroup, error) {
	session := DB().Order("name").Limit(limit)

	var lst []UserGroup
	if u.IsAdmin() {
		err := session.Where("name like ?", "%"+query+"%").Find(&lst).Error
		if err != nil {
			return lst, err
		}

		if len(lst) == 0 && len(query) > 0 {
			// 隐藏功能，一般人不告诉，哈哈。query可能是给的用户名，所以上面的sql没有查到，当做user来查一下试试
			user, err := UserGetByUsername(query)
			if user == nil {
				return lst, err
			}
			var ids []int64
			ids, err = MyGroupIds(user.Id)
			if err != nil || len(ids) == 0 {
				return lst, err
			}
			lst, err = UserGroupGetByIds(ids)
		}
		return lst, err
	}

	ids, err := MyGroupIds(u.Id)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get MyGroupIds")
	}

	if len(ids) > 0 {
		session = session.Where("id in ? or create_by = ?", ids, u.Username)
	} else {
		session = session.Where("create_by = ?", u.Username)
	}

	err = session.Where("name like ?", "%"+query+"%").Find(&lst).Error
	return lst, err
}
