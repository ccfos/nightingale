package models

import (
	"errors"
	"fmt"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"gorm.io/gorm"
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

const maxFilesPerSkill = 200

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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

	var count int64
	DB(c).Model(&AISkillFile{}).Where("skill_id = ?", f.SkillId).Count(&count)
	if count >= maxFilesPerSkill {
		return fmt.Errorf("max %d files per skill", maxFilesPerSkill)
	}

	return Insert(c, f)
}


// BatchUpsert batch-upserts files for a given skill within a single transaction.
// When fullSync is true, existing files not present in the incoming list are deleted (full replace).
func AISkillFileBatchUpsert(c *ctx.Context, skillId int64, files []*AISkillFile, fullSync bool) error {
	return DB(c).Transaction(func(tx *gorm.DB) error {
		var existingFiles []*AISkillFile
		if err := tx.Select("id, name").Where("skill_id = ?", skillId).Find(&existingFiles).Error; err != nil {
			return err
		}

		existingMap := make(map[string]int64, len(existingFiles))
		for _, ef := range existingFiles {
			existingMap[ef.Name] = ef.Id
		}

		incomingNames := make(map[string]struct{}, len(files))
		for _, f := range files {
			incomingNames[f.Name] = struct{}{}
		}

		if fullSync {
			var staleIds []int64
			for _, ef := range existingFiles {
				if _, ok := incomingNames[ef.Name]; !ok {
					staleIds = append(staleIds, ef.Id)
				}
			}
			if len(staleIds) > 0 {
				if err := tx.Where("id IN ?", staleIds).Delete(&AISkillFile{}).Error; err != nil {
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
				if err := tx.Model(&AISkillFile{Id: existId}).Updates(map[string]interface{}{
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

		// Re-count inside the transaction to narrow the TOCTOU window vs. concurrent writers.
		var totalCount int64
		if err := tx.Model(&AISkillFile{}).Where("skill_id = ?", skillId).Count(&totalCount).Error; err != nil {
			return err
		}
		if totalCount+int64(len(toInsert)) > maxFilesPerSkill {
			return fmt.Errorf("max %d files per skill, current: %d, importing: %d", maxFilesPerSkill, totalCount, len(toInsert))
		}

		return tx.Create(&toInsert).Error
	})
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

// AISkillFilesBySkillIds returns all files for the given skill ids in a single
// query, grouped by skill id. Used by the startup skill sync to avoid the N+1
// round-trip that the single-skill helper would produce.
//
// Empty input returns an empty map (not nil) so callers can key into it without
// a nil-check.
func AISkillFilesBySkillIds(c *ctx.Context, ids []int64) (map[int64][]*AISkillFile, error) {
	out := make(map[int64][]*AISkillFile, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var lst []*AISkillFile
	if err := DB(c).Where("skill_id IN ?", ids).Order("skill_id, id").Find(&lst).Error; err != nil {
		return nil, err
	}
	for _, f := range lst {
		out[f.SkillId] = append(out[f.SkillId], f)
	}
	return out, nil
}
