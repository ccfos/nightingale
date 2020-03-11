package model

import (
	"crypto/tls"
	"fmt"
	"log"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/toolkits/pkg/errors"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/str"
	"gopkg.in/ldap.v3"

	"github.com/didi/nightingale/src/modules/monapi/config"
)

type User struct {
	Id       int64  `json:"id"`
	Username string `json:"username"`
	Password string `json:"-"`
	Dispname string `json:"dispname"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Im       string `json:"im"`
	IsRoot   int    `json:"is_root"`
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
}

func (u *User) Update(cols ...string) error {
	u.CheckFields()
	_, err := DB["uic"].Where("id=?", u.Id).Cols(cols...).Update(u)
	return err
}

func (u *User) Save() error {
	u.CheckFields()

	if u.Id > 0 {
		return fmt.Errorf("user.id[%d] not equal 0", u.Id)
	}

	cnt, err := DB["uic"].Where("username=?", u.Username).Count(new(User))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return fmt.Errorf("username already exists")
	}

	_, err = DB["uic"].Insert(u)
	return err
}

func (u *User) Del() error {
	session := DB["uic"].NewSession()
	defer session.Close()

	if err := session.Begin(); err != nil {
		return err
	}

	if _, err := session.Exec("DELETE FROM team_user WHERE user_id=?", u.Id); err != nil {
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

	session := DB["uic"].Where("team_id=? and user_id=?", t.Id, u.Id)
	if t.Mgmt == 1 {
		session = session.Where("is_admin=1")
	}

	cnt, err := session.Count(new(TeamUser))
	return cnt > 0, err
}

func InitRoot() {
	var u User
	has, err := DB["uic"].Where("username=?", "root").Get(&u)
	if err != nil {
		log.Fatalln("cannot query user[root]", err)
	}

	if has {
		return
	}

	// gen := str.RandLetters(32)

	u = User{
		Username: "root",
		Password: config.CryptoPass("root"),
		Dispname: "超管",
		IsRoot:   1,
	}

	_, err = DB["uic"].Insert(&u)
	if err != nil {
		log.Fatalln("cannot insert user[root]")
	}

	logger.Info("user root init done")
}

func LdapLogin(user, pass string) error {
	var conn *ldap.Conn
	var err error

	lc := config.Get().LDAP
	addr := fmt.Sprintf("%s:%d", lc.Host, lc.Port)

	if lc.TLS {
		conn, err = ldap.DialTLS("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	} else {
		conn, err = ldap.Dial("tcp", addr)
	}

	if err != nil {
		return fmt.Errorf("cannot dial ldap: %v", err)
	}

	defer conn.Close()

	if !lc.TLS && lc.StartTLS {
		err = conn.StartTLS(&tls.Config{InsecureSkipVerify: true})
		if err != nil {
			return fmt.Errorf("ldap.conn startTLS fail: %v", err)
		}
	}

	err = conn.Bind(lc.BindUser, lc.BindPass)
	if err != nil {
		return fmt.Errorf("bind ldap fail: %v, use %s", err, lc.BindUser)
	}

	searchRequest := ldap.NewSearchRequest(
		lc.BaseDn, // The base dn to search
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(lc.AuthFilter, user), // The filter to apply
		[]string{},                       // A list attributes to retrieve
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return fmt.Errorf("ldap search fail: %v", err)
	}

	if len(sr.Entries) == 0 {
		return fmt.Errorf("cannot find such user: %v", user)
	}

	if len(sr.Entries) > 1 {
		return fmt.Errorf("multi users is search, query user: %v", user)
	}

	err = conn.Bind(sr.Entries[0].DN, pass)
	if err != nil {
		return fmt.Errorf("password error")
	}

	cnt, err := DB["uic"].Where("username=?", user).Count(new(User))
	if err != nil {
		return err
	}

	if cnt > 0 {
		return nil
	}

	u := &User{
		Username: user,
		Password: "******",
		Dispname: "",
		Email:    "",
	}

	_, err = DB["uic"].Insert(u)
	return err
}

func PassLogin(user, pass string) error {
	var u User
	has, err := DB["uic"].Where("username=?", user).Cols("password").Get(&u)
	if err != nil {
		return err
	}

	if !has {
		return fmt.Errorf("user[%s] not found", user)
	}

	if config.CryptoPass(pass) != u.Password {
		return fmt.Errorf("password error")
	}

	return nil
}

func UserGet(col string, val interface{}) (*User, error) {
	var obj User
	has, err := DB["uic"].Where(col+"=?", val).Get(&obj)
	if err != nil {
		return nil, err
	}

	if !has {
		return nil, nil
	}

	return &obj, nil
}

func UserTotal(query string) (int64, error) {
	if query != "" {
		q := "%" + query + "%"
		return DB["uic"].Where("username like ? or dispname like ? or phone like ? or email like ?", q, q, q, q).Count(new(User))
	}

	return DB["uic"].Count(new(User))
}

func UserGets(query string, limit, offset int) ([]User, error) {
	session := DB["uic"].Limit(limit, offset).OrderBy("username")
	if query != "" {
		q := "%" + query + "%"
		session = session.Where("username like ? or dispname like ? or phone like ? or email like ?", q, q, q, q)
	}

	var users []User
	err := session.Find(&users)
	return users, err
}

func UserNameGetByIds(ids string) ([]string, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	var userIds []int64
	if err := json.Unmarshal([]byte(ids), &userIds); err != nil {
		return nil, err
	}

	var names []string
	err := DB["uic"].Table("user").In("id", userIds).Select("username").Find(&names)
	return names, err
}

func UserGetByIds(ids []int64) ([]User, error) {
	var objs []User
	err := DB["uic"].In("id", ids).Find(&objs)
	return objs, err
}
