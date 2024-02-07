package ldapx

import (
	"crypto/tls"
	"fmt"
	"sync"

	"github.com/go-ldap/ldap/v3"
)

type Config struct {
	Enable          bool
	Host            string
	Port            int
	BaseDn          string
	BindUser        string
	BindPass        string
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
	AuthFilter      string
	Attributes      LdapAttributes
	CoverAttributes bool
	TLS             bool
	StartTLS        bool
	DefaultRoles    []string

	sync.RWMutex
}

type LdapAttributes struct {
	Nickname string `yaml:"nickname"`
	Phone    string `yaml:"phone"`
	Email    string `yaml:"email"`
}

func New(cf Config) *SsoClient {
	var s = &SsoClient{}
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
}

func (s *SsoClient) genLdapAttributeSearchList() []string {
	var ldapAttributes []string

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

func (s *SsoClient) LdapReq(user, pass string) (*ldap.SearchResult, error) {
	var conn *ldap.Conn
	var err error

	s.RLock()
	lc := s
	s.RUnlock()

	addr := fmt.Sprintf("%s:%d", lc.Host, lc.Port)

	if lc.TLS {
		conn, err = ldap.DialTLS("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	} else {
		conn, err = ldap.Dial("tcp", addr)
	}

	if err != nil {
		return nil, fmt.Errorf("ldap.error: cannot dial ldap(%s): %v", addr, err)
	}

	defer conn.Close()

	if !lc.TLS && lc.StartTLS {
		if err := conn.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
			return nil, fmt.Errorf("ldap.error: conn startTLS fail: %v", err)
		}
	}

	// if bindUser is empty, anonymousSearch mode
	if lc.BindUser != "" {
		// BindSearch mode
		if err := conn.Bind(lc.BindUser, lc.BindPass); err != nil {
			return nil, fmt.Errorf("ldap.error: bind ldap fail: %v, use user(%s) to bind", err, lc.BindUser)
		}
	}

	searchRequest := ldap.NewSearchRequest(
		lc.BaseDn, // The base dn to search
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(lc.AuthFilter, user), // The filter to apply
		s.genLdapAttributeSearchList(),   // A list attributes to retrieve
		nil,
	)

	sr, err := conn.Search(searchRequest)
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
