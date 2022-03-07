package models

import (
	"github.com/toolkits/pkg/str"
	"gorm.io/gorm"

	"github.com/didi/nightingale/v5/src/storage"
)

const AdminRole = "Admin"

func DB() *gorm.DB {
	return storage.DB
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

func Insert(obj interface{}) error {
	return DB().Create(obj).Error
}

// CryptoPass crypto password use salt
func CryptoPass(raw string) (string, error) {
	salt, err := ConfigsGet("salt")
	if err != nil {
		return "", err
	}

	return str.MD5(salt + "<-*Uk30^96eY*->" + raw), nil
}

type Statistics struct {
	Total       int64 `gorm:"total"`
	LastUpdated int64 `gorm:"last_updated"`
}
