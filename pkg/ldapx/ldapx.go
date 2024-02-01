package ldapx

import (
	"crypto/tls"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/go-ldap/ldap/v3"
	"github.com/pkg/errors"
)

type Config struct {
	Enable          bool
	Host            string
	Port            int
	BaseDn          string
	BindUser        string
	BindPass        string
	SyncUsers       bool
	SyncCycle       time.Duration
	UserFilter      string
	AuthFilter      string
	Attributes      LdapAttributes
	CoverAttributes bool
	TLS             bool
	StartTLS        bool
	DefaultRoles    []string
}

type SsoClient struct {
	Enable          bool
	Host            string
	Port            int
	BaseDn          string
	BindUser        string
	BindPass        string
	SyncUsers       bool
	SyncInterval    time.Duration
	UserFilter      string
	AuthFilter      string
	Attributes      LdapAttributes
	CoverAttributes bool
	TLS             bool
	StartTLS        bool
	DefaultRoles    []string

	Ticker *time.Ticker
	sync.RWMutex
}

type LdapAttributes struct {
	Nickname string `yaml:"nickname"`
	Phone    string `yaml:"phone"`
	Email    string `yaml:"email"`
}

func New(cf Config) *SsoClient {
	var s = &SsoClient{
		Ticker: time.NewTicker(time.Hour * 24),
	}

	if !cf.Enable {
		return s
	}

	s.Reload(cf)
	return s
}

func (s *SsoClient) Reload(cf Config) {
	s.Lock()
	defer s.Unlock()
	if !cf.Enable {
		s.Enable = cf.Enable
		return
	}

	s.Enable = cf.Enable
	s.Host = cf.Host
	s.Port = cf.Port
	s.BaseDn = cf.BaseDn
	s.BindUser = cf.BindUser
	s.BindPass = cf.BindPass
	s.AuthFilter = cf.AuthFilter
	s.Attributes = cf.Attributes
	s.CoverAttributes = cf.CoverAttributes
	s.TLS = cf.TLS
	s.StartTLS = cf.StartTLS
	s.DefaultRoles = cf.DefaultRoles
	s.SyncUsers = cf.SyncUsers
	s.SyncInterval = cf.SyncCycle
	s.UserFilter = cf.UserFilter

	if s.SyncInterval > 0 {
		s.Ticker.Reset(s.SyncInterval * time.Second)
	}
}

func (s *SsoClient) genLdapAttributeSearchList() []string {
	ldapAttributes := []string{"uid"}

	s.RLock()
	attrs := s.Attributes
	s.RUnlock()

	if attrs.Nickname != "" {
		ldapAttributes = append(ldapAttributes, attrs.Nickname)
	}
	if attrs.Email != "" {
		ldapAttributes = append(ldapAttributes, attrs.Email)
	}
	if attrs.Phone != "" {
		ldapAttributes = append(ldapAttributes, attrs.Phone)
	}
	return ldapAttributes
}

func (s *SsoClient) newLdapConn() (*ldap.Conn, error) {
	var conn *ldap.Conn
	var err error

	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	if s.TLS {
		conn, err = ldap.DialTLS("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	} else {
		conn, err = ldap.Dial("tcp", addr)
	}

	if err != nil {
		return nil, fmt.Errorf("ldap.error: cannot dial ldap(%s): %v", addr, err)
	}

	if !s.TLS && s.StartTLS {
		if err := conn.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
			return nil, fmt.Errorf("ldap.error: conn startTLS fail: %v", err)
		}
	}

	// if bindUser is empty, anonymousSearch mode
	if s.BindUser != "" {
		// BindSearch mode
		if err := conn.Bind(s.BindUser, s.BindPass); err != nil {
			return nil, fmt.Errorf("ldap.error: bind ldap fail: %v, use user(%s) to bind", err, s.BindUser)
		}
	}

	return conn, nil
}

func (s *SsoClient) ldapReq(conn *ldap.Conn, filter string, values ...interface{}) (*ldap.SearchResult, error) {
	searchRequest := ldap.NewSearchRequest(
		s.BaseDn, // The base dn to search
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(filter, values...), // The filter to apply
		s.genLdapAttributeSearchList(), // A list attributes to retrieve
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		return nil, fmt.Errorf("ldap.error: ldap search fail: %v", err)
	}

	return sr, nil
}

func (s *SsoClient) LoginCheck(user, pass string) (*ldap.SearchResult, error) {
	s.RLock()
	lc := s
	s.RUnlock()

	conn, err := lc.newLdapConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	sr, err := lc.ldapReq(conn, lc.AuthFilter, user)
	if err != nil {
		return nil, fmt.Errorf("ldap.error: ldap search fail: %v", err)
	}

	if len(sr.Entries) == 0 {
		return nil, fmt.Errorf("username or password invalid")
	}

	if len(sr.Entries) > 1 {
		return nil, fmt.Errorf("ldap.error: search user(%s), multi entries found", user)
	}

	if err := conn.Bind(sr.Entries[0].DN, pass); err != nil {
		return nil, fmt.Errorf("username or password invalid")
	}

	return sr, nil
}

func (s *SsoClient) UserGetAll() (map[string]*models.User, error) {
	s.RLock()
	lc := s
	s.RUnlock()

	conn, err := lc.newLdapConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	sr, err := lc.ldapReq(conn, lc.UserFilter)
	if err != nil {
		return nil, fmt.Errorf("ldap.error: ldap search fail: %v", err)
	}

	res := make(map[string]*models.User, len(sr.Entries))
	for _, entry := range sr.Entries {
		res[entry.GetAttributeValue("uid")] = entryAttributeToUser(entry)
	}

	return res, nil
}

func entryAttributeToUser(entry *ldap.Entry) *models.User {
	var user models.User
	user.Username = entry.GetAttributeValue("uid")
	user.Email = entry.GetAttributeValue("mail")
	user.Phone = entry.GetAttributeValue("phone")
	user.Nickname = entry.GetAttributeValue("cn")
	return &user
}

func (s *SsoClient) UserExist(uid string) (bool, error) {
	s.RLock()
	lc := s
	s.RUnlock()

	conn, err := lc.newLdapConn()
	if err != nil {
		return false, err
	}
	defer conn.Close()

	sr, err := s.ldapReq(conn, "&(uid=%s)", uid)
	if err != nil {
		return false, err
	}

	if len(sr.Entries) > 0 {
		return true, nil
	}

	return false, nil
}

func LdapLogin(ctx *ctx.Context, username, pass, roles string, ldap *SsoClient) (*models.User, error) {
	sr, err := ldap.LoginCheck(username, pass)
	if err != nil {
		return nil, err
	}

	user, err := models.UserGetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	if user == nil {
		// default user settings
		user = &models.User{
			Username: username,
			Nickname: username,
		}
	}

	// copy attributes from ldap
	ldap.RLock()
	attrs := ldap.Attributes
	coverAttributes := ldap.CoverAttributes
	ldap.RUnlock()

	if attrs.Nickname != "" {
		user.Nickname = sr.Entries[0].GetAttributeValue(attrs.Nickname)
	}
	if attrs.Email != "" {
		user.Email = sr.Entries[0].GetAttributeValue(attrs.Email)
	}
	if attrs.Phone != "" {
		user.Phone = strings.Replace(sr.Entries[0].GetAttributeValue(attrs.Phone), " ", "", -1)
	}

	if user.Roles == "" {
		user.Roles = roles
	}

	if user.Id > 0 {
		if coverAttributes {
			err := models.DB(ctx).Updates(user).Error
			if err != nil {
				return nil, errors.WithMessage(err, "failed to update user")
			}
		}
		return user, nil
	}

	now := time.Now().Unix()

	user.Password = "******"
	user.Portrait = ""

	user.Contacts = []byte("{}")
	user.CreateAt = now
	user.UpdateAt = now
	user.CreateBy = "ldap"
	user.UpdateBy = "ldap"
	user.Belong = "ldap"

	err = models.DB(ctx).Create(user).Error
	return user, err
}
