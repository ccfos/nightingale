package models

import (
	"errors"
	"strings"
	"time"

	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

var (
	ErrSavedViewNameEmpty = errors.New("saved view name is blank")
	ErrSavedViewPageEmpty = errors.New("saved view page is blank")
	ErrSavedViewNotFound  = errors.New("saved view not found")
)

// SavedView 保存的查询视图
type SavedView struct {
	Id         int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	Name       string  `json:"name" gorm:"type:varchar(255);not null"`
	Page       string  `json:"page" gorm:"type:varchar(64);not null;index"`
	Filter     string  `json:"filter" gorm:"type:text"`
	PublicCate int     `json:"public_cate" gorm:"default:0"` // 0: self, 1: team, 2: all
	Gids       []int64 `json:"gids" gorm:"column:gids;type:text;serializer:json"`
	CreateAt   int64   `json:"create_at" gorm:"type:bigint;not null;default:0"`
	CreateBy   string  `json:"create_by" gorm:"type:varchar(64);index"`
	UpdateAt   int64   `json:"update_at" gorm:"type:bigint;not null;default:0"`
	UpdateBy   string  `json:"update_by" gorm:"type:varchar(64)"`

	// 查询时填充的字段
	IsFavorite bool `json:"is_favorite" gorm:"-"`
}

func (SavedView) TableName() string {
	return "saved_view"
}

func (sv *SavedView) Verify() error {
	sv.Name = strings.TrimSpace(sv.Name)
	if sv.Name == "" {
		return ErrSavedViewNameEmpty
	}
	if sv.Page == "" {
		return ErrSavedViewPageEmpty
	}
	return nil
}

// SavedViewAdd 创建保存的视图
func SavedViewAdd(c *ctx.Context, sv *SavedView) error {
	if err := sv.Verify(); err != nil {
		return err
	}
	now := time.Now().Unix()
	sv.CreateAt = now
	sv.UpdateAt = now
	return Insert(c, sv)
}

// SavedViewUpdate 更新保存的视图
func SavedViewUpdate(c *ctx.Context, sv *SavedView, username string) error {
	if err := sv.Verify(); err != nil {
		return err
	}
	sv.UpdateAt = time.Now().Unix()
	sv.UpdateBy = username
	return DB(c).Model(sv).Select("name", "filter", "public_cate", "gids", "update_at", "update_by").Updates(sv).Error
}

// SavedViewDel 删除保存的视图
func SavedViewDel(c *ctx.Context, id int64) error {
	// 先删除收藏关联
	if err := DB(c).Where("view_id = ?", id).Delete(&UserViewFavorite{}).Error; err != nil {
		return err
	}
	return DB(c).Where("id = ?", id).Delete(&SavedView{}).Error
}

// SavedViewGetById 根据 ID 获取视图
func SavedViewGetById(c *ctx.Context, id int64) (*SavedView, error) {
	var sv SavedView
	err := DB(c).Where("id = ?", id).First(&sv).Error
	if err != nil {
		return nil, err
	}
	return &sv, nil
}

// SavedViewGets 获取视图列表
func SavedViewGets(c *ctx.Context, page string) ([]SavedView, error) {
	var views []SavedView

	session := DB(c).Where("page = ?", page)

	if err := session.Order("update_at DESC").Find(&views).Error; err != nil {
		return nil, err
	}

	return views, nil
}

// SavedViewFavoriteGetByUserId 获取用户收藏的视图ID列表
func SavedViewFavoriteGetByUserId(c *ctx.Context, userId int64) (map[int64]bool, error) {
	var favorites []UserViewFavorite
	if err := DB(c).Where("user_id = ?", userId).Find(&favorites).Error; err != nil {
		return nil, err
	}

	result := make(map[int64]bool)
	for _, f := range favorites {
		result[f.ViewId] = true
	}
	return result, nil
}

// UserViewFavorite 用户视图收藏
type UserViewFavorite struct {
	Id       int64 `json:"id" gorm:"primaryKey;autoIncrement"`
	ViewId   int64 `json:"view_id" gorm:"index"`
	UserId   int64 `json:"user_id" gorm:"index"`
	CreateAt int64 `json:"create_at"`
}

func (UserViewFavorite) TableName() string {
	return "user_view_favorite"
}

// UserViewFavoriteAdd 添加收藏
func UserViewFavoriteAdd(c *ctx.Context, viewId, userId int64) error {
	// 检查视图是否存在
	var count int64
	if err := DB(c).Model(&SavedView{}).Where("id = ?", viewId).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrSavedViewNotFound
	}

	// 检查是否已收藏
	if err := DB(c).Model(&UserViewFavorite{}).Where("view_id = ? AND user_id = ?", viewId, userId).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil // 已收藏，直接返回成功
	}

	fav := &UserViewFavorite{
		ViewId:   viewId,
		UserId:   userId,
		CreateAt: time.Now().Unix(),
	}
	return DB(c).Create(fav).Error
}

// UserViewFavoriteDel 取消收藏
func UserViewFavoriteDel(c *ctx.Context, viewId, userId int64) error {
	return DB(c).Where("view_id = ? AND user_id = ?", viewId, userId).Delete(&UserViewFavorite{}).Error
}
