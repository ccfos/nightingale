package models

import (
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type AISkillFile struct {
	Id        int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	SkillId   int64  `json:"skill_id"`
	Name      string `json:"name"`
	Content   string `json:"content" gorm:"type:mediumtext"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"created_at"`
	CreatedBy string `json:"created_by"`
}

func (f *AISkillFile) TableName() string {
	return "ai_skill_file"
}

func AISkillFileGets(c *ctx.Context, skillId int64) ([]*AISkillFile, error) {
	var lst []*AISkillFile
	err := DB(c).Select("id, skill_id, name, size, created_at, created_by").Where("skill_id = ?", skillId).Order("id").Find(&lst).Error
	return lst, err
}

func AISkillFileGet(c *ctx.Context, where string, args ...interface{}) (*AISkillFile, error) {
	var obj AISkillFile
	err := DB(c).Where(where, args...).First(&obj).Error
	if err != nil {
		if err.Error() == "record not found" {
			return nil, nil
		}
		return nil, err
	}
	return &obj, nil
}

func AISkillFileGetById(c *ctx.Context, id int64) (*AISkillFile, error) {
	return AISkillFileGet(c, "id = ?", id)
}

func (f *AISkillFile) Create(c *ctx.Context) error {
	f.Size = int64(len(f.Content))
	f.CreatedAt = time.Now().Unix()

	// Check file count limit per skill (max 20)
	var count int64
	DB(c).Model(&AISkillFile{}).Where("skill_id = ?", f.SkillId).Count(&count)
	if count >= 20 {
		return fmt.Errorf("max 20 files per skill")
	}

	return Insert(c, f)
}

func (f *AISkillFile) Delete(c *ctx.Context) error {
	return DB(c).Where("id = ?", f.Id).Delete(&AISkillFile{}).Error
}

func AISkillFileDeleteBySkillId(c *ctx.Context, skillId int64) error {
	return DB(c).Where("skill_id = ?", skillId).Delete(&AISkillFile{}).Error
}

func AISkillFileGetContents(c *ctx.Context, skillId int64) ([]*AISkillFile, error) {
	var lst []*AISkillFile
	err := DB(c).Where("skill_id = ?", skillId).Find(&lst).Error
	return lst, err
}
