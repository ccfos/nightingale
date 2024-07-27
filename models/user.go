package models

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/ccfos/nightingale/v6/pkg/poster"
	"github.com/ccfos/nightingale/v6/storage"
	"github.com/redis/go-redis/v9"

	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

const (
	Dingtalk     = "dingtalk"
	Wecom        = "wecom"
	Feishu       = "feishu"
	FeishuCard   = "feishucard"
	Mm           = "mm"
	Telegram     = "telegram"
	Email        = "email"
	EmailSubject = "mailsubject"

	DingtalkKey = "dingtalk_robot_token"
	WecomKey    = "wecom_robot_token"
	FeishuKey   = "feishu_robot_token"
	MmKey       = "mm_webhook_url"
	TelegramKey = "telegram_robot_token"

	DingtalkDomain = "oapi.dingtalk.com"
	WecomDomain    = "qyapi.weixin.qq.com"
	FeishuDomain   = "open.feishu.cn"

	// FeishuCardDomain The domain name of the feishu card is the same as the feishu,distinguished by the parameter
	FeishuCardDomain = "open.feishu.cn?card=1"
	TelegramDomain   = "api.telegram.org"
	IbexDomain       = "ibex"
	DefaultDomain    = "default"
)

var (
	DefaultChannels = []string{Dingtalk, Wecom, Feishu, Mm, Telegram, Email, FeishuCard}
)

type User struct {
	Id             int64           `json:"id" gorm:"primaryKey"`
	Username       string          `json:"username"`
	Nickname       string          `json:"nickname"`
	Password       string          `json:"-"`
	Phone          string          `json:"phone"`
	Email          string          `json:"email"`
	Portrait       string          `json:"portrait"`
	Roles          string          `json:"-"`              // 这个字段写入数据库
	RolesLst       []string        `json:"roles" gorm:"-"` // 这个字段和前端交互
	TeamsLst       []int64         `json:"-" gorm:"-"`     // 这个字段方便映射团队，前端和数据库都不用到
	Contacts       ormx.JSONObj    `json:"contacts"`       // 内容为 map[string]string 结构
	Maintainer     int             `json:"maintainer"`     // 是否给管理员发消息 0:not send 1:send
	CreateAt       int64           `json:"create_at"`
	CreateBy       string          `json:"create_by"`
	UpdateAt       int64           `json:"update_at"`
	UpdateBy       string          `json:"update_by"`
	Belong         string          `json:"belong"`
	Admin          bool            `json:"admin" gorm:"-"` // 方便前端使用
	UserGroupsRes  []*UserGroupRes `json:"user_groups" gorm:"-"`
	BusiGroupsRes  []*BusiGroupRes `json:"busi_groups" gorm:"-"`
	LastActiveTime int64           `json:"last_active_time"`
}

type UserGroupRes struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

type BusiGroupRes struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
}

func (u *User) TableName() string {
	return "users"
}

func (u *User) String() string {
	bs, err := u.Contacts.MarshalJSON()
	if err != nil {
		return err.Error()
	}

	return fmt.Sprintf("<id:%d username:%s nickname:%s email:%s phone:%s contacts:%s>", u.Id, u.Username, u.Nickname, u.Email, u.Phone, string(bs))
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

func (u *User) UpdateSsoFields(sso string, nickname, phone, email string) []interface{} {
	u.UpdateAt = time.Now().Unix()

	if nickname != "" {
		u.Nickname = nickname
	}
	if phone != "" {
		u.Phone = phone
	}
	if email != "" {
		u.Email = email
	}
	u.UpdateBy = sso
	u.Belong = sso

	updatedFields := []interface{}{"nickname", "phone", "email", "update_by", "belong"}
	return updatedFields
}

func (u *User) UpdateSsoFieldsWithRoles(sso string, nickname, phone, email string, roles []string) []interface{} {
	updatedFields := u.UpdateSsoFields(sso, nickname, phone, email)

	if len(roles) == 0 {
		return updatedFields
	}

	u.Roles = strings.Join(roles, " ")
	u.RolesLst = roles

	return append(updatedFields, "roles")
}

func (u *User) FullSsoFields(sso, username, nickname, phone, email string, defaultRoles []string) {
	now := time.Now().Unix()

	u.Username = username
	u.Password = "******"
	u.Nickname = nickname
	u.Phone = phone
	u.Email = email
	u.Portrait = ""
	u.Roles = strings.Join(defaultRoles, " ")
	u.RolesLst = defaultRoles
	u.Contacts = []byte("{}")
	u.CreateAt = now
	u.UpdateAt = now
	u.CreateBy = sso
	u.UpdateBy = sso
	u.Belong = sso
}

func (u *User) FullSsoFieldsWithTeams(sso, username, nickname, phone, email string, defaultRoles []string,
	teams []int64) {
	u.FullSsoFields(sso, username, nickname, phone, email, defaultRoles)
	u.TeamsLst = teams
}

func (u *User) Add(ctx *ctx.Context) error {
	user, err := UserGetByUsername(ctx, u.Username)
	if err != nil {
		return errors.WithMessage(err, "failed to query user")
	}

	if user != nil {
		return errors.New("Username already exists")
	}

	now := time.Now().Unix()
	u.CreateAt = now
	u.UpdateAt = now
	return Insert(ctx, u)
}

func (u *User) Update(ctx *ctx.Context, selectField interface{}, selectFields ...interface{}) error {
	if u.Belong == "" {
		if err := u.Verify(); err != nil {
			return err
		}
	}

	return DB(ctx).Model(u).Select(selectField, selectFields...).Updates(u).Error
}

func (u *User) UpdateAllFields(ctx *ctx.Context) error {
	if err := u.Verify(); err != nil {
		return err
	}

	u.UpdateAt = time.Now().Unix()
	return DB(ctx).Model(u).Select("*").Updates(u).Error
}

func (u *User) UpdatePassword(ctx *ctx.Context, password, updateBy string) error {
	return DB(ctx).Model(u).Updates(map[string]interface{}{
		"password":  password,
		"update_at": time.Now().Unix(),
		"update_by": updateBy,
	}).Error
}

func UpdateUserLastActiveTime(ctx *ctx.Context, userId int64, lastActiveTime int64) error {
	return DB(ctx).Model(&User{}).Where("id = ?", userId).Updates(map[string]interface{}{
		"last_active_time": lastActiveTime,
		"update_at":        time.Now().Unix(),
	}).Error
}

func (u *User) Del(ctx *ctx.Context) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id=?", u.Id).Delete(&UserGroupMember{}).Error; err != nil {
			return err
		}

		if err := tx.Where("id=?", u.Id).Delete(&User{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func (u *User) ChangePassword(ctx *ctx.Context, oldpass, newpass string) error {
	_oldpass, err := CryptoPass(ctx, oldpass)
	if err != nil {
		return err
	}

	_newpass, err := CryptoPass(ctx, newpass)
	if err != nil {
		return err
	}

	if u.Password != _oldpass {
		return errors.New("Incorrect old password")
	}

	return u.UpdatePassword(ctx, _newpass, u.Username)
}

func UserGet(ctx *ctx.Context, where string, args ...interface{}) (*User, error) {
	var lst []*User
	err := DB(ctx).Where(where, args...).Find(&lst).Error
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

func UserGetByUsername(ctx *ctx.Context, username string) (*User, error) {
	return UserGet(ctx, "username=?", username)
}

func UserGetById(ctx *ctx.Context, id int64) (*User, error) {
	return UserGet(ctx, "id=?", id)
}

func UsersGetByGroupIds(ctx *ctx.Context, groupIds []int64) ([]User, error) {
	if len(groupIds) == 0 {
		return nil, nil
	}

	userIds, err := GroupsMemberIds(ctx, groupIds)
	if err != nil {
		return nil, err
	}
	users, err := UserGetsByIds(ctx, userIds)
	if err != nil {
		return nil, err
	}
	return users, nil
}

func InitRoot(ctx *ctx.Context) {
	user, err := UserGetByUsername(ctx, "root")
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

	newPass, err := CryptoPass(ctx, user.Password)
	if err != nil {
		fmt.Println("failed to crypto pass:", err)
		os.Exit(1)
	}

	err = DB(ctx).Model(user).Update("password", newPass).Error
	if err != nil {
		fmt.Println("failed to update root password:", err)
		os.Exit(1)
	}

	fmt.Println("root password init done")
}

func reachLoginFailCount(ctx *ctx.Context, redisObj storage.Redis, username string, count int64) (bool, error) {
	key := "/userlogin/errorcount/" + username
	val, err := redisObj.Get(ctx.GetContext(), key).Result()
	if err == redis.Nil {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	c, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return false, err
	}

	return c >= count, nil
}

func incrLoginFailCount(ctx *ctx.Context, redisObj storage.Redis, username string, seconds int64) {
	key := "/userlogin/errorcount/" + username
	duration := time.Duration(seconds) * time.Second

	val, err := redisObj.Get(ctx.GetContext(), key).Result()
	if err == redis.Nil {
		redisObj.Set(ctx.GetContext(), key, "1", duration)
		return
	}

	if err != nil {
		logger.Warningf("login_fail_count: failed to get redis value. key:%s, error:%s", key, err)
		redisObj.Set(ctx.GetContext(), key, "1", duration)
		return
	}

	count, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		logger.Warningf("login_fail_count: failed to parse int64. key:%s, error:%s", key, err)
		redisObj.Set(ctx.GetContext(), key, "1", duration)
		return
	}

	count++
	redisObj.Set(ctx.GetContext(), key, fmt.Sprintf("%d", count), duration)
}

func PassLogin(ctx *ctx.Context, redis storage.Redis, username, pass string) (*User, error) {
	// 300 5 meaning: 300 seconds, 5 times
	val, err := ConfigsGet(ctx, "login_fail_count")
	if err != nil {
		return nil, err
	}

	var (
		needCheck = val != "" // DB 里有配置，说明启用了这个 feature
		seconds   int64
		count     int64
	)

	if needCheck {
		pair := strings.Fields(val)
		if len(pair) != 2 {
			logger.Warningf("login_fail_count config invalid: %s", val)
			needCheck = false
		} else {
			seconds, err = strconv.ParseInt(pair[0], 10, 64)
			if err != nil {
				logger.Warningf("login_fail_count seconds invalid: %s", pair[0])
				needCheck = false
			}

			count, err = strconv.ParseInt(pair[1], 10, 64)
			if err != nil {
				logger.Warningf("login_fail_count count invalid: %s", pair[1])
				needCheck = false
			}
		}
	}

	if needCheck {
		reach, err := reachLoginFailCount(ctx, redis, username, count)
		if err != nil {
			return nil, err
		}

		if reach {
			return nil, fmt.Errorf("reach login fail count")
		}
	}

	user, err := UserGetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	if user == nil {
		if needCheck {
			incrLoginFailCount(ctx, redis, username, seconds)
		}

		return nil, fmt.Errorf("Username or password invalid")
	}

	loginPass, err := CryptoPass(ctx, pass)
	if err != nil {
		return nil, err
	}

	if loginPass != user.Password {
		if needCheck {
			incrLoginFailCount(ctx, redis, username, seconds)
		}
		return nil, fmt.Errorf("Username or password invalid")
	}

	return user, nil
}

func UserTotal(ctx *ctx.Context, query string, stime, etime int64) (num int64, err error) {
	db := DB(ctx).Model(&User{})

	if stime != 0 && etime != 0 {
		db = db.Where("last_active_time between ? and ?", stime, etime)
	}

	if query != "" {
		q := "%" + query + "%"
		num, err = Count(db.Where("username like ? or nickname like ? or phone like ? or email like ?", q, q, q, q))
	} else {
		num, err = Count(db)
	}

	if err != nil {
		return num, errors.WithMessage(err, "failed to count user")
	}

	return num, nil
}

func UserGets(ctx *ctx.Context, query string, limit, offset int, stime, etime int64,
	order string, desc bool) ([]User, error) {

	session := DB(ctx)

	if stime != 0 && etime != 0 {
		session = session.Where("last_active_time between ? and ?", stime, etime)
	}

	if desc {
		order = order + " desc"
	} else {
		order = order + " asc"
	}

	session = session.Order(order)

	if query != "" {
		q := "%" + query + "%"
		session = session.Where("username like ? or nickname like ? or phone like ? or email like ?", q, q, q, q)
	}

	var users []User
	err := session.Limit(limit).Offset(offset).Find(&users).Error
	if err != nil {
		return users, errors.WithMessage(err, "failed to query user")
	}

	for i := range users {
		users[i].RolesLst = strings.Fields(users[i].Roles)
		users[i].Admin = users[i].IsAdmin()
		users[i].Password = ""

		// query for user group information
		var userGroupIDs []int64
		userGroupIDs, err = MyGroupIds(ctx, users[i].Id)
		if err != nil {
			return users, errors.WithMessage(err, "failed to query group_ids")
		}

		if err = DB(ctx).Table("user_group").Where("id IN (?)", userGroupIDs).
			Find(&users[i].UserGroupsRes).Error; err != nil {
			return users, errors.WithMessage(err, "failed to query user_groups")
		}

		// query business group information
		var busiGroupIDs []int64
		busiGroupIDs, err = BusiGroupIds(ctx, userGroupIDs)
		if err != nil {
			return users, errors.WithMessage(err, "failed to query busi_group_id")
		}

		if err = DB(ctx).Table("busi_group").Where("id IN (?)", busiGroupIDs).
			Find(&users[i].BusiGroupsRes).Error; err != nil {
			return users, errors.WithMessage(err, "failed to query busi_groups")
		}
	}

	return users, nil
}

func UserGetAll(ctx *ctx.Context) ([]*User, error) {
	if !ctx.IsCenter {
		lst, err := poster.GetByUrls[[]*User](ctx, "/v1/n9e/users")
		return lst, err
	}

	var lst []*User
	err := DB(ctx).Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].RolesLst = strings.Fields(lst[i].Roles)
			lst[i].Admin = lst[i].IsAdmin()
		}
	}
	return lst, err
}

func UserGetsByIds(ctx *ctx.Context, ids []int64) ([]User, error) {
	if len(ids) == 0 {
		return []User{}, nil
	}

	var lst []User
	err := DB(ctx).Where("id in ?", ids).Order("username").Find(&lst).Error
	if err == nil {
		for i := 0; i < len(lst); i++ {
			lst[i].RolesLst = strings.Fields(lst[i].Roles)
			lst[i].Admin = lst[i].IsAdmin()
		}
	}

	return lst, err
}

func UserGetsBySso(ctx *ctx.Context, sso string) (map[string]*User, error) {
	session := DB(ctx).Where("belong=?", sso).Order("username")

	var users []User
	err := session.Find(&users).Error
	if err != nil {
		return nil, errors.WithMessage(err, "failed to query user")
	}

	usersMap := make(map[string]*User, len(users))
	for i, user := range users {
		usersMap[user.Username] = &users[i]
	}

	return usersMap, nil
}

func UserDelByIds(ctx *ctx.Context, userIds []int64) error {
	return DB(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id in ?", userIds).Delete(&UserGroupMember{}).Error; err != nil {
			return err
		}

		if err := tx.Where("id in ?", userIds).Delete(&User{}).Error; err != nil {
			return err
		}

		return nil
	})
}

func (u *User) CanModifyUserGroup(ctx *ctx.Context, ug *UserGroup) (bool, error) {
	// 我是管理员，自然可以
	if u.IsAdmin() {
		return true, nil
	}

	// 我是创建者，自然可以
	if ug.CreateBy == u.Username {
		return true, nil
	}

	// 我是成员，也可以吧，简单搞
	num, err := UserGroupMemberCount(ctx, "user_id=? and group_id=?", u.Id, ug.Id)
	if err != nil {
		return false, err
	}

	return num > 0, nil
}

func (u *User) CanDoBusiGroup(ctx *ctx.Context, bg *BusiGroup, permFlag ...string) (bool, error) {
	if u.IsAdmin() {
		return true, nil
	}

	// 我在任意一个UserGroup里，就有权限
	ugids, err := UserGroupIdsOfBusiGroup(ctx, bg.Id, permFlag...)
	if err != nil {
		return false, err
	}

	if len(ugids) == 0 {
		return false, nil
	}

	num, err := UserGroupMemberCount(ctx, "user_id = ? and group_id in ?", u.Id, ugids)
	return num > 0, err
}

func (u *User) CheckPerm(ctx *ctx.Context, operation string) (bool, error) {
	if u.IsAdmin() {
		return true, nil
	}

	return RoleHasOperation(ctx, u.RolesLst, operation)
}

func UserStatistics(ctx *ctx.Context) (*Statistics, error) {
	if !ctx.IsCenter {
		s, err := poster.GetByUrls[*Statistics](ctx, "/v1/n9e/statistic?name=user")
		return s, err
	}

	session := DB(ctx).Model(&User{}).Select("count(*) as total", "max(update_at) as last_updated")

	var stats []*Statistics
	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func (u *User) NopriIdents(ctx *ctx.Context, idents []string) ([]string, error) {
	if u.IsAdmin() {
		return []string{}, nil
	}

	ugids, err := MyGroupIds(ctx, u.Id)
	if err != nil {
		return []string{}, err
	}

	if len(ugids) == 0 {
		return idents, nil
	}

	bgids, err := BusiGroupIds(ctx, ugids, "rw")
	if err != nil {
		return []string{}, err
	}

	if len(bgids) == 0 {
		return idents, nil
	}

	var allowedIdents []string
	err = DB(ctx).Model(&Target{}).Where("group_id in ?", bgids).Pluck("ident", &allowedIdents).Error
	if err != nil {
		return []string{}, err
	}

	return slice.SubString(idents, allowedIdents), nil
}

// 我是管理员，返回所有
// 或者我是成员
func (u *User) BusiGroups(ctx *ctx.Context, limit int, query string, all ...bool) ([]BusiGroup, error) {
	session := DB(ctx).Order("name").Limit(limit)

	var lst []BusiGroup
	if u.IsAdmin() || (len(all) > 0 && all[0]) {
		err := session.Where("name like ?", "%"+query+"%").Find(&lst).Error
		if err != nil {
			return lst, err
		}

		if len(lst) == 0 && len(query) > 0 {
			// 隐藏功能，一般人不告诉，哈哈。query可能是给的ident，所以上面的sql没有查到，当做ident来查一下试试
			var t *Target
			t, err = TargetGet(ctx, "ident=?", query)
			if err != nil {
				return lst, err
			}

			if t == nil {
				return lst, nil
			}

			err = DB(ctx).Order("name").Limit(limit).Where("id=?", t.GroupId).Find(&lst).Error
		}

		return lst, err
	}

	userGroupIds, err := MyGroupIds(ctx, u.Id)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get MyGroupIds")
	}

	busiGroupIds, err := BusiGroupIds(ctx, userGroupIds)
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
		t, err = TargetGet(ctx, "ident=?", query)
		if err != nil {
			return lst, err
		}

		if t != nil && slice.ContainsInt64(busiGroupIds, t.GroupId) {
			err = DB(ctx).Order("name").Limit(limit).Where("id=?", t.GroupId).Find(&lst).Error
		}
	}

	return lst, err
}

func (u *User) UserGroups(ctx *ctx.Context, limit int, query string) ([]UserGroup, error) {
	session := DB(ctx).Order("name").Limit(limit)

	var lst []UserGroup
	if u.IsAdmin() {
		err := session.Where("name like ?", "%"+query+"%").Find(&lst).Error
		if err != nil {
			return lst, err
		}

		var user *User
		if len(lst) == 0 && len(query) > 0 {
			// 隐藏功能，一般人不告诉，哈哈。query可能是给的用户名，所以上面的sql没有查到，当做user来查一下试试
			user, err = UserGetByUsername(ctx, query)
			if user == nil {
				return lst, err
			}
			var ids []int64
			ids, err = MyGroupIds(ctx, user.Id)
			if err != nil || len(ids) == 0 {
				return lst, err
			}
			lst, err = UserGroupGetByIds(ctx, ids)
		}
		return lst, err
	}

	ids, err := MyGroupIds(ctx, u.Id)
	if err != nil {
		return nil, errors.WithMessage(err, "failed to get MyGroupIds")
	}

	if len(ids) > 0 {
		session = session.Where("id in ? or create_by = ?", ids, u.Username)
	} else {
		session = session.Where("create_by = ?", u.Username)
	}

	if len(query) > 0 {
		session = session.Where("name like ?", "%"+query+"%")
	}

	err = session.Find(&lst).Error
	return lst, err
}

func (u *User) ExtractToken(key string) (string, bool) {
	bs, err := u.Contacts.MarshalJSON()
	if err != nil {
		logger.Errorf("handle_notice: failed to marshal contacts: %v", err)
		return "", false
	}

	switch key {
	case Dingtalk:
		ret := gjson.GetBytes(bs, DingtalkKey)
		return ret.String(), ret.Exists()
	case Wecom:
		ret := gjson.GetBytes(bs, WecomKey)
		return ret.String(), ret.Exists()
	case Feishu, FeishuCard:
		ret := gjson.GetBytes(bs, FeishuKey)
		return ret.String(), ret.Exists()
	case Mm:
		ret := gjson.GetBytes(bs, MmKey)
		return ret.String(), ret.Exists()
	case Telegram:
		ret := gjson.GetBytes(bs, TelegramKey)
		return ret.String(), ret.Exists()
	case Email:
		return u.Email, u.Email != ""
	default:
		return "", false
	}
}

func (u *User) FindSameContact(email, phone string) string {
	if u.Email != "" && u.Email == email {
		return "email"
	}

	if u.Phone != "" && u.Phone == phone {
		return "phone"
	}

	return ""
}

// AddUserAndGroups Add a user and add it to multiple groups in a single transaction
func (u *User) AddUserAndGroups(ctx *ctx.Context, coverTeams bool) error {

	// Try to add a user
	if err := u.Add(ctx); err != nil {
		return errors.WithMessage(err, "failed to add user")
	}

	// Try to add a group for the user
	if err := UserGroupMemberSyncByUser(ctx, u, coverTeams); err != nil {
		return errors.WithMessage(err, "failed to add user to groups")
	}

	return nil
}
