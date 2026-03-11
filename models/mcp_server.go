package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type MCPServer struct {
	Id          int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	Headers     string `json:"headers" gorm:"type:text"`
	Description string `json:"description" gorm:"type:text"`
	Enabled     int    `json:"enabled"`
	CreatedAt   int64  `json:"created_at"`
	CreatedBy   string `json:"created_by"`
	UpdatedAt   int64  `json:"updated_at"`
	UpdatedBy   string `json:"updated_by"`
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
		if err.Error() == "record not found" {
			return nil, nil
		}
		return nil, err
	}
	return &obj, nil
}

func MCPServerGetById(c *ctx.Context, id int64) (*MCPServer, error) {
	return MCPServerGet(c, "id = ?", id)
}

func (s *MCPServer) Create(c *ctx.Context) error {
	now := time.Now().Unix()
	s.CreatedAt = now
	s.UpdatedAt = now
	if s.Enabled == 0 {
		s.Enabled = 1
	}
	return Insert(c, s)
}

func (s *MCPServer) Update(c *ctx.Context, ref MCPServer) error {
	ref.UpdatedAt = time.Now().Unix()
	return DB(c).Model(s).Select("name", "url", "headers", "env_vars", "description",
		"enabled", "updated_at", "updated_by").Updates(ref).Error
}

func (s *MCPServer) Delete(c *ctx.Context) error {
	return DB(c).Where("id = ?", s.Id).Delete(&MCPServer{}).Error
}

func MCPServerGetEnabled(c *ctx.Context) ([]*MCPServer, error) {
	var lst []*MCPServer
	err := DB(c).Where("enabled = 1").Order("id").Find(&lst).Error
	return lst, err
}
