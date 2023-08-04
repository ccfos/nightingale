package models

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
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

// IntArray Gorm 自定义数据类型, 用于直接存放 int 数组
type IntArray []int64

func (a IntArray) Value() (driver.Value, error) {
	if len(a) == 0 {
		return nil, nil
	}
	// 将 int 数组转换为逗号分隔的字符串
	j, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}

	return string(j), nil

}

func (a *IntArray) Scan(value interface{}) error {
	if value == nil {
		*a = nil
		return nil
	}
	// 将逗号分隔的字符串转换为 int 数组
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan IntArray: %v", value)
	}

	ints := make([]int64, 0)

	if err := json.Unmarshal(b, &ints); err != nil {
		return fmt.Errorf("failed to scan IntArray: %v", err)
	}
	*a = ints
	return nil
}

type StringArray []string

func (s StringArray) Value() (driver.Value, error) {
	if len(s) == 0 {
		return nil, nil
	}
	// 将 int 数组转换为逗号分隔的字符串
	j, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return string(j), nil
}

func (s *StringArray) Scan(value interface{}) error {
	if value == nil {
		*s = nil
		return nil
	}
	// 将逗号分隔的字符串转换为 string 数组
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan StringArray: %v", value)
	}
	strings := make([]string, 0)
	if err := json.Unmarshal(b, &strings); err != nil {
		return fmt.Errorf("failed to scan StringArray: %v", err)
	}
	*s = strings
	return nil
}

type StringMap map[string]string

func (m StringMap) Value() (driver.Value, error) {
	if len(m) == 0 {
		return nil, nil
	}
	// 将 int 数组转换为逗号分隔的字符串
	j, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return string(j), nil
}

func (m *StringMap) Scan(value interface{}) error {
	if value == nil {
		*m = nil
		return nil
	}
	// 使用 JSON 解码 map[string]string
	b, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("failed to scan StringMap: %v", value)
	}
	var data map[string]string
	if err := json.Unmarshal(b, &data); err != nil {
		return fmt.Errorf("failed to scan StringMap: %v", err)
	}
	*m = data
	return nil
}
