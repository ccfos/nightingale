package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
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
	AuthMode  string `json:"auth_mode"`
	CreatedAt int64  `json:"created_at"`
	CreatedBy string `json:"created_by"`
	UpdatedAt int64  `json:"updated_at"`
	UpdatedBy string `json:"updated_by"`
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
	ServerId              int64  `json:"server_id" gorm:"uniqueIndex"`
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
	return nil
}

func MCPServerGets(c *ctx.Context) ([]*MCPServer, error) {
	var lst []*MCPServer
	err := DB(c).Order("id").Find(&lst).Error
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

	ref.UpdatedAt = time.Now().Unix()
	return DB(c).Model(s).Select("name", "url", "headers", "description",
		"enabled", "auth_mode", "updated_at", "updated_by").Updates(ref).Error
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
