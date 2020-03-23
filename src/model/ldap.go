package model

import (
	"crypto/tls"
	"fmt"

	"github.com/didi/nightingale/src/modules/monapi/config"
	"gopkg.in/ldap.v3"
)

func genLdapAttributeSearchList() []string {
	ldapAttributes := []string{}
	attrs := config.Get().LDAP.Attributes
	if attrs.Dispname != "" {
		ldapAttributes = append(ldapAttributes, attrs.Dispname)
	}
	if attrs.Email != "" {
		ldapAttributes = append(ldapAttributes, attrs.Email)
	}
	if attrs.Phone != "" {
		ldapAttributes = append(ldapAttributes, attrs.Phone)
	}
	if attrs.Im != "" {
		ldapAttributes = append(ldapAttributes, attrs.Im)
	}
	return ldapAttributes
}

func ldapReq(user, pass string) (*ldap.SearchResult, error) {
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
		return nil, fmt.Errorf("cannot dial ldap: %v", err)
	}

	defer conn.Close()

	if !lc.TLS && lc.StartTLS {
		if err := conn.StartTLS(&tls.Config{InsecureSkipVerify: true}); err != nil {
			return nil, fmt.Errorf("ldap.conn startTLS fail: %v", err)
		}
	}
	//if bindUser is empty, anonymousSearch mode
	if lc.BindUser != "" {
		//BindSearch mode
		if err := conn.Bind(lc.BindUser, lc.BindPass); err != nil {
			return nil, fmt.Errorf("bind ldap fail: %v, use %s", err, lc.BindUser)
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
		return nil, fmt.Errorf("ldap search fail: %v", err)
	}

	if len(sr.Entries) == 0 {
		return nil, fmt.Errorf("cannot find such user: %v", user)
	}

	if len(sr.Entries) > 1 {
		return nil, fmt.Errorf("multi users is search, query user: %v", user)
	}

	if err := conn.Bind(sr.Entries[0].DN, pass); err != nil {
		return nil, fmt.Errorf("password error")
	}
	return sr, nil
}
