package migrate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskHostDoing_SQLiteAllowsSeveralHostsForOneTask(t *testing.T) {
	db := sqliteMigrationDB(t)
	require.NoError(t, db.AutoMigrate(&TaskHostDoing{}))

	require.NoError(t, db.Create(&TaskHostDoing{Id: 42, Host: "web-1", Action: "start"}).Error)
	require.NoError(t, db.Create(&TaskHostDoing{Id: 42, Host: "web-2", Action: "stop"}).Error)

	var rows []TaskHostDoing
	require.NoError(t, db.Order("host").Find(&rows).Error)
	require.Equal(t, []TaskHostDoing{
		{Id: 42, Host: "web-1", Action: "start"},
		{Id: 42, Host: "web-2", Action: "stop"},
	}, rows)
}

func TestTaskHostDoing_SQLiteRejectsDuplicateTaskHost(t *testing.T) {
	db := sqliteMigrationDB(t)
	require.NoError(t, db.AutoMigrate(&TaskHostDoing{}))
	require.NoError(t, db.Create(&TaskHostDoing{Id: 42, Host: "web-1", Action: "start"}).Error)

	err := db.Create(&TaskHostDoing{Id: 42, Host: "web-1", Action: "stop"}).Error

	require.Error(t, err)
	var rows []TaskHostDoing
	require.NoError(t, db.Find(&rows).Error)
	require.Equal(t, []TaskHostDoing{{Id: 42, Host: "web-1", Action: "start"}}, rows)
}

func TestDashAnnotation_SQLiteMigrationPreservesAnnotations(t *testing.T) {
	db := sqliteMigrationDB(t)
	require.NoError(t, db.AutoMigrate(&DashAnnotation{}))
	annotation := DashAnnotation{
		Id: 1, DashboardId: 7, PanelId: "cpu", Tags: `{"env":"prod"}`,
		Description: "deployment", TimeStart: 100, TimeEnd: 200,
	}
	require.NoError(t, db.Create(&annotation).Error)

	require.NoError(t, db.AutoMigrate(&DashAnnotation{}))

	var row DashAnnotation
	require.NoError(t, db.First(&row, 1).Error)
	require.Equal(t, annotation, row)
	require.True(t, db.Migrator().HasTable("dash_annotation"))
}
