package models

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTaskTplSave_TrimsHostsAndPersistsNormalizedFields(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	require.NoError(t, c.DB.Table("task_tpl_host").AutoMigrate(&TaskTplHost{}))
	tpl := TaskTpl{GroupId: 7, Title: "restart", Script: "echo one\r\necho two"}

	require.NoError(t, tpl.Save(c, []string{" ", " host-a ", ""}))

	row, err := TaskTplGetById(c, tpl.Id)
	require.NoError(t, err)
	require.Equal(t, "echo one\necho two", row.Script)
	require.Equal(t, 30, row.Timeout)
	var hosts []TaskTplHost
	require.NoError(t, c.DB.Table("task_tpl_host").Find(&hosts).Error)
	require.Equal(t, []TaskTplHost{{Id: tpl.Id, Host: "host-a"}}, hosts)
}

func TestTaskTplSave_RejectsDuplicateTitleWithinGroup(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	require.NoError(t, c.DB.Create(&TaskTpl{GroupId: 7, Title: "restart"}).Error)
	tpl := TaskTpl{GroupId: 7, Title: "restart", Script: "echo ok"}

	err := tpl.Save(c, nil)

	require.ErrorContains(t, err, "already exists")
	var count int64
	require.NoError(t, c.DB.Model(&TaskTpl{}).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestTaskTplSave_RollsBackTemplateWhenHostInsertFails(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	require.NoError(t, c.DB.Table("task_tpl_host").AutoMigrate(&TaskTplHost{}))
	failure := errors.New("host storage unavailable")
	require.NoError(t, c.DB.Callback().Create().Before("gorm:create").Register("round5:reject_host", func(tx *gorm.DB) {
		if tx.Statement.Table == "task_tpl_host" {
			tx.AddError(failure)
		}
	}))
	tpl := TaskTpl{GroupId: 7, Title: "restart", Script: "echo ok"}

	err := tpl.Save(c, []string{"host-a"})

	require.ErrorIs(t, err, failure)
	var count int64
	require.NoError(t, c.DB.Model(&TaskTpl{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestTaskTplUpdate_ReplacesHostsAndWritesZeroValues(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	require.NoError(t, c.DB.Table("task_tpl_host").AutoMigrate(&TaskTplHost{}))
	tpl := TaskTpl{GroupId: 7, Title: "restart", Script: "echo old", Batch: 5, AuthLevel: 3}
	require.NoError(t, tpl.Save(c, []string{"host-a"}))
	tpl.Title = "reload"
	tpl.Script = "echo new"
	tpl.Batch = 0
	tpl.AuthLevel = 0
	tpl.UpdateBy = "editor"

	require.NoError(t, tpl.Update(c, []string{" ", " host-b "}))

	row, err := TaskTplGetById(c, tpl.Id)
	require.NoError(t, err)
	require.Equal(t, "reload", row.Title)
	require.Equal(t, "echo new", row.Script)
	require.Zero(t, row.Batch)
	require.Zero(t, row.AuthLevel)
	require.Equal(t, "editor", row.UpdateBy)
	var hosts []TaskTplHost
	require.NoError(t, c.DB.Table("task_tpl_host").Find(&hosts).Error)
	require.Equal(t, []TaskTplHost{{Id: tpl.Id, Host: "host-b"}}, hosts)
}

func TestTaskTplUpdate_RejectsConflictingTitleWithoutChangingHosts(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	require.NoError(t, c.DB.Table("task_tpl_host").AutoMigrate(&TaskTplHost{}))
	tpl := TaskTpl{GroupId: 7, Title: "restart", Script: "echo ok"}
	require.NoError(t, tpl.Save(c, []string{"host-a"}))
	require.NoError(t, c.DB.Create(&TaskTpl{GroupId: 7, Title: "reload"}).Error)
	tpl.Title = "reload"

	err := tpl.Update(c, []string{"host-b"})

	require.ErrorContains(t, err, "already exists")
	row, err := TaskTplGetById(c, tpl.Id)
	require.NoError(t, err)
	require.Equal(t, "restart", row.Title)
	var hosts []TaskTplHost
	require.NoError(t, c.DB.Table("task_tpl_host").Find(&hosts).Error)
	require.Equal(t, []TaskTplHost{{Id: tpl.Id, Host: "host-a"}}, hosts)
}

func TestTaskTplUpdate_RollsBackFieldsAndHostsWhenReplacementFails(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	require.NoError(t, c.DB.Table("task_tpl_host").AutoMigrate(&TaskTplHost{}))
	tpl := TaskTpl{GroupId: 7, Title: "restart", Script: "echo old"}
	require.NoError(t, tpl.Save(c, []string{"host-a"}))
	failure := errors.New("host replacement rejected")
	require.NoError(t, c.DB.Callback().Create().Before("gorm:create").Register("round5:reject_host", func(tx *gorm.DB) {
		if tx.Statement.Table == "task_tpl_host" {
			tx.AddError(failure)
		}
	}))
	tpl.Script = "echo new"

	err := tpl.Update(c, []string{"host-b"})

	require.ErrorIs(t, err, failure)
	row, err := TaskTplGetById(c, tpl.Id)
	require.NoError(t, err)
	require.Equal(t, "echo old", row.Script)
	var hosts []TaskTplHost
	require.NoError(t, c.DB.Table("task_tpl_host").Find(&hosts).Error)
	require.Equal(t, []TaskTplHost{{Id: tpl.Id, Host: "host-a"}}, hosts)
}

func TestTaskTplDel_RemovesOnlyItsTemplateAndHosts(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	require.NoError(t, c.DB.Table("task_tpl_host").AutoMigrate(&TaskTplHost{}))
	first := TaskTpl{GroupId: 7, Title: "restart", Script: "echo ok"}
	second := TaskTpl{GroupId: 7, Title: "reload", Script: "echo ok"}
	require.NoError(t, first.Save(c, []string{"host-a"}))
	require.NoError(t, second.Save(c, []string{"host-b"}))

	require.NoError(t, first.Del(c))

	rows, err := TaskTplGetAll(c)
	require.NoError(t, err)
	require.Equal(t, []*TaskTpl{&second}, rows)
	var hosts []TaskTplHost
	require.NoError(t, c.DB.Table("task_tpl_host").Find(&hosts).Error)
	require.Equal(t, []TaskTplHost{{Id: second.Id, Host: "host-b"}}, hosts)
}

func TestTaskTplAddTags_DeduplicatesAndSortsTags(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	tpl := TaskTpl{Title: "restart", Tags: "zeta "}
	require.NoError(t, c.DB.Create(&tpl).Error)

	require.NoError(t, tpl.AddTags(c, []string{"alpha", "zeta", "alpha"}, "editor"))

	row, err := TaskTplGetById(c, tpl.Id)
	require.NoError(t, err)
	require.Equal(t, "alpha zeta ", row.Tags)
	require.Equal(t, "editor", row.UpdateBy)
}

func TestTaskTplDelTags_PreservesUnselectedTags(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	tpl := TaskTpl{Title: "restart", Tags: "alpha zeta "}
	require.NoError(t, c.DB.Create(&tpl).Error)

	require.NoError(t, tpl.DelTags(c, []string{"alpha", "absent"}, "editor"))

	row, err := TaskTplGetById(c, tpl.Id)
	require.NoError(t, err)
	require.Equal(t, "zeta ", row.Tags)
	require.Equal(t, "editor", row.UpdateBy)
}

func TestTaskTplUpdateGroup_ChangesOnlySelectedTemplate(t *testing.T) {
	c := pipelineContext(t, &TaskTpl{})
	tpl := TaskTpl{GroupId: 7, Title: "restart"}
	require.NoError(t, c.DB.Create(&tpl).Error)
	require.NoError(t, c.DB.Create(&TaskTpl{Id: 99, GroupId: 7}).Error)

	require.NoError(t, tpl.UpdateGroup(c, 8, "editor"))

	row, err := TaskTplGetById(c, tpl.Id)
	require.NoError(t, err)
	require.Equal(t, int64(8), row.GroupId)
	require.Equal(t, "editor", row.UpdateBy)
	other, err := TaskTplGetById(c, 99)
	require.NoError(t, err)
	require.Equal(t, int64(7), other.GroupId)
}
