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
	"github.com/toolkits/pkg/container/set"
	"github.com/toolkits/pkg/logger"
)

type Config struct {
	Enable          bool
	Host            string
	Port            int
	BaseDn          string
	BindUser        string
	BindPass        string
	SyncAddUsers    bool
	SyncDelUsers    bool
	SyncInterval    time.Duration
	UserFilter      string
	AuthFilter      string
	Attributes      LdapAttributes
	CoverAttributes bool
	CoverTeams      bool
	TLS             bool
	StartTLS        bool
	DefaultRoles    []string
	DefaultTeams    []int64
	RoleTeamMapping []RoleTeamMapping
}

type SsoClient struct {
	Enable          bool
	Host            string
	Port            int
	BaseDn          string
	BaseDns         []string
	BindUser        string
	BindPass        string
	SyncAdd         bool
	SyncDel         bool
	SyncInterval    time.Duration
	UserFilter      string
	AuthFilter      string
	Attributes      LdapAttributes
	CoverAttributes bool
	CoverTeams      bool
	TLS             bool
	StartTLS        bool
	DefaultRoles    []string
	DefaultTeams    []int64
	RoleTeamMapping map[string]RoleTeamMapping

	Ticker *time.Ticker
	sync.RWMutex
}

type LdapAttributes struct {
	Username string
	Nickname string
	Phone    string
	Email    string
	Group    string // User support is “memberOf” by default
}

type RoleTeamMapping struct {
	DN    string
	Roles []string
	Teams []int64
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
	s.CoverTeams = cf.CoverTeams
	s.TLS = cf.TLS
	s.StartTLS = cf.StartTLS
	s.DefaultRoles = cf.DefaultRoles
	s.DefaultTeams = cf.DefaultTeams
	s.SyncAdd = cf.SyncAddUsers
	s.SyncDel = cf.SyncDelUsers
	s.SyncInterval = cf.SyncInterval
	s.SyncDel = cf.SyncDelUsers
	s.UserFilter = cf.UserFilter

	// Needs to be used to pull the group of LDAP to which the user belongs, that is,
	// the memberOf property of the user needs to be pulled by default
	s.Attributes.Group = "memberOf"

	// Role Mapping and team mapping are configured
	s.RoleTeamMapping = make(map[string]RoleTeamMapping, len(cf.RoleTeamMapping))
	for _, mapping := range cf.RoleTeamMapping {
		s.RoleTeamMapping[mapping.DN] = mapping
	}

	if s.SyncInterval > 0 {
		s.Ticker.Reset(s.SyncInterval * time.Second)
	}

	s.BaseDns = strings.Split(s.BaseDn, "|")
}

func (s *SsoClient) Copy() *SsoClient {
	s.RLock()

	newRoles := make([]string, len(s.DefaultRoles))
	copy(newRoles, s.DefaultRoles)
	newTeams := make([]int64, len(s.DefaultTeams))
	copy(newTeams, s.DefaultTeams)
	lc := *s
	lc.DefaultRoles = newRoles
	lc.DefaultTeams = newTeams

	s.RUnlock()

	return &lc
}

func (s *SsoClient) LoginCheck(user, pass string) (*ldap.SearchResult, error) {
	lc := s.Copy()

	conn, err := lc.newLdapConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	srs, err := lc.ldapReq(conn, lc.AuthFilter, user)
	if err != nil {
		return nil, fmt.Errorf("ldap.error: ldap search fail: %v", err)
	}

	var sr *ldap.SearchResult

	for i := range srs {
		if srs[i] == nil || len(srs[i].Entries) == 0 {
			continue
		}

		// 多个 dn 中，账号的唯一性由 LDAP 保证
		if len(srs[i].Entries) > 1 {
			return nil, fmt.Errorf("ldap.error: search user(%s), multi entries found", user)
		}

		sr = srs[i]

		if err := conn.Bind(srs[i].Entries[0].DN, pass); err != nil {
			return nil, fmt.Errorf("username or password invalid")
		}

		for _, info := range srs[i].Entries[0].Attributes {
			logger.Infof("ldap.info: user(%s) info: %+v", user, info)
		}

		break
	}

	if sr == nil {
		return nil, fmt.Errorf("username or password invalid")
	}

	return sr, nil
}

func (s *SsoClient) newLdapConn() (*ldap.Conn, error) {
	var conn *ldap.Conn
	var err error

	addr := fmt.Sprintf("%s:%d", s.Host, s.Port)

	ldap.DefaultTimeout = time.Second * 10
	if s.TLS {
		conn, err = ldap.DialTLS("tcp", addr, &tls.Config{InsecureSkipVerify: true})
	} else {
		conn, err = ldap.Dial("tcp", addr)
	}

	if err != nil {
		return nil, fmt.Errorf("ldap.error: cannot dial ldap(%s): %v", addr, err)
	}

	conn.SetTimeout(time.Second * 10)

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

func (s *SsoClient) ldapReq(conn *ldap.Conn, filter string, values ...interface{}) ([]*ldap.SearchResult, error) {
	srs := make([]*ldap.SearchResult, 0, len(s.BaseDns))

	for i := range s.BaseDns {
		searchRequest := ldap.NewSearchRequest(
			strings.TrimSpace(s.BaseDns[i]), // The base dn to search
			ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
			fmt.Sprintf(filter, values...), // The filter to apply
			s.genLdapAttributeSearchList(), // A list attributes to retrieve
			nil,
		)
		sr, err := conn.Search(searchRequest)
		if err != nil {
			logger.Errorf("ldap.error: ldap search fail: %v", err)
			continue
		}
		srs = append(srs, sr)
	}

	return srs, nil
}

// GetUserRolesAndTeams Gets the roles and teams of the user
func (s *SsoClient) GetUserRolesAndTeams(entry *ldap.Entry) *RoleTeamMapping {
	lc := s.Copy()

	groups := entry.GetAttributeValues(lc.Attributes.Group)
	rolesSet := set.NewStringSet()
	teamsSet := set.NewInt64Set()
	mapping := lc.RoleTeamMapping

	// Collect DNs and Groups
	dns := append(groups, entry.DN)
	for _, dn := range dns {
		// adds roles to the given set from the specified dn entry.
		if rt, exists := mapping[dn]; exists {
			for _, role := range rt.Roles {
				rolesSet.Add(role)
			}
		}
		// adds teams to the given set from the specified dn entry.
		if rt, exists := mapping[dn]; exists {
			for _, team := range rt.Teams {
				teamsSet.Add(team)
			}
		}
	}

	// Convert sets to slices
	return &RoleTeamMapping{
		DN:    entry.DN,
		Roles: rolesSet.ToSlice(),
		Teams: teamsSet.ToSlice(),
	}
}

func (s *SsoClient) genLdapAttributeSearchList() []string {
	var ldapAttributes []string

	attrs := s.Attributes

	if attrs.Username == "" {
		ldapAttributes = append(ldapAttributes, "uid")
	} else {
		ldapAttributes = append(ldapAttributes, attrs.Username)
	}

	if attrs.Nickname != "" {
		ldapAttributes = append(ldapAttributes, attrs.Nickname)
	}
	if attrs.Email != "" {
		ldapAttributes = append(ldapAttributes, attrs.Email)
	}
	if attrs.Phone != "" {
		ldapAttributes = append(ldapAttributes, attrs.Phone)
	}

	if attrs.Group != "" {
		ldapAttributes = append(ldapAttributes, attrs.Group)
	}

	return ldapAttributes
}

func LdapLogin(ctx *ctx.Context, username, pass string, defaultRoles []string, defaultTeams []int64, ldap *SsoClient) (*models.User, error) {
	sr, err := ldap.LoginCheck(username, pass)
	if err != nil {
		return nil, err
	}

	// copy attributes from ldap
	ldap.RLock()
	attrs := ldap.Attributes
	coverAttributes := ldap.CoverAttributes
	coverTeams := ldap.CoverTeams
	ldap.RUnlock()

	var nickname, email, phone string
	if attrs.Nickname != "" {
		nickname = sr.Entries[0].GetAttributeValue(attrs.Nickname)
	}
	if attrs.Email != "" {
		email = sr.Entries[0].GetAttributeValue(attrs.Email)
	}
	if attrs.Phone != "" {
		phone = strings.Replace(sr.Entries[0].GetAttributeValue(attrs.Phone), " ", "", -1)
	}

	// Gets the roles and teams for this entry
	roleTeamMapping := ldap.GetUserRolesAndTeams(sr.Entries[0])

	user, err := models.UserGetByUsername(ctx, username)
	if err != nil {
		return nil, err
	}

	if user != nil && user.Id > 0 {
		if coverAttributes {
			// need to override the user's basic properties
			updatedFields := user.UpdateSsoFieldsWithRoles("ldap", nickname, phone, email, roleTeamMapping.Roles)
			if err = user.Update(ctx, "update_at", updatedFields...); err != nil {
				return nil, errors.WithMessage(err, "failed to update user")
			}
		}

		if len(roleTeamMapping.Teams) == 0 {
			roleTeamMapping.Teams = defaultTeams
		}

		// Synchronize group information
		if err = models.UserGroupMemberSync(ctx, roleTeamMapping.Teams, user.Id, coverTeams); err != nil {
			logger.Errorf("ldap.error: failed to update user(%s) group member err: %+v", user, err)
		}
	} else {
		user = new(models.User)
		if len(roleTeamMapping.Roles) == 0 {
			// No role mapping is configured, the configured default role is used
			roleTeamMapping.Roles = defaultRoles
		}

		user.FullSsoFields("ldap", username, nickname, phone, email, roleTeamMapping.Roles)
		if err = models.DB(ctx).Create(user).Error; err != nil {
			return nil, errors.WithMessage(err, "failed to add user")
		}

		if len(roleTeamMapping.Teams) == 0 {
			for _, gid := range defaultTeams {
				err = models.UserGroupMemberAdd(ctx, gid, user.Id)
				if err != nil {
					logger.Errorf("user:%v gid:%d UserGroupMemberAdd: %s", user, gid, err)
				}
			}
		}

		if err = models.UserGroupMemberSync(ctx, roleTeamMapping.Teams, user.Id, false); err != nil {
			logger.Errorf("ldap.error: failed to update user(%s) group member err: %+v", user, err)
		}
	}

	return user, nil
}
