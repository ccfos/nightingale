package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"

	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"
)

const AdminRole = "Admin"

// if rule's cluster field contains `ClusterAll`, means it take effect in all clusters
const DatasourceIdAll = 0

func DB(ctx *ctx.Context) *gorm.DB {
	return ctx.DB
}

func Count(tx *gorm.DB) (int64, error) {
	var cnt int64
	err := tx.Count(&cnt).Error
	return cnt, err
}

func Exists(tx *gorm.DB) (bool, error) {
	num, err := Count(tx)
	return num > 0, err
}

func Insert(ctx *ctx.Context, obj interface{}) error {
	return DB(ctx).Create(obj).Error
}

// CryptoPass crypto password use salt
func CryptoPass(ctx *ctx.Context, raw string) (string, error) {
	salt, err := ConfigsGet(ctx, "salt")
	if err != nil {
		return "", err
	}

	return str.MD5(salt + "<-*Uk30^96eY*->" + raw), nil
}

type Statistics struct {
	Total       int64 `gorm:"total"`
	LastUpdated int64 `gorm:"last_updated"`
}

func StatisticsGet[T any](ctx *ctx.Context, model T) (*Statistics, error) {
	var stats []*Statistics
	session := DB(ctx).Model(model).Select("count(*) as total", "max(update_at) as last_updated")

	err := session.Find(&stats).Error
	if err != nil {
		return nil, err
	}

	return stats[0], nil
}

func MatchDatasource(ids []int64, id int64) bool {
	if id == DatasourceIdAll {
		return true
	}

	for _, i := range ids {
		if i == id {
			return true
		}
	}
	return false
}

func IsAllDatasource(datasourceIds []int64) bool {
	for _, id := range datasourceIds {
		if id == 0 {
			return true
		}
	}
	return false
}

type LabelAndKey struct {
	Label string `json:"label"`
	Key   string `json:"key"`
}

func LabelAndKeyHasKey(keys []LabelAndKey, key string) bool {
	for i := 0; i < len(keys); i++ {
		if keys[i].Key == key {
			return true
		}
	}
	return false
}
