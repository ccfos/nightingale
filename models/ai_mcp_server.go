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
	CreatedAt   int64             `json:"created_at"`
	CreatedBy   string            `json:"created_by"`
	UpdatedAt   int64             `json:"updated_at"`
	UpdatedBy   string            `json:"updated_by"`
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
		"enabled", "updated_at", "updated_by").Updates(ref).Error
}

func (s *MCPServer) Delete(c *ctx.Context) error {
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
