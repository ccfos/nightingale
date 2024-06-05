package models

import (
	"errors"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

type BuiltinPayload struct {
	ID        int64  `json:"id" gorm:"primaryKey;type:bigint;autoIncrement;comment:'unique identifier'"`
	Type      string `json:"type" gorm:"type:varchar(191);not null;index:idx_type,sort:asc;comment:'type of payload'"`                // Alert Dashboard Collet
	Component string `json:"component" gorm:"type:varchar(191);not null;index:idx_component,sort:asc;comment:'component of payload'"` // Host MySQL Redis
	Cate      string `json:"cate" gorm:"type:varchar(191);not null;comment:'category of payload'"`                                    // categraf_v1 telegraf_v1
	Name      string `json:"name" gorm:"type:varchar(191);not null;index:idx_name,sort:asc;comment:'name of payload'"`                //
	Tags      string `json:"tags" gorm:"type:varchar(191);not null;default:'';comment:'tags of payload'"`                             // {"host":"
	Content   string `json:"content" gorm:"type:longtext;not null;comment:'content of payload'"`
	CreatedAt int64  `json:"created_at" gorm:"type:bigint;not null;default:0;comment:'create time'"`
	CreatedBy string `json:"created_by" gorm:"type:varchar(191);not null;default:'';comment:'creator'"`
	UpdatedAt int64  `json:"updated_at" gorm:"type:bigint;not null;default:0;comment:'update time'"`
	UpdatedBy string `json:"updated_by" gorm:"type:varchar(191);not null;default:'';comment:'updater'"`
}

func (bp *BuiltinPayload) TableName() string {
	return "builtin_payloads"
}

func (bp *BuiltinPayload) Verify() error {
	bp.Type = strings.TrimSpace(bp.Type)
	if bp.Type == "" {
		return errors.New("type is blank")
	}

	bp.Component = strings.TrimSpace(bp.Component)
	if bp.Component == "" {
		return errors.New("component is blank")
	}

	if bp.Name == "" {
		return errors.New("name is blank")
	}

	return nil
}

func BuiltinPayloadExists(ctx *ctx.Context, bp *BuiltinPayload) (bool, error) {
	var count int64
	err := DB(ctx).Model(bp).Where("type = ? AND component = ? AND name = ? AND cate = ?", bp.Type, bp.Component, bp.Name, bp.Cate).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (bp *BuiltinPayload) Add(ctx *ctx.Context, username string) error {
	if err := bp.Verify(); err != nil {
		return err
	}
	exists, err := BuiltinPayloadExists(ctx, bp)
	if err != nil {
		return err
	}
	if exists {
		return errors.New("builtin payload already exists")
	}
	now := time.Now().Unix()
	bp.CreatedAt = now
	bp.UpdatedAt = now
	bp.CreatedBy = username
	return Insert(ctx, bp)
}

func (bp *BuiltinPayload) Update(ctx *ctx.Context, req BuiltinPayload) error {
	if err := req.Verify(); err != nil {
		return err
	}

	if bp.Type != req.Type || bp.Component != req.Component || bp.Name != req.Name {
		exists, err := BuiltinPayloadExists(ctx, &req)
		if err != nil {
			return err
		}
		if exists {
			return errors.New("builtin payload already exists")
		}
	}
	req.UpdatedAt = time.Now().Unix()

	return DB(ctx).Model(bp).Select("*").Updates(req).Error
}

func BuiltinPayloadDels(ctx *ctx.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return DB(ctx).Where("id in ?", ids).Delete(new(BuiltinPayload)).Error
}

func BuiltinPayloadGet(ctx *ctx.Context, where string, args ...interface{}) (*BuiltinPayload, error) {
	var bp BuiltinPayload
	err := DB(ctx).Where(where, args...).First(&bp).Error
	if err != nil {
		return nil, err
	}
	return &bp, nil
}

func BuiltinPayloadGets(ctx *ctx.Context, typ, component, cate, query string) ([]*BuiltinPayload, error) {
	session := DB(ctx)
	if typ != "" {
		session = session.Where("type = ?", typ)
	}
	if component != "" {
		session = session.Where("component = ?", component)
	}

	if cate != "" {
		session = session.Where("cate = ?", cate)
	}

	if query != "" {
		arr := strings.Fields(query)
		for i := 0; i < len(arr); i++ {
			qarg := "%" + arr[i] + "%"
			session = session.Where("name like ? or tags like ?", qarg, qarg)
		}
	}

	var lst []*BuiltinPayload
	err := session.Find(&lst).Error
	return lst, err
}

// get cates of BuiltinPayload by type and component, return []string
func BuiltinPayloadCates(ctx *ctx.Context, typ, component string) ([]string, error) {
	var cates []string
	err := DB(ctx).Model(new(BuiltinPayload)).Where("type = ? and component = ?", typ, component).Distinct("cate").Pluck("cate", &cates).Error
	return cates, err
}

// get components of BuiltinPayload by type and cate, return string
func BuiltinPayloadComponents(ctx *ctx.Context, typ, cate string) (string, error) {
	var components []string
	err := DB(ctx).Model(new(BuiltinPayload)).Where("type = ? and cate = ?", typ, cate).Distinct("component").Pluck("component", &components).Error
	if err != nil {
		return "", err
	}

	if len(components) == 0 {
		return "", nil
	}
	return components[0], nil
}
