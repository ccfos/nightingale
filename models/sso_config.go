package models

import "github.com/ccfos/nightingale/v6/pkg/ctx"

type SsoConfig struct {
	Id      int64  `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (b *SsoConfig) TableName() string {
	return "sso_config"
}

// get all sso_config
func SsoConfigGets(c *ctx.Context) ([]SsoConfig, error) {
	var lst []SsoConfig
	err := DB(c).Find(&lst).Error
	return lst, err
}

// 创建 builtin_cate
func (b *SsoConfig) Create(c *ctx.Context) error {
	return Insert(c, b)
}

func (b *SsoConfig) Update(c *ctx.Context) error {
	return DB(c).Model(b).Select("content").Updates(b).Error
}

// get sso_config coutn by name
func SsoConfigCountByName(c *ctx.Context, name string) (int64, error) {
	var count int64
	err := DB(c).Model(&SsoConfig{}).Where("name = ?", name).Count(&count).Error
	return count, err
}
