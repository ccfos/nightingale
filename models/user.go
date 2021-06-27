package models

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
	"xorm.io/builder"

	"github.com/didi/nightingale/v5/pkg/ierr"
)

type User struct {
	Id       int64           `json:"id"`
	Username string          `json:"username"`
	Nickname string          `json:"nickname"`
	Password string          `json:"-"`
	Phone    string          `json:"phone"`
	Email    string          `json:"email"`
	Portrait string          `json:"portrait"`
	Status   int             `json:"status"`
	Role     string          `json:"role"`
	Contacts json.RawMessage `json:"contacts"` //内容为 map[string]string 结构
	CreateAt int64           `json:"create_at"`
	CreateBy string          `json:"create_by"`
	UpdateAt int64           `json:"update_at"`
	UpdateBy string          `json:"update_by"`
}

func (u *User) TableName() string {
	return "user"
}

func (u *User) Validate() error {
	u.Username = strings.TrimSpace(u.Username)

	if u.Username == "" {
		return _e("Username is blank")
	}

	if str.Dangerous(u.Username) {
		return _e("Username has invalid characters")
	}

	if str.Dangerous(u.Nickname) {
		return _e("Nickname has invalid characters")
	}

	if u.Phone != "" && !str.IsPhone(u.Phone) {
		return _e("Phone invalid")
	}

	if u.Email != "" && !str.IsMail(u.Email) {
		return _e("Email invalid")
	}

	return nil
}

func (u *User) Update(cols ...string) error {
	if err := u.Validate(); err != nil {
		return err
	}

	_, err := DB.Where("id=?", u.Id).Cols(cols...).Update(u)
	if err != nil {
		logger.Errorf("mysql.error: update user fail: %v", err)
		return internalServerError
	}

	return nil
}

func (u *User) Add() error {
	num, err := DB.Where("username=?", u.Username).Count(new(User))
	if err != nil {
		logger.Errorf("mysql.error: count user(%s) fail: %v", u.Username, err)
		return internalServerError
	}

	if num > 0 {
		return _e("Username %s already exists", u.Username)
	}

	return DBInsertOne(u)
}

func InitRoot() {
	var u User
	has, err := DB.Where("username=?", "root").Get(&u)
	if err != nil {
		fmt.Println("fatal: cannot query user root,", err)
		os.Exit(1)
	}

	if has {
		return
	}

	pass, err := CryptoPass("root.2020")
	if err != nil {
		fmt.Println("fatal: cannot crypto password,", err)
		os.Exit(1)
	}

	now := time.Now().Unix()

	u = User{
		Username: "root",
		Password: pass,
		Nickname: "超管",
		Portrait: "",
		Role:     "Admin",
		Contacts: []byte("{}"),
		CreateAt: now,
		UpdateAt: now,
		CreateBy: "system",
		UpdateBy: "system",
	}

	_, err = DB.Insert(u)
	if err != nil {
		fmt.Println("fatal: cannot insert user root", err)
		os.Exit(1)
	}

	fmt.Println("user root init done")
}

func UserGetByUsername(username string) (*User, error) {
	return UserGet("username=?", username)
}

func UserGetById(id int64) (*User, error) {
	return UserGet("id=?", id)
}

func UserGet(where string, args ...interface{}) (*User, error) {
	var obj User
	has, err := DB.Where(where, args...).Get(&obj)
	if err != nil {
		logger.Errorf("mysql.error: query user(%s)%+v fail: %s", where, args, err)
		return nil, internalServerError
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func UserTotal(query string) (num int64, err error) {
	if query != "" {
		q := "%" + query + "%"
		num, err = DB.Where("username like ? or nickname like ? or phone like ? or email like ?", q, q, q, q).Count(new(User))
	} else {
		num, err = DB.Count(new(User))
	}

	if err != nil {
		logger.Errorf("mysql.error: count user(query: %s) fail: %v", query, err)
		return num, internalServerError
	}

	return num, nil
}

func UserGets(query string, limit, offset int) ([]User, error) {
	session := DB.Limit(limit, offset).OrderBy("username")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("username like ? or nickname like ? or phone like ? or email like ?", q, q, q, q)
	}

	var users []User
	err := session.Find(&users)
	if err != nil {
		logger.Errorf("mysql.error: select user(query: %s) fail: %v", query, err)
		return users, internalServerError
	}

	if len(users) == 0 {
		return []User{}, nil
	}

	return users, nil
}

func UserGetAll() ([]User, error) {
	var users []User

	err := DB.Find(&users)
	if err != nil {
		logger.Errorf("mysql.error: select user fail: %v", err)
		return users, internalServerError
	}

	if len(users) == 0 {
		return []User{}, nil
	}

	return users, nil
}

func UserGetsByIds(ids []int64) ([]User, error) {
	if len(ids) == 0 {
		return []User{}, nil
	}

	var users []User
	err := DB.In("id", ids).OrderBy("username").Find(&users)
	if err != nil {
		logger.Errorf("mysql.error: query users by ids fail: %v", err)
		return users, internalServerError
	}

	if len(users) == 0 {
		return []User{}, nil
	}

	return users, nil
}

func UserGetsByIdsStr(ids []string) ([]User, error) {
	var objs []User

	err := DB.Where("id in (" + strings.Join(ids, ",") + ")").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: UserGetsByIds fail: %v", err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []User{}, nil
	}

	return objs, nil
}

func PassLogin(username, pass string) (*User, error) {
	user, err := UserGetByUsername(username)
	if err != nil {
		return nil, err
	}

	if user == nil {
		logger.Infof("password auth fail, no such user: %s", username)
		return nil, loginFailError
	}

	loginPass, err := CryptoPass(pass)
	if err != nil {
		return nil, internalServerError
	}

	if loginPass != user.Password {
		logger.Infof("password auth fail, password error, user: %s", username)
		return nil, loginFailError
	}

	return user, nil
}

func LdapLogin(username, pass string) (*User, error) {
	sr, err := ldapReq(username, pass)
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
	attrs := LDAP.Attributes
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
		if LDAP.CoverAttributes {
			_, err := DB.Where("id=?", user.Id).Update(user)
			if err != nil {
				logger.Errorf("mysql.error: update user %+v fail: %v", user, err)
				return nil, internalServerError
			}
		}
		return user, nil
	}

	now := time.Now().Unix()

	user.Password = "******"
	user.Portrait = "/img/linux.jpeg"
	user.Role = "Standard"
	user.Contacts = []byte("{}")
	user.CreateAt = now
	user.UpdateAt = now
	user.CreateBy = "ldap"
	user.UpdateBy = "ldap"

	err = DBInsertOne(user)
	return user, err
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
		return _e("Incorrect old password")
	}

	u.Password = _newpass
	return u.Update("password")
}

func (u *User) _del() error {
	session := DB.NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM user_token WHERE user_id=?", u.Id); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM user_group_member WHERE user_id=?", u.Id); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM classpath_favorite WHERE user_id=?", u.Id); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM alert_rule_group_favorite WHERE user_id=?", u.Id); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM user WHERE id=?", u.Id); err != nil {
		return err
	}

	return session.Commit()
}

func (u *User) Del() error {
	err := u._del()
	if err != nil {
		logger.Errorf("mysql.error: delete user(%d, %s) fail: %v", u.Id, u.Username, err)
		return internalServerError
	}
	return nil
}

func (u *User) FavoriteClasspathIds() ([]int64, error) {
	return ClasspathFavoriteGetClasspathIds(u.Id)
}

func (u *User) FavoriteAlertRuleGroupIds() ([]int64, error) {
	return AlertRuleGroupFavoriteGetGroupIds(u.Id)
}

func (u *User) FavoriteDashboardIds() ([]int64, error) {
	return DashboardFavoriteGetDashboardIds(u.Id)
}

// UserGroupIds 我是成员的用户组ID列表
func (u *User) UserGroupIds() ([]int64, error) {
	var ids []int64
	err := DB.Table(new(UserGroupMember)).Select("group_id").Where("user_id=?", u.Id).Find(&ids)
	if err != nil {
		logger.Errorf("mysql.error: query user_group_member fail: %v", err)
		return ids, internalServerError
	}

	return ids, nil
}

func (u *User) FavoriteClasspaths() ([]Classpath, error) {
	ids, err := u.FavoriteClasspathIds()
	if err != nil {
		return nil, err
	}

	var objs []Classpath
	err = DB.In("id", ids).OrderBy("path").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query my classpath fail: %v", err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []Classpath{}, nil
	}

	return objs, nil
}

func (u *User) FavoriteAlertRuleGroups() ([]AlertRuleGroup, error) {
	ids, err := u.FavoriteAlertRuleGroupIds()
	if err != nil {
		return nil, err
	}

	var objs []AlertRuleGroup
	err = DB.In("id", ids).OrderBy("name").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query my alert_rule_group fail: %v", err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []AlertRuleGroup{}, nil
	}

	return objs, nil
}

func (u *User) MyUserGroups() ([]UserGroup, error) {
	cond := builder.NewCond()
	cond = cond.And(builder.Eq{"create_by": u.Username})

	ids, err := u.UserGroupIds()
	if err != nil {
		return nil, err
	}

	if len(ids) > 0 {
		cond = cond.Or(builder.In("id", ids))
	}

	var objs []UserGroup
	err = DB.Where(cond).OrderBy("name").Find(&objs)
	if err != nil {
		logger.Errorf("mysql.error: query my user_group fail: %v", err)
		return nil, internalServerError
	}

	if len(objs) == 0 {
		return []UserGroup{}, nil
	}

	return objs, nil
}

func (u *User) CanModifyUserGroup(ug *UserGroup) (bool, error) {
	// 我是管理员，自然可以
	if u.Role == "Admin" {
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

func (u *User) CanDo(op string) (bool, error) {
	if u.Role == "Admin" {
		return true, nil
	}

	return RoleHasOperation(u.Role, op)
}

// MustPerm return *User for link program
func (u *User) MustPerm(op string) *User {
	can, err := u.CanDo(op)
	ierr.Dangerous(err, 500)

	if !can {
		ierr.Bomb(403, "forbidden")
	}

	return u
}
