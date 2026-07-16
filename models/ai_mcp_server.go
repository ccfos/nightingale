package models

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/slice"
	"gorm.io/gorm"
)

type MCPServer struct {
	Id          int64             `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string            `json:"name"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers" gorm:"serializer:json"`
	Description string            `json:"description" gorm:"type:text"`
	Enabled     bool              `json:"enabled"`
	// AuthMode: none | header | oauth. Empty (legacy rows) is treated as
	// "header" when Headers is non-empty, else "none".
	AuthMode string `json:"auth_mode"`
	// UserGroupIds are the teams that own this server: members (plus admins) may
	// manage it, and — when Private — are the only ones who can see/use it.
	UserGroupIds []int64 `json:"user_group_ids" gorm:"serializer:json"`
	// Private: 0 = public (visible/usable by everyone), 1 = team-scoped (visible
	// only to UserGroupIds members). Management is always team-scoped regardless.
	Private   int    `json:"private"`
	CreatedAt int64  `json:"created_at"`
	CreatedBy string `json:"created_by"`
	UpdatedAt int64  `json:"updated_at"`
	UpdatedBy string `json:"updated_by"`
	// CanManage is computed per requesting user (not persisted): whether the
	// caller may edit/delete/test this server.
	CanManage bool `json:"can_manage" gorm:"-"`
	// OAuthConnected is computed per request (not persisted): whether an OAuth
	// token is stored for this server. Only meaningful when AuthMode == "oauth";
	// lets the list page surface saved-but-not-yet-authorized servers.
	OAuthConnected bool `json:"oauth_connected" gorm:"-"`
}

// EffectiveAuthMode normalizes the (possibly empty, legacy) AuthMode value.
func (s *MCPServer) EffectiveAuthMode() string {
	switch s.AuthMode {
	case "none", "header", "oauth":
		return s.AuthMode
	default:
		if len(s.Headers) > 0 {
			return "header"
		}
		return "none"
	}
}

// MCPServerOAuth holds the per-server OAuth 2.1 client material and tokens for
// servers with AuthMode == "oauth". 1:1 with MCPServer via ServerId.
// ClientSecret / AccessToken / RefreshToken are stored encrypted at rest (AES-CBC,
// "{{cipher}}" sentinel) — the router encrypts on write and decrypts on use. Unlike
// the git PAT's RSA scheme (router_ai_skill_git.go), a symmetric cipher is used
// because OAuth JWTs exceed RSA's block-size limit (see router_mcp_server_oauth.go).
// They are json:"-" so they never leak through API responses.
type MCPServerOAuth struct {
	Id                    int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	ServerId              int64  `json:"server_id" gorm:"uniqueIndex:uk_server_id"`
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
	Scope                 string `json:"scope"`
	Resource              string `json:"resource"`
	RedirectURI           string `json:"redirect_uri"`
	ClientId              string `json:"client_id"`
	ClientSecret          string `json:"-" gorm:"type:text"`
	AccessToken           string `json:"-" gorm:"type:text"`
	RefreshToken          string `json:"-" gorm:"type:text"`
	TokenType             string `json:"token_type"`
	Expiry                int64  `json:"expiry"` // unix seconds; 0 = unknown/no expiry
	ConnectedBy           string `json:"connected_by"`
	CreatedAt             int64  `json:"created_at"`
	UpdatedAt             int64  `json:"updated_at"`
}

func (o *MCPServerOAuth) TableName() string {
	return "mcp_server_oauth"
}

func (s *MCPServer) TableName() string {
	return "mcp_server"
}

func (s *MCPServer) Verify() error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	s.URL = strings.TrimSpace(s.URL)
	if s.URL == "" {
		return fmt.Errorf("url is required")
	}
	if err := ValidateMCPServerURL(s.URL); err != nil {
		return err
	}
	if s.Private != 0 && s.Private != 1 {
		return fmt.Errorf("private flag must be 0 or 1")
	}
	if s.Private == 1 && len(s.UserGroupIds) == 0 {
		return fmt.Errorf("user group ids of private mcp server cannot be empty")
	}
	return nil
}

// ValidateMCPServerURL requires an absolute http/https URL carrying a host. The
// runtime always speaks Streamable HTTP to this address (mcpServerConfig pins
// MCPTransportHTTP), so a scheme-less "mcp.example.com" or an "ftp://…" address
// can never connect. Reject it at write time — every caller goes through Verify —
// rather than letting it land and then fail silently at tool-discovery time, where
// the only symptom is the server's tools quietly never showing up.
func ValidateMCPServerURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url must start with http:// or https://")
	}
	if u.Host == "" {
		return fmt.Errorf("url must include a host, e.g. https://mcp.example.com/mcp")
	}
	return nil
}

// CanBeManagedBy reports whether the user may manage (edit/delete/test/authorize)
// this server: admins always can; others need a team in common with UserGroupIds.
// Management is team-scoped regardless of Private. Single source of truth for the
// HTTP routes and the AI-chat tools, so the two surfaces can't drift apart.
func (s *MCPServer) CanBeManagedBy(u *User, gids []int64) bool {
	if u == nil {
		return false
	}
	if u.IsAdmin() {
		return true
	}
	return slice.HaveIntersection(gids, s.UserGroupIds)
}

// CanBeUsedBy reports whether the user may use this server's tools in a
// conversation: public servers (Private==0) are usable by everyone; private ones
// only by those who may manage them.
func (s *MCPServer) CanBeUsedBy(u *User, gids []int64) bool {
	if s.Private == 0 {
		return true
	}
	return s.CanBeManagedBy(u, gids)
}

// DB2FE normalizes fields for the frontend: a nil UserGroupIds serializes as
// null, which the multi-select cannot bind — coerce it to an empty slice.
func (s *MCPServer) DB2FE() {
	if s.UserGroupIds == nil {
		s.UserGroupIds = make([]int64, 0)
	}
}

// MaskSecrets strips credential-bearing fields before returning the server to a
// user who may not manage it. A public server is visible to everyone, but its
// Headers can carry an Authorization token that only managers should see — the
// runtime reads headers server-side from the DB, so redacting them here never
// affects usage.
func (s *MCPServer) MaskSecrets() {
	s.Headers = nil
}

func MCPServerGets(c *ctx.Context) ([]*MCPServer, error) {
	var lst []*MCPServer
	err := DB(c).Order("id").Find(&lst).Error
	for _, s := range lst {
		s.DB2FE()
	}
	return lst, err
}

func MCPServerGet(c *ctx.Context, where string, args ...interface{}) (*MCPServer, error) {
	var obj MCPServer
	err := DB(c).Where(where, args...).First(&obj).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	obj.DB2FE()
	return &obj, nil
}

func MCPServerGetById(c *ctx.Context, id int64) (*MCPServer, error) {
	return MCPServerGet(c, "id = ?", id)
}

func MCPServerGetByName(c *ctx.Context, name string) (*MCPServer, error) {
	return MCPServerGet(c, "name = ?", name)
}

func (s *MCPServer) Create(c *ctx.Context) error {
	exist, err := MCPServerGetByName(c, s.Name)
	if err != nil {
		return err
	}
	if exist != nil {
		return fmt.Errorf("mcp server name %s already exists", s.Name)
	}

	now := time.Now().Unix()
	s.CreatedAt = now
	s.UpdatedAt = now
	return Insert(c, s)
}

func (s *MCPServer) Update(c *ctx.Context, ref MCPServer) error {
	if ref.Name != s.Name {
		exist, err := MCPServerGetByName(c, ref.Name)
		if err != nil {
			return err
		}
		if exist != nil {
			return fmt.Errorf("mcp server name %s already exists", ref.Name)
		}
	}

	// Stored OAuth tokens are an authorization against the old config: after a
	// URL change the runtime would keep sending the Bearer token to whatever host
	// the row now points at, and after switching away from oauth the tokens would
	// linger unused. Drop them first (aborting on failure, so a new URL can never
	// reuse an old token) and require an explicit re-authorization. The oauth
	// callback's flip-to-oauth Update keeps the URL and moves *into* oauth, so it
	// never matches here and the freshly saved tokens survive.
	if ref.URL != s.URL || (s.EffectiveAuthMode() == "oauth" && ref.EffectiveAuthMode() != "oauth") {
		if err := MCPServerOAuthDelByServerId(c, s.Id); err != nil {
			return err
		}
	}

	ref.UpdatedAt = time.Now().Unix()
	return DB(c).Model(s).Select("name", "url", "headers", "description",
		"enabled", "auth_mode", "user_group_ids", "private", "updated_at", "updated_by").Updates(ref).Error
}

func (s *MCPServer) Delete(c *ctx.Context) error {
	// Cascade: drop the associated OAuth tokens so a re-created server with the
	// same id can't inherit stale credentials.
	if err := MCPServerOAuthDelByServerId(c, s.Id); err != nil {
		return err
	}
	return DB(c).Where("id = ?", s.Id).Delete(&MCPServer{}).Error
}

func MCPServerGetEnabled(c *ctx.Context) ([]*MCPServer, error) {
	var lst []*MCPServer
	err := DB(c).Where("enabled = ?", true).Order("id").Find(&lst).Error
	return lst, err
}

// MCPServersByIds returns enabled MCP servers whose ids are in the input list.
// Disabled entries are filtered so toggling `enabled=false` effectively detaches
// a server from agents that still reference it, without requiring a DB cleanup
// on the join.
func MCPServersByIds(c *ctx.Context, ids []int64) ([]*MCPServer, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var lst []*MCPServer
	err := DB(c).Where("id IN ? AND enabled = ?", ids, true).Order("id").Find(&lst).Error
	return lst, err
}

func MCPServerOAuthGetByServerId(c *ctx.Context, serverId int64) (*MCPServerOAuth, error) {
	var obj MCPServerOAuth
	err := DB(c).Where("server_id = ?", serverId).First(&obj).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &obj, nil
}

// Save upserts the OAuth record for its ServerId. Callers must have already
// encrypted ClientSecret / AccessToken / RefreshToken.
func (o *MCPServerOAuth) Save(c *ctx.Context) error {
	now := time.Now().Unix()
	o.UpdatedAt = now

	exist, err := MCPServerOAuthGetByServerId(c, o.ServerId)
	if err != nil {
		return err
	}
	if exist == nil {
		o.CreatedAt = now
		return Insert(c, o)
	}
	o.Id = exist.Id
	o.CreatedAt = exist.CreatedAt
	return DB(c).Model(&MCPServerOAuth{}).Where("id = ?", exist.Id).Select(
		"issuer", "authorization_endpoint", "token_endpoint", "registration_endpoint",
		"scope", "resource", "redirect_uri", "client_id", "client_secret",
		"access_token", "refresh_token", "token_type", "expiry", "connected_by",
		"updated_at",
	).Updates(o).Error
}

func MCPServerOAuthDelByServerId(c *ctx.Context, serverId int64) error {
	return DB(c).Where("server_id = ?", serverId).Delete(&MCPServerOAuth{}).Error
}

// MCPServerOAuthInvalidateTokens clears the stored credential of a server the
// token endpoint definitively rejected.
//
// alsoClearClient distinguishes the two verdicts. invalid_grant only kills the
// grant, so the client registration is kept and re-consent can reuse it (a server
// that never supported DCR could not be re-registered at all). invalid_client /
// unauthorized_client rejects the CLIENT, so it must go too — otherwise prepare
// would hand the very same rejected client back to the next authorize attempt and
// the user could never escape the loop; with it cleared, prepare falls back to DCR,
// or tells them to supply a valid client by hand.
//
// expectAccessToken/expectRefreshToken are the exact stored ciphertexts the failing
// request was using, and the update is conditional on BOTH still being there.
// Returns rows affected: 0 means the stored credential is no longer the one that
// failed — a concurrent OAuth callback or another instance's refresh already
// replaced it — and a late "your token is dead" verdict from the OLD credential
// must not destroy the new one, or one stale rejection would knock the server
// offline for the whole org moments after it was renewed.
//
// Matching on the access token alone is not enough: OAuth doesn't promise a fresh
// access token on refresh, so a rotation can keep access=X while moving
// refresh=R1→R2 — the stale verdict would still match X and wipe R2.
func MCPServerOAuthInvalidateTokens(c *ctx.Context, serverId int64, expectAccessToken, expectRefreshToken string, alsoClearClient bool) (int64, error) {
	if expectAccessToken == "" {
		return 0, nil
	}
	updates := map[string]interface{}{
		"access_token":  "",
		"refresh_token": "",
		"updated_at":    time.Now().Unix(),
	}
	if alsoClearClient {
		updates["client_id"] = ""
		updates["client_secret"] = ""
	}
	res := DB(c).Model(&MCPServerOAuth{}).
		Where("server_id = ? AND access_token = ? AND refresh_token = ?", serverId, expectAccessToken, expectRefreshToken).
		Updates(updates)
	return res.RowsAffected, res.Error
}

// MCPServerOAuthConnectedServerIds returns the ids of servers that hold a
// non-empty access token, i.e. whose OAuth connection is established.
func MCPServerOAuthConnectedServerIds(c *ctx.Context) ([]int64, error) {
	var ids []int64
	err := DB(c).Model(&MCPServerOAuth{}).Where("access_token != ''").Pluck("server_id", &ids).Error
	return ids, err
}
