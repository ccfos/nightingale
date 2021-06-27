package models

import (
	"crypto/tls"
	"fmt"

	"github.com/go-ldap/ldap/v3"
	"github.com/toolkits/pkg/logger"
)

type LdapSection struct {
	Enable          bool           `yaml:"enable"`
	Host            string         `yaml:"host"`
	Port            int            `yaml:"port"`
	BaseDn          string         `yaml:"baseDn"`
	BindUser        string         `yaml:"bindUser"`
	BindPass        string         `yaml:"bindPass"`
	AuthFilter      string         `yaml:"authFilter"`
	Attributes      ldapAttributes `yaml:"attributes"`
	CoverAttributes bool           `yaml:"coverAttributes"`
	TLS             bool           `yaml:"tls"`
	StartTLS        bool           `yaml:"startTLS"`
}

type ldapAttributes struct {
	Nickname string `yaml:"nickname"`
	Phone    string `yaml:"phone"`
	Email    string `yaml:"email"`
}

var LDAP LdapSection

func InitLdap(ldap LdapSection) {
	LDAP = ldap
}

func genLdapAttributeSearchList() []string {
	var ldapAttributes []string
	attrs := LDAP.Attributes
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

func ldapReq(user, pass string) (*ldap.SearchResult, error) {
	var conn *ldap.Conn
	var err error
	lc := LDAP
	addr := fmt.Sprintf("%s:%d", lc.Host, lc.Port)

	if lc.TLS {
		conn, err = ldap.DialTLS("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	} else {
		conn, err = ldap.Dial("tcp", addr)
	}

	if err != nil {
		logger.Errorf("ldap.error: cannot dial ldap(%s): %v", addr, err)
		return nil, internalServerError
	}

	defer conn.Close()

	if !lc.TLS && lc.StartTLS {
		if err := conn.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
			logger.Errorf("ldap.error: conn startTLS fail: %v", err)
			return nil, internalServerError
		}
	}

	// if bindUser is empty, anonymousSearch mode
	if lc.BindUser != "" {
		// BindSearch mode
		if err := conn.Bind(lc.BindUser, lc.BindPass); err != nil {
			logger.Errorf("ldap.error: bind ldap fail: %v, use user(%s) to bind", err, lc.BindUser)
			return nil, internalServerError
		}
	}

	searchRequest := ldap.NewSearchRequest(
		lc.BaseDn, // The base dn to search
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		fmt.Sprintf(lc.AuthFilter, user), // The filter to apply
		genLdapAttributeSearchList(),     // A list attributes to retrieve
		nil,
	)

	sr, err := conn.Search(searchRequest)
	if err != nil {
		logger.Errorf("ldap.error: ldap search fail: %v", err)
		return nil, internalServerError
	}

	if len(sr.Entries) == 0 {
		logger.Infof("ldap auth fail, no such user: %s", user)
		return nil, loginFailError
	}

	if len(sr.Entries) > 1 {
		logger.Errorf("ldap.error: search user(%s), multi entries found", user)
		return nil, internalServerError
	}

	if err := conn.Bind(sr.Entries[0].DN, pass); err != nil {
		logger.Infof("ldap auth fail, password error, user: %s", user)
		return nil, loginFailError
	}
	return sr, nil
}
