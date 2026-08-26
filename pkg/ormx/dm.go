package ormx

import (
	"fmt"
	"net/url"
	"strings"

	dameng "github.com/godoes/gorm-dameng"
	"gorm.io/gorm"
)

// 达梦以 schema 而非 database 为隔离单位，DSN 形如
// dm://user:password@host:5236[?schema=XXX]。省略 schema 时，当前 schema 即登录用户名。
//
// 注意：带 ?schema=XXX 连接一个尚不存在的 schema 时，达梦会直接重置连接而不是返回
// SQL 错误，所以判断存在性与建 schema 都必须先用不带 schema 参数的 DSN 连上去。

// dmDialector 构造达梦 dialector。
//
// VarcharSizeIsCharLength 让 size:N 生成 VARCHAR(N CHAR) 而非 N 个字节。达梦实例的
// LENGTH_IN_CHAR 是建库期参数、事后不可改，开启该选项后无论客户建库时怎么选，列宽语义
// 都与 MySQL 一致（N 个字符），也就不会因为列比现状窄而触发缩窄 ALTER。
func dmDialector(dsn string) gorm.Dialector {
	return dameng.New(dameng.Config{
		DSN:                     dsn,
		VarcharSizeIsCharLength: true,
	})
}

// splitDMSchema 从 DSN 中分离出 schema 名与去掉 schema 参数后的 DSN。
// schema 参数缺省时回落到登录用户名，与达梦自身的默认行为一致。
func splitDMSchema(dsn string) (schema, dsnWithoutSchema string, err error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse dm dsn: %v", err)
	}

	q := u.Query()
	schema = q.Get("schema")
	if schema != "" {
		q.Del("schema")
	} else if u.User != nil {
		schema = u.User.Username()
	}
	if schema == "" {
		return "", "", fmt.Errorf("cannot determine dm schema from dsn, please specify ?schema=xxx")
	}

	u.RawQuery = q.Encode()
	return schema, u.String(), nil
}

func checkDMDatabaseExist(c DBConfig, gconfig *gorm.Config) (bool, error) {
	schema, dsn, err := splitDMSchema(c.DSN)
	if err != nil {
		return false, err
	}

	db, err := gorm.Open(dmDialector(dsn), gconfig)
	if err != nil {
		return false, fmt.Errorf("failed to open dm connection: %v", err)
	}
	defer closeGormDB(db)

	var count int64
	if err := db.Raw(
		"SELECT COUNT(*) FROM DBA_OBJECTS WHERE OBJECT_TYPE = 'SCH' AND OBJECT_NAME = ?",
		schema,
	).Scan(&count).Error; err != nil {
		return false, fmt.Errorf("failed to query dm schema: %v", err)
	}

	return count > 0, nil
}

func createDMDatabase(c DBConfig, gconfig *gorm.Config) error {
	schema, dsn, err := splitDMSchema(c.DSN)
	if err != nil {
		return err
	}

	owner := schema
	if u, e := url.Parse(c.DSN); e == nil && u.User != nil && u.User.Username() != "" {
		owner = u.User.Username()
	}

	db, err := gorm.Open(dmDialector(dsn), gconfig)
	if err != nil {
		return fmt.Errorf("failed to open dm connection: %v", err)
	}
	defer closeGormDB(db)

	// schema 名不能参数化，这里限制成标识符字面量以免拼接出问题
	if !isSafeDMIdentifier(schema) || !isSafeDMIdentifier(owner) {
		return fmt.Errorf("invalid dm schema or owner name: %s / %s", schema, owner)
	}

	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA "%s" AUTHORIZATION "%s"`, schema, owner)).Error; err != nil {
		return fmt.Errorf("failed to create dm schema %s: %v", schema, err)
	}

	return nil
}

func isSafeDMIdentifier(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '$' {
			continue
		}
		return false
	}
	return true
}

func closeGormDB(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// isDM 供其他包判断当前方言，避免各处硬编码字符串。
func IsDM(db *gorm.DB) bool {
	return db != nil && db.Dialector != nil && strings.EqualFold(db.Dialector.Name(), "dm")
}
