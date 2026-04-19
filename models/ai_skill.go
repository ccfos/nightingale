package models

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"gorm.io/gorm"
)

type AISkill struct {
	Id            int64             `json:"id" gorm:"primaryKey;autoIncrement"`
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Instructions  string            `json:"instructions" gorm:"type:text"`
	License       string            `json:"license,omitempty"`
	Compatibility string            `json:"compatibility,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty" gorm:"serializer:json"`
	AllowedTools  string            `json:"allowed_tools,omitempty"`
	Enabled       bool              `json:"enabled"`
	CreatedAt     int64             `json:"created_at"`
	CreatedBy     string            `json:"created_by"`
	UpdatedAt     int64             `json:"updated_at"`
	UpdatedBy     string            `json:"updated_by"`

	// Runtime fields, not stored in DB
	Files   []*AISkillFile `json:"files,omitempty" gorm:"-"`
	Builtin bool           `json:"builtin" gorm:"-"`
}

func (s *AISkill) TableName() string {
	return "ai_skill"
}

func (s *AISkill) Verify() error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return fmt.Errorf("name is required")
	}
	s.Instructions = strings.TrimSpace(s.Instructions)
	if s.Instructions == "" {
		return fmt.Errorf("instructions is required")
	}
	return nil
}

func AISkillGets(c *ctx.Context, search string) ([]*AISkill, error) {
	var lst []*AISkill
	session := DB(c).Order("id")
	if search != "" {
		session = session.Where("name like ? or description like ?", "%"+search+"%", "%"+search+"%")
	}
	err := session.Find(&lst).Error
	for _, s := range lst {
		s.Builtin = s.CreatedBy == "system"
	}
	return lst, err
}

func AISkillGet(c *ctx.Context, where string, args ...interface{}) (*AISkill, error) {
	var obj AISkill
	err := DB(c).Where(where, args...).First(&obj).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &obj, nil
}

func AISkillGetById(c *ctx.Context, id int64) (*AISkill, error) {
	return AISkillGet(c, "id = ?", id)
}

func AISkillGetByName(c *ctx.Context, name string) (*AISkill, error) {
	return AISkillGet(c, "name = ?", name)
}

func (s *AISkill) Create(c *ctx.Context) error {
	exist, err := AISkillGetByName(c, s.Name)
	if err != nil {
		return err
	}
	if exist != nil {
		return fmt.Errorf("ai skill name %s already exists", s.Name)
	}

	now := time.Now().Unix()
	s.CreatedAt = now
	s.UpdatedAt = now
	return Insert(c, s)
}

func (s *AISkill) Update(c *ctx.Context, ref AISkill) error {
	if ref.Name != s.Name {
		exist, err := AISkillGetByName(c, ref.Name)
		if err != nil {
			return err
		}
		if exist != nil {
			return fmt.Errorf("ai skill name %s already exists", ref.Name)
		}
	}

	ref.UpdatedAt = time.Now().Unix()
	return DB(c).Model(s).Select("name", "description", "instructions",
		"license", "compatibility", "metadata", "allowed_tools",
		"enabled", "updated_at", "updated_by").Updates(ref).Error
}

func (s *AISkill) Delete(c *ctx.Context) error {
	return DB(c).Where("id = ?", s.Id).Delete(&AISkill{}).Error
}
