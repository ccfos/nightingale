package models

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/toolkits/pkg/cache"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/str"
	"gopkg.in/ldap.v3"
)

const (
	LOGIN_T_SMS      = "sms-code"
	LOGIN_T_EMAIL    = "email-code"
	LOGIN_T_PWD      = "password"
	LOGIN_T_LDAP     = "ldap"
	LOGIN_T_RST      = "rst-code"
	LOGIN_T_LOGIN    = "login-code"
	LOGIN_EXPIRES_IN = 300
)
const (
	USER_S_ACTIVE = iota
	USER_S_INACTIVE
	USER_S_LOCKED
	USER_S_FROZEN
	USER_S_WRITEN_OFF
)
const (
	USER_T_NATIVE = iota
	USER_T_TEMP
)

type User struct {
	Id           int64     `json:"id"`
	UUID         string    `json:"uuid" xorm:"'uuid'"`
	Username     string    `json:"username"`
	Password     string    `json:"-"`
	Passwords    string    `json:"-"`
	Dispname     string    `json:"dispname"`
	Phone        string    `json:"phone"`
	Email        string    `json:"email"`
	Im           string    `json:"im"`
	Portrait     string    `json:"portrait"`
	Intro        string    `json:"intro"`
	Organization string    `json:"organization"`
	Type         int       `json:"type" xorm:"'typ'" description:"0: long-term account; 1: temporary account"`
	Status       int       `json:"status" description:"0: active, 1: inactive, 2: locked, 3: frozen, 4: writen-off"`
	IsRoot       int       `json:"is_root"`
	LeaderId     int64     `json:"leader_id"`
	LeaderName   string    `json:"leader_name"`
	LoginErrNum  int       `json:"login_err_num"`
	ActiveBegin  int64     `json:"active_begin" description:"for temporary account"`
	ActiveEnd    int64     `json:"active_end" description:"for temporary account"`
	LockedAt     int64     `json:"locked_at" description:"locked time"`
	UpdatedAt    int64     `json:"updated_at" description:"user info change time"`
	PwdUpdatedAt int64     `json:"pwd_updated_at" description:"password change time"`
	PwdExpiresAt int64     `xorm:"-" json:"pwd_expires_at" description:"password expires time"`
	LoggedAt     int64     `json:"logged_at" description:"last logged time"`
	CreateAt     time.Time `json:"create_at" xorm:"<-"`
}

func (u *User) Validate() error {
	u.Username = strings.TrimSpace(u.Username)
	if u.Username == "" {
		return _e("username is blank")
	}

	if str.Dangerous(u.Username) {
		return _e("%s %s format error", _s("username"), u.Username)
	}

	if str.Dangerous(u.Dispname) {
		return _e("%s %s format error", _s("dispname"), u.Dispname)
	}

	if u.Phone != "" && !str.IsPhone(u.Phone) {
		return _e("%s %s format error", _s("phone"), u.Phone)
	}

	if u.Email != "" && !str.IsMail(u.Email) {
		return _e("%s %s format error", _s("email"), u.Email)
	}

	if len(u.Username) > 32 {
		return _e("username too long (max:%d)", 32)
	}

	if len(u.Dispname) > 32 {
		return _e("dispname too long (max:%d)", 32)
	}

	if strings.ContainsAny(u.Im, "%'") {
		return _e("%s %s format error", "im", u.Im)
	}

	cnt, _ := DB["rdb"].Where("((email <> '' and email=?) or (phone <> '' and phone=?)) and username=?",
		u.Email, u.Phone, u.Username).Count(u)
	if cnt > 0 {
		return _e("email %s or phone %s is exists", u.Email, u.Phone)
	}
	return nil
}

func (u *User) CopyLdapAttr(sr *ldap.SearchResult) {
	attrs := LDAPConfig.Attributes
	if attrs.Dispname != "" {
		u.Dispname = sr.Entries[0].GetAttributeValue(attrs.Dispname)
	}
	if attrs.Email != "" {
		u.Email = sr.Entries[0].GetAttributeValue(attrs.Email)
	}
	if attrs.Phone != "" {
		u.Phone = sr.Entries[0].GetAttributeValue(attrs.Phone)
	}
	if attrs.Im != "" {
		u.Im = sr.Entries[0].GetAttributeValue(attrs.Im)
	}
}

func InitRooter() {
	var u User
	has, err := DB["rdb"].Where("username=?", "root").Get(&u)
	if err != nil {
		log.Fatalln("cannot query user[root]", err)
	}

	if has {
		return
	}

	pass, err := CryptoPass("root.2020")
	if err != nil {
		log.Fatalln(err)
	}

	u = User{
		Username: "root",
		Password: pass,
		Dispname: "超管",
		IsRoot:   1,
		UUID:     GenUUIDForUser("root"),
	}

	_, err = DB["rdb"].Insert(u)
	if err != nil {
		log.Fatalln("cannot insert user[root]")
	}

	log.Println("user root init done")
}

func LdapLogin(username, pass string) (*User, error) {
	sr, err := ldapReq(username, pass)
	if err != nil {
		return nil, err
	}

	var user User
	has, err := DB["rdb"].Where("username=?", username).Get(&user)
	if err != nil {
		return nil, err
	}

	user.CopyLdapAttr(sr)

	if has {
		if LDAPConfig.CoverAttributes {
			_, err := DB["rdb"].Where("id=?", user.Id).Update(user)
			return &user, err
		} else {
			return &user, err
		}
	}

	user.Username = username
	user.Password = "******"
	user.UUID = GenUUIDForUser(username)
	_, err = DB["rdb"].Insert(user)
	return &user, nil
}

func PassLogin(username, pass string) (*User, error) {
	var user User
	has, err := DB["rdb"].Where("username=?", username).Get(&user)
	if err != nil {
		return nil, _e("Login fail, check your username and password")
	}

	if !has {
		logger.Infof("password auth fail, no such user: %s", username)
		return nil, _e("Login fail, check your username and password")
	}

	loginPass, err := CryptoPass(pass)
	if err != nil {
		return &user, err
	}

	if loginPass != user.Password {
		logger.Infof("password auth fail, password error, user: %s", username)
		return &user, _e("Login fail, check your username and password")
	}

	return &user, nil
}

func SmsCodeLogin(phone, code string) (*User, error) {
	user, _ := UserGet("phone=?", phone)
	if user == nil {
		return nil, fmt.Errorf("phone %s dose not exist", phone)
	}

	lc, err := LoginCodeGet("username=? and code=? and login_type=?", user.Username, code, LOGIN_T_LOGIN)
	if err != nil {
		logger.Debugf("sms-code auth fail, user: %s", user.Username)
		return user, _e("The code is incorrect")
	}

	if time.Now().Unix()-lc.CreatedAt > LOGIN_EXPIRES_IN {
		logger.Debugf("sms-code auth expired, user: %s", user.Username)
		return user, _e("The code has expired")
	}

	lc.Del()

	return user, nil
}

func EmailCodeLogin(email, code string) (*User, error) {
	user, _ := UserGet("email=?", email)
	if user == nil {
		return nil, fmt.Errorf("email %s dose not exist", email)
	}

	lc, err := LoginCodeGet("username=? and code=? and login_type=?", user.Username, code, LOGIN_T_LOGIN)
	if err != nil {
		logger.Debugf("email-code auth fail, user: %s", user.Username)
		return user, _e("The code is incorrect")
	}

	if time.Now().Unix()-lc.CreatedAt > LOGIN_EXPIRES_IN {
		logger.Debugf("email-code auth expired, user: %s", user.Username)
		return user, _e("The code has expired")
	}

	lc.Del()

	return user, nil
}

func UserGet(where string, args ...interface{}) (*User, error) {
	var obj User
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func UserMustGet(where string, args ...interface{}) (*User, error) {
	var obj User
	has, err := DB["rdb"].Where(where, args...).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, _e("User dose not exist")
	}

	return &obj, nil
}

func (u *User) IsRooter() bool {
	return u.IsRoot == 1
}

func (u *User) Update(cols ...string) error {
	if err := u.Validate(); err != nil {
		return err
	}

	_, err := DB["rdb"].Where("id=?", u.Id).Cols(cols...).Update(u)
	return err
}

func (u *User) Save() error {
	if err := u.Validate(); err != nil {
		return err
	}

	if u.Id > 0 {
		return _e("user.id[%d] not equal 0", u.Id)
	}

	if u.UUID == "" {
		u.UUID = GenUUIDForUser(u.Username)
	}

	cnt, err := DB["rdb"].Where("username=?", u.Username).Count(new(User))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return _e("Username %s already exists", u.Username)
	}

	u.UpdatedAt = time.Now().Unix()

	_, err = DB["rdb"].Insert(u)
	return err
}

func UserTotal(ids []int64, where string, args ...interface{}) (int64, error) {
	session := DB["rdb"].NewSession()
	defer session.Close()

	if len(ids) > 0 {
		session = session.In("id", ids)
	}

	if where != "" {
		session = session.Where(where, args...)
	}

	return session.Count(new(User))
}

func UserGetsByIds(ids []int64) ([]User, error) {
	var users []User
	err := DB["rdb"].In("id", ids).Find(&users)
	return users, err
}

func UserGets(ids []int64, limit, offset int, where string, args ...interface{}) ([]User, error) {
	session := DB["rdb"].Limit(limit, offset).OrderBy("username")
	if len(ids) > 0 {
		session = session.In("id", ids)
	}

	if where != "" {
		session = session.Where(where, args...)
	}

	var users []User
	err := session.Find(&users)
	return users, err
}

func (u *User) Del() error {
	session := DB["rdb"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM team_user WHERE user_id=?", u.Id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM role_global_user WHERE user_id=?", u.Id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM node_admin WHERE user_id=?", u.Id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM user_token WHERE user_id=?", u.Id); err != nil {
		session.Rollback()
		return err
	}

	if _, err := session.Exec("DELETE FROM user WHERE id=?", u.Id); err != nil {
		session.Rollback()
		return err
	}

	return session.Commit()
}

func (u *User) CanModifyTeam(t *Team) (bool, error) {
	if u.IsRoot == 1 {
		return true, nil
	}

	if u.Id == t.Creator {
		return true, nil
	}

	session := DB["rdb"].Where("team_id=? and user_id=?", t.Id, u.Id)
	if t.Mgmt == 1 {
		session = session.Where("is_admin=1")
	}

	cnt, err := session.Count(new(TeamUser))
	return cnt > 0, err
}

func (u *User) CheckPermByNode(node *Node, operation string) {
	if node == nil {
		errors.Bomb(_s("node is nil"))
	}

	if operation == "" {
		errors.Bomb(_s("operation is blank"))
	}

	has, err := u.HasPermByNode(node, operation)
	errors.Dangerous(err)

	if !has {
		errors.Bomb(_s("no privilege"))
	}
}

func (u *User) HasPermByNode(node *Node, operation string) (bool, error) {
	// 我是超管，自然有权限
	if u.IsRoot == 1 {
		return true, nil
	}

	// 我是path上游的某个admin，自然有权限
	nodeIds, err := NodeIdsByPaths(Paths(node.Path))
	if err != nil {
		return false, err
	}

	if len(nodeIds) == 0 {
		// 这个数据有问题，是个不正常的path
		return false, nil
	}

	yes, err := NodesAdminExists(nodeIds, u.Id)
	if err != nil {
		return false, err
	}

	if yes {
		return true, nil
	}

	// 都不是，当成普通用户校验
	// 1. 查看哪些角色包含这个操作（权限点）
	roleIds, err := RoleIdsHasOp(operation)
	if err != nil {
		return false, err
	}

	if len(roleIds) == 0 {
		return false, nil
	}

	// 2. 用户在上游任一节点绑过任一角色？
	yes, err = NodeRoleExists(nodeIds, roleIds, u.Username)
	if err != nil {
		return false, err
	}

	return yes, nil
}

func (u *User) CheckPermGlobal(operation string) {
	has, err := u.HasPermGlobal(operation)
	errors.Dangerous(err)

	if !has {
		errors.Bomb(_s("no privilege"))
	}
}

func (u *User) HasPermGlobal(operation string) (bool, error) {
	if u.IsRoot == 1 {
		return true, nil
	}

	rids, err := RoleIdsHasOp(operation)
	if err != nil {
		return false, _e("[CheckPermGlobal] RoleIdsHasOp fail: %v, operation: %s", err, operation)
	}

	if rids == nil || len(rids) == 0 {
		return false, nil
	}

	has, err := UserHasGlobalRole(u.Id, rids)
	if err != nil {
		return false, _e("[CheckPermGlobal] UserHasGlobalRole fail: %v, username: %s", err, u.Username)
	}

	return has, nil
}

func UserGetByIds(ids []int64) ([]User, error) {
	if ids == nil || len(ids) == 0 {
		return []User{}, nil
	}

	var objs []User
	err := DB["rdb"].In("id", ids).OrderBy("username").Find(&objs)
	return objs, err
}

func UserGetByNames(names []string) ([]User, error) {
	if names == nil || len(names) == 0 {
		return []User{}, nil
	}

	var objs []User
	err := DB["rdb"].In("username", names).OrderBy("username").Find(&objs)
	return objs, err
}

func UserGetByUUIDs(uuids []string) ([]User, error) {
	if uuids == nil || len(uuids) == 0 {
		return []User{}, nil
	}

	var objs []User
	err := DB["rdb"].In("uuid", uuids).OrderBy("username").Find(&objs)
	return objs, err
}

func UserSearchListInIds(ids []int64, query string, limit, offset int) ([]User, error) {
	if ids == nil || len(ids) == 0 {
		return []User{}, nil
	}

	session := DB["rdb"].In("id", ids)
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("username like ? or dispname like ?", q, q)
	}

	var objs []User
	err := session.OrderBy("username").Limit(limit, offset).Find(&objs)
	return objs, err
}

func UserSearchTotalInIds(ids []int64, query string) (int64, error) {
	if ids == nil || len(ids) == 0 {
		return 0, nil
	}

	session := DB["rdb"].In("id", ids)
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("username like ? or dispname like ?", q, q)
	}

	return session.Count(new(User))
}

func safeUserIds(ids []int64) ([]int64, error) {
	cnt := len(ids)
	ret := make([]int64, 0, cnt)
	for i := 0; i < cnt; i++ {
		user, err := UserGet("id=?", ids[i])
		if err != nil {
			return nil, err
		}

		if user != nil {
			ret = append(ret, ids[i])
		}
	}
	return ret, nil
}

// Deprecated
func UsernameByUUID(uuid string) string {
	logger.Warningf("UsernameByUUID is Deprectaed, use UsernameBySid instead of it")
	if uuid == "" {
		return ""
	}

	var username string
	if err := cache.Get("user."+uuid, &username); err == nil {
		return username
	}

	user, err := UserGet("uuid=?", uuid)
	if err != nil {
		logger.Errorf("UserGet(uuid=%s) fail:%v", uuid, err)
		return ""
	}

	if user == nil {
		return ""
	}

	cache.Set("user."+uuid, user.Username, time.Hour*24)

	return user.Username
}

func UserFillUUIDs() error {
	var users []User
	err := DB["rdb"].Find(&users)
	if err != nil {
		return err
	}

	count := len(users)
	for i := 0; i < count; i++ {
		if users[i].UUID == "" {
			users[i].UUID = GenUUIDForUser(users[i].Username)
			err = users[i].Update("uuid")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// PermResIds 我在某些节点是管理员，或者我在某些节点有此权限点，获取下面的叶子节点挂载的资源列表
func (u *User) PermResIds(operation string) ([]int64, error) {
	nids1, err := NodeIdsIamAdmin(u.Id)
	if err != nil {
		return nil, err
	}

	nids2, err := NodeIdsBindingUsernameWithOp(u.Username, operation)
	if err != nil {
		return nil, err
	}

	nids := append(nids1, nids2...)

	nodes, err := NodeByIds(nids)
	if err != nil {
		return nil, err
	}

	lids, err := LeafIdsByNodes(nodes)
	if err != nil {
		return nil, err
	}

	if len(lids) == 0 {
		return []int64{}, nil
	}

	return ResIdsGetByNodeIds(lids)
}

// NopriResIdents 我没有权限的资源ident列表
func (u *User) NopriResIdents(resIds []int64, op string) ([]string, error) {
	permIds, err := u.PermResIds(op)
	if err != nil {
		return nil, err
	}

	// 全量的 - 我有权限的 = 我没有权限的
	nopris := slice.SubInt64(resIds, permIds)
	return ResourceIdentsByIds(nopris)
}

func GetUsersNameByIds(ids string) ([]string, error) {
	var names []string
	ids = strings.Replace(ids, "[", "", -1)
	ids = strings.Replace(ids, "]", "", -1)
	idsStrArr := strings.Split(ids, ",")

	userIds := []int64{}
	for _, userId := range idsStrArr {
		id, _ := strconv.ParseInt(userId, 10, 64)
		userIds = append(userIds, id)
	}

	users, err := UserGetByIds(userIds)

	if err != nil {
		return names, err
	}
	for _, user := range users {
		names = append(names, user.Username)
	}
	return names, err
}

func UsersGet(where string, args ...interface{}) ([]User, error) {
	var objs []User
	err := DB["rdb"].Where(where, args...).Find(&objs)
	if err != nil {
		return nil, err
	}

	return objs, nil
}

func (u *User) PermByNode(node *Node, localOpsList []string) ([]string, error) {
	// 我是超管，自然有权限
	if u.IsRoot == 1 {
		return localOpsList, nil
	}

	// 我是path上游的某个admin，自然有权限
	nodeIds, err := NodeIdsByPaths(Paths(node.Path))
	if err != nil {
		return nil, err
	}

	if len(nodeIds) == 0 {
		return nil, nil
	}

	if yes, err := NodesAdminExists(nodeIds, u.Id); err != nil {
		return nil, err
	} else if yes {
		return localOpsList, nil
	}

	if roleIds, err := RoleIdsBindingUsername(u.Username, nodeIds); err != nil {
		return nil, err
	} else {
		return OperationsOfRoles(roleIds)
	}
}
