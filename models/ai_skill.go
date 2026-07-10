package models

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/slice"
	"gorm.io/gorm"
)

type AISkill struct {
	Id            int64             `json:"id" gorm:"primaryKey;autoIncrement"`
	Name          string            `json:"name"`
	Description   string            `json:"description" gorm:"type:varchar(4096)"`
	Instructions  string            `json:"instructions" gorm:"type:text"`
	License       string            `json:"license,omitempty" gorm:"type:varchar(255)"`
	Compatibility string            `json:"compatibility,omitempty" gorm:"type:varchar(255)"`
	Metadata      map[string]string `json:"metadata,omitempty" gorm:"serializer:json"`
	AllowedTools  string            `json:"allowed_tools,omitempty" gorm:"type:varchar(4096)"`
	Enabled       bool              `json:"enabled"`
	// UserGroupIds 授权团队：成员有权编辑该 skill；Private 授权范围（0-公共 1-私有）
	// 决定 skill 在 AI 对话中的可见性。二者均为空/0 的存量 skill 走创建人/更新人兜底。
	UserGroupIds  []int64           `json:"user_group_ids" gorm:"serializer:json"`
	Private       int               `json:"private" gorm:"not null;default:0"`
	SourceType    string            `json:"source_type" gorm:"type:varchar(16);default:'local'"`
	GitInfo       *AISkillGitInfo   `json:"git_info,omitempty" gorm:"column:git_info;type:text;serializer:json"`
	CreatedAt     int64             `json:"created_at"`
	CreatedBy     string            `json:"created_by"`
	UpdatedAt     int64             `json:"updated_at"`
	UpdatedBy     string            `json:"updated_by"`

	// Runtime fields, not stored in DB
	Files         []*AISkillFile `json:"files,omitempty" gorm:"-"`
	Builtin       bool           `json:"builtin" gorm:"-"`
	HasNewVersion bool           `json:"has_new_version,omitempty" gorm:"-"`
	// CanEdit 按请求用户动态计算：该用户是否有权编辑/删除本 skill。由页面接口盖上，
	// 供前端 gate 增删改按钮，与后端 CanBeEditedBy(403) 同一判定，避免前后端漂移。
	CanEdit bool `json:"can_edit" gorm:"-"`
}

type AISkillGitInfo struct {
	URL           string `json:"url,omitempty"`
	RefType       string `json:"ref_type,omitempty"`
	Ref           string `json:"ref,omitempty"`
	AuthType      string `json:"auth_type,omitempty"`
	Token         string `json:"token,omitempty"`
	Subdir        string `json:"subdir,omitempty"`
	CurrentCommit string `json:"current_commit"`
}

func (s *AISkill) TableName() string {
	return "ai_skill"
}

const (
	AISkillSourceLocal = "local"
	AISkillSourceGit   = "git"
)

func (s *AISkill) SetDefaultSourceType() {
	if s.SourceType == "" {
		s.SourceType = AISkillSourceLocal
	}
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
	if s.Private != 0 && s.Private != 1 {
		return fmt.Errorf("private flag must be 0 or 1")
	}
	if s.Private == 1 && len(s.UserGroupIds) == 0 {
		return fmt.Errorf("user group ids of private skill cannot be empty")
	}
	s.SetDefaultSourceType()
	return nil
}

// CanBeEditedBy reports whether the user may edit or delete this skill.
// Skills carrying an authorized team allow admins and members of that team.
// Legacy skills without a team fall back to admin, creator, or last updater —
// preserving pre-authorization behavior until the skill is re-saved with a team.
func (s *AISkill) CanBeEditedBy(u *User, gids []int64) bool {
	if u.IsAdmin() {
		return true
	}
	if len(s.UserGroupIds) > 0 {
		return slice.HaveIntersection(gids, s.UserGroupIds)
	}
	return u.Username == s.CreatedBy || u.Username == s.UpdatedBy
}

// CanBeViewedBy reports whether the user may see/read this skill's content:
// admins and public skills (Private==0) are always visible; private skills only
// to members of an authorized team.
func (s *AISkill) CanBeViewedBy(u *User, gids []int64) bool {
	if u.IsAdmin() || s.Private == 0 {
		return true
	}
	return slice.HaveIntersection(gids, s.UserGroupIds)
}

// FilterAISkillsVisible keeps the skills a non-admin user may see: public ones
// (Private==0) plus private ones authorized to a team the user belongs to.
func FilterAISkillsVisible(lst []*AISkill, gids []int64) []*AISkill {
	res := make([]*AISkill, 0, len(lst))
	for _, s := range lst {
		if s.Private == 0 || slice.HaveIntersection(gids, s.UserGroupIds) {
			res = append(res, s)
		}
	}
	return res
}

// AISkillHiddenNames returns the names of private skills the user's teams are
// not authorized to see. Used to strip private skills from the on-demand skill
// catalog surfaced in AI conversations so they only reach their teams.
func AISkillHiddenNames(c *ctx.Context, gids []int64) ([]string, error) {
	var lst []*AISkill
	if err := DB(c).Where("private = ?", 1).Find(&lst).Error; err != nil {
		return nil, err
	}
	names := make([]string, 0)
	for _, s := range lst {
		if !slice.HaveIntersection(gids, s.UserGroupIds) {
			names = append(names, s.Name)
		}
	}
	return names, nil
}

func AISkillGets(c *ctx.Context, search string) ([]*AISkill, error) {
	var lst []*AISkill
	session := DB(c).Order("id")
	if search != "" {
		session = session.Where("name like ? or description like ?", "%"+search+"%", "%"+search+"%")
	}
	err := session.Find(&lst).Error
	for _, s := range lst {
		s.SetDefaultSourceType()
		s.Builtin = s.CreatedBy == "system"
		if s.UserGroupIds == nil {
			s.UserGroupIds = []int64{}
		}
	}
	sort.SliceStable(lst, func(i, j int) bool {
		return lst[i].Builtin && !lst[j].Builtin
	})
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
	obj.SetDefaultSourceType()
	if obj.UserGroupIds == nil {
		obj.UserGroupIds = []int64{}
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
	s.SetDefaultSourceType()
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
		"enabled", "user_group_ids", "private", "updated_at", "updated_by").Updates(ref).Error
}

func (s *AISkill) UpdateWithGit(c *ctx.Context, ref AISkill) error {
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
		"license", "compatibility", "metadata", "allowed_tools", "enabled",
		"git_info", "updated_at", "updated_by").Updates(ref).Error
}

func (s *AISkill) UpdateGitFields(c *ctx.Context, ref AISkill) error {
	ref.SetDefaultSourceType()
	ref.UpdatedAt = time.Now().Unix()
	return DB(c).Model(s).Select("source_type", "git_info",
		"updated_at", "updated_by").Updates(ref).Error
}

func (s *AISkill) Delete(c *ctx.Context) error {
	return DB(c).Where("id = ?", s.Id).Delete(&AISkill{}).Error
}

// AISkillsEnabled returns all enabled skills ordered by id. Used by the DB→FS
// skill sync to materialize DB skills into the on-disk registry.
func AISkillsEnabled(c *ctx.Context) ([]*AISkill, error) {
	var lst []*AISkill
	err := DB(c).Where("enabled = ?", true).Order("id").Find(&lst).Error
	return lst, err
}

// AISkillsByIds returns enabled skills whose ids are in the input list.
// Disabled skills are skipped — an agent that binds a disabled skill simply
// loses it from the active set rather than erroring out.
func AISkillsByIds(c *ctx.Context, ids []int64) ([]*AISkill, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var lst []*AISkill
	err := DB(c).Where("id IN ? AND enabled = ?", ids, true).Order("id").Find(&lst).Error
	return lst, err
}

// AISkillNamesByIds is a small projection of AISkillsByIds used when the caller
// only needs skill names (e.g. to populate SkillConfig.SkillNames). Disabled
// skills drop out of the result.
func AISkillNamesByIds(c *ctx.Context, ids []int64) ([]string, error) {
	skills, err := AISkillsByIds(c, ids)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(skills))
	for _, s := range skills {
		names = append(names, s.Name)
	}
	return names, nil
}
