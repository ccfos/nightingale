package migrate

import (
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
)

func TestDatasourceDescriptionWidenedToText(t *testing.T) {
	db := sqliteMigrationDB(t)
	// Legacy layout from docker/sqlite.sql before description became text.
	require.NoError(t, db.Exec("CREATE TABLE `datasource` (`id` integer primary key autoincrement, `name` varchar(191) not null default '' unique, `description` varchar(255) not null default '')").Error)
	require.NoError(t, db.Exec("INSERT INTO `datasource` (`name`, `description`) VALUES ('legacy', 'short')").Error)

	require.NoError(t, db.AutoMigrate(&Datasource{}))

	cols, err := db.Migrator().ColumnTypes(&Datasource{})
	require.NoError(t, err)
	for _, col := range cols {
		if col.Name() == "description" {
			require.Equal(t, "text", strings.ToLower(col.DatabaseTypeName()))
		}
	}

	var desc string
	require.NoError(t, db.Table("datasource").Where("name = ?", "legacy").Pluck("description", &desc).Error)
	require.Equal(t, "short", desc)

	long := strings.Repeat("d", 1000)
	require.NoError(t, db.Table("datasource").Where("name = ?", "legacy").Update("description", long).Error)
	var got models.Datasource
	require.NoError(t, db.Table("datasource").Select("id", "name", "description").Where("name = ?", "legacy").First(&got).Error)
	require.Equal(t, long, got.Description)
}
