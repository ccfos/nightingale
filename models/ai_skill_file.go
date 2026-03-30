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
	UpdatedAt int64  `json:"updated_at"`
	UpdatedBy string `json:"updated_by"`
}

func (f *AISkillFile) TableName() string {
	return "ai_skill_file"
}

// PostgresAISkillFile is the PostgreSQL-compatible variant of AISkillFile.
// PostgreSQL does not support mediumtext; its text type is unlimited.
type PostgresAISkillFile struct {
	Id        int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	SkillId   int64  `json:"skill_id"`
	Name      string `json:"name"`
	Content   string `json:"content" gorm:"type:text"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"created_at"`
	CreatedBy string `json:"created_by"`
	UpdatedAt int64  `json:"updated_at"`
	UpdatedBy string `json:"updated_by"`
}

func (f *PostgresAISkillFile) TableName() string {
	return "ai_skill_file"
}

func AISkillFileGets(c *ctx.Context, skillId int64) ([]*AISkillFile, error) {
	var lst []*AISkillFile
	err := DB(c).Select("id, skill_id, name, size, created_at, created_by, updated_at, updated_by").Where("skill_id = ?", skillId).Order("id").Find(&lst).Error
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
	now := time.Now().Unix()
	f.Size = int64(len(f.Content))
	f.CreatedAt = now
	f.UpdatedAt = now
	f.UpdatedBy = f.CreatedBy

	// Check file count limit per skill (max 50)
	var count int64
	DB(c).Model(&AISkillFile{}).Where("skill_id = ?", f.SkillId).Count(&count)
	if count >= 50 {
		return fmt.Errorf("max 50 files per skill")
	}

	return Insert(c, f)
}


// BatchUpsert batch-upserts files for a given skill.
// When fullSync is true, existing files not present in the incoming list are deleted (full replace).
// 1 SELECT to find all existing files, then updates/inserts/deletes as needed.
func AISkillFileBatchUpsert(c *ctx.Context, skillId int64, files []*AISkillFile, fullSync bool) error {
	// Query ALL existing files for this skill (not just matching names)
	var existingFiles []*AISkillFile
	err := DB(c).Select("id, name").Where("skill_id = ?", skillId).Find(&existingFiles).Error
	if err != nil {
		return err
	}

	existingMap := make(map[string]int64, len(existingFiles))
	for _, ef := range existingFiles {
		existingMap[ef.Name] = ef.Id
	}

	// Collect incoming file names for stale file detection
	incomingNames := make(map[string]struct{}, len(files))
	for _, f := range files {
		incomingNames[f.Name] = struct{}{}
	}

	// Delete files not in the new archive
	if fullSync {
		var staleIds []int64
		for _, ef := range existingFiles {
			if _, ok := incomingNames[ef.Name]; !ok {
				staleIds = append(staleIds, ef.Id)
			}
		}
		if len(staleIds) > 0 {
			if err := DB(c).Where("id IN ?", staleIds).Delete(&AISkillFile{}).Error; err != nil {
				return err
			}
		}
	}

	now := time.Now().Unix()
	var toInsert []*AISkillFile

	for _, f := range files {
		f.SkillId = skillId
		f.Size = int64(len(f.Content))
		f.CreatedAt = now
		f.UpdatedAt = now
		f.UpdatedBy = f.CreatedBy

		if existId, ok := existingMap[f.Name]; ok {
			// Update: only change content/size and updated fields, preserve created fields
			if err := DB(c).Model(&AISkillFile{Id: existId}).Updates(map[string]interface{}{
				"content":    f.Content,
				"size":       f.Size,
				"updated_at": now,
				"updated_by": f.CreatedBy,
			}).Error; err != nil {
				return err
			}
		} else {
			toInsert = append(toInsert, f)
		}
	}

	if len(toInsert) == 0 {
		return nil
	}

	// Check file count limit: existing (not being replaced) + new inserts
	var totalCount int64
	DB(c).Model(&AISkillFile{}).Where("skill_id = ?", skillId).Count(&totalCount)
	if totalCount+int64(len(toInsert)) > 50 {
		return fmt.Errorf("max 50 files per skill, current: %d, importing: %d", totalCount, len(toInsert))
	}

	return DB(c).Create(&toInsert).Error
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
