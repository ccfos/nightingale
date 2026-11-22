package ormx

import (
	"strings"

	"gorm.io/gorm"
)

// IsOpenGaussDB reports whether the postgres connection is backed by openGauss
// or a derivative (PostgreSQL 9.2 kernel), which rejects several SQL constructs
// GORM emits for postgres. Supports openGauss v5 and PanWeiDB 2.0.
func IsOpenGaussDB(db *gorm.DB) bool {
	var version string
	if err := db.Raw("SELECT version()").Scan(&version).Error; err != nil || version == "" {
		return false
	}
	return isOpenGaussVersion(version)
}

// NeedsOpenGaussCompat reports whether db should take the openGauss
// compatibility path. The dialector check comes first so mysql/sqlite never
// run the version() probe, which sqlite does not implement.
func NeedsOpenGaussCompat(db *gorm.DB) bool {
	return db.Dialector.Name() == "postgres" && IsOpenGaussDB(db)
}

// isOpenGaussVersion matches the version banner of openGauss and its
// derivatives (PanWeiDB prints "PanWeiDB(openGauss) ...").
func isOpenGaussVersion(version string) bool {
	return strings.Contains(strings.ToLower(version), "opengauss")
}
