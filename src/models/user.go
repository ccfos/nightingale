package models

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/toolkits/pkg/slice"

	"github.com/toolkits/pkg/cache"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
	"gopkg.in/ldap.v3"

	"github.com/didi/nightingale/src/modules/rdb/config"
)

type User struct {
	Id         int64  `json:"id"`
	UUID       string `json:"-" xorm:"'uuid'"`
	Username   string `json:"username"`
	Password   string `json:"-"`
	Dispname   string `json:"dispname"`
	Phone      string `json:"phone"`
	Email      string `json:"email"`
	Im         string `json:"im"`
	Portrait   string `json:"portrait"`
	Intro      string `json:"intro"`
	IsRoot     int    `json:"is_root"`
	LeaderId   int64  `json:"leader_id"`
	LeaderName string `json:"leader_name"`
}

func (u *User) CopyLdapAttr(sr *ldap.SearchResult) {
	attrs := config.Config.LDAP.Attributes
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

func LdapLogin(user, pass, clientIP string) error {
	sr, err := ldapReq(user, pass)
	if err != nil {
		return err
	}

	go LoginLogNew(user, clientIP, "in")

	var u User
	has, err := DB["rdb"].Where("username=?", user).Get(&u)
	if err != nil {
		return err
	}

	u.CopyLdapAttr(sr)

	if has {
		if config.Config.LDAP.CoverAttributes {
			_, err := DB["rdb"].Where("id=?", u.Id).Update(u)
			return err
		} else {
			return nil
		}
	}

	u.Username = user
	u.Password = "******"
	u.UUID = GenUUIDForUser(user)
	_, err = DB["rdb"].Insert(u)
	return err
}

func PassLogin(user, pass, clientIP string) error {
	var u User
	has, err := DB["rdb"].Where("username=?", user).Cols("password").Get(&u)
	if err != nil {
		return err
	}

	if !has {
		logger.Infof("password auth fail, no such user: %s", user)
		return fmt.Errorf("login fail, check your username and password")
	}

	loginPass, err := CryptoPass(pass)
	if err != nil {
		return err
	}

	if loginPass != u.Password {
		logger.Infof("password auth fail, password error, user: %s", user)
		return fmt.Errorf("login fail, check your username and password")
	}

	go LoginLogNew(user, clientIP, "in")

	return nil
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

func (u *User) IsRooter() bool {
	return u.IsRoot == 1
}

func (u *User) CheckFields() {
	u.Username = strings.TrimSpace(u.Username)
	if u.Username == "" {
		errors.Bomb("username is blank")
	}

	if str.Dangerous(u.Username) {
		errors.Bomb("username is dangerous")
	}

	if str.Dangerous(u.Dispname) {
		errors.Bomb("dispname is dangerous")
	}

	if u.Phone != "" && !str.IsPhone(u.Phone) {
		errors.Bomb("%s format error", u.Phone)
	}

	if u.Email != "" && !str.IsMail(u.Email) {
		errors.Bomb("%s format error", u.Email)
	}

	if len(u.Username) > 32 {
		errors.Bomb("username too long")
	}

	if len(u.Dispname) > 32 {
		errors.Bomb("dispname too long")
	}

	if strings.ContainsAny(u.Im, "%'") {
		errors.Bomb("im invalid")
	}
}

func (u *User) Update(cols ...string) error {
	u.CheckFields()
	_, err := DB["rdb"].Where("id=?", u.Id).Cols(cols...).Update(u)
	return err
}

func (u *User) Save() error {
	u.CheckFields()

	if u.Id > 0 {
		return fmt.Errorf("user.id[%d] not equal 0", u.Id)
	}

	if u.UUID == "" {
		u.UUID = GenUUIDForUser(u.Username)
	}

	cnt, err := DB["rdb"].Where("username=?", u.Username).Count(new(User))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("username already exists")
	}

	_, err = DB["rdb"].Insert(u)
	return err
}

func UserTotal(ids []int64, query string) (int64, error) {
	session := DB["rdb"].NewSession()
	defer session.Close()

	if len(ids) > 0 {
		session = session.In("id", ids)
	}

	if query != "" {
		q := "%" + query + "%"
		return session.Where("username like ? or dispname like ? or phone like ? or email like ?", q, q, q, q).Count(new(User))
	}

	return session.Count(new(User))
}

func UserGets(ids []int64, query string, limit, offset int) ([]User, error) {
	session := DB["rdb"].Limit(limit, offset).OrderBy("username")
	if len(ids) > 0 {
		session = session.In("id", ids)
	}

	if query != "" {
		q := "%" + query + "%"
		session = session.Where("username like ? or dispname like ? or phone like ? or email like ?", q, q, q, q)
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
		errors.Bomb("node is nil")
	}

	if operation == "" {
		errors.Bomb("operation is blank")
	}

	has, err := u.HasPermByNode(node, operation)
	errors.Dangerous(err)

	if !has {
		errors.Bomb("no privilege")
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
		errors.Bomb("no privilege")
	}
}

func (u *User) HasPermGlobal(operation string) (bool, error) {
	if u.IsRoot == 1 {
		return true, nil
	}

	rids, err := RoleIdsHasOp(operation)
	if err != nil {
		return false, fmt.Errorf("[CheckPermGlobal] RoleIdsHasOp fail: %v, operation: %s", err, operation)
	}

	if rids == nil || len(rids) == 0 {
		return false, nil
	}

	has, err := UserHasGlobalRole(u.Id, rids)
	if err != nil {
		return false, fmt.Errorf("[CheckPermGlobal] UserHasGlobalRole fail: %v, username: %s", err, u.Username)
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

func UsernameByUUID(uuid string) string {
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
