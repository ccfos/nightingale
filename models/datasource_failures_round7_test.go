package models

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestInitEmbeddedTSDBDatasource_InvalidStoredConfigIsPreserved(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	original := Datasource{Id: 1, Name: "embedded", Identifier: EmbeddedTSDBIdentifier, Settings: "{", HTTP: "{}"}
	require.NoError(t, c.DB.Create(&original).Error)

	InitEmbeddedTSDBDatasource(c, "http://127.0.0.1:17000", "", "", false)

	var stored Datasource
	require.NoError(t, c.DB.First(&stored, 1).Error)
	require.Equal(t, original.Settings, stored.Settings)
	require.Equal(t, original.HTTP, stored.HTTP)
	require.Equal(t, original.Name, stored.Name)
}

func TestInitEmbeddedTSDBDatasource_UpdateFailurePreservesConnection(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	original := Datasource{
		Id: 1, Name: "embedded", Identifier: EmbeddedTSDBIdentifier,
		Settings: `{}`, HTTP: `{"url":"http://127.0.0.1:17000"}`,
	}
	require.NoError(t, c.DB.Create(&original).Error)
	failure := errors.New("connection update rejected")
	const callback = "round7:reject_embedded_update"
	require.NoError(t, c.DB.Callback().Update().Before("gorm:update").Register(callback, func(tx *gorm.DB) {
		tx.AddError(failure)
	}))
	t.Cleanup(func() { c.DB.Callback().Update().Remove(callback) })

	InitEmbeddedTSDBDatasource(c, "http://127.0.0.1:18000", "", "", false)

	var stored Datasource
	require.NoError(t, c.DB.First(&stored, 1).Error)
	require.Equal(t, original.HTTP, stored.HTTP)
	require.Equal(t, original.Settings, stored.Settings)
}

func TestInitEmbeddedTSDBDatasource_CreateFailureLeavesNoPartialDatasource(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	failure := errors.New("datasource insert rejected")
	const callback = "round7:reject_embedded_create"
	require.NoError(t, c.DB.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		tx.AddError(failure)
	}))
	t.Cleanup(func() { c.DB.Callback().Create().Remove(callback) })

	InitEmbeddedTSDBDatasource(c, "http://127.0.0.1:17000", "", "", false)

	var count int64
	require.NoError(t, c.DB.Model(&Datasource{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestDatasourceGetMap_DatabaseFailureReturnsNoPartialMap(t *testing.T) {
	c := pipelineContext(t, &Datasource{})
	failure := errors.New("datasource query rejected")
	const callback = "round7:reject_datasource_query"
	require.NoError(t, c.DB.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		tx.AddError(failure)
	}))
	t.Cleanup(func() { c.DB.Callback().Query().Remove(callback) })

	datasources, err := DatasourceGetMap(c)

	require.ErrorIs(t, err, failure)
	require.Nil(t, datasources)
}

func TestAlertRuleUpdateColumn_UnserializableDatasourceIDsLeaveRuleUnchanged(t *testing.T) {
	c := pipelineContext(t, &AlertRule{})
	rule := AlertRule{Id: 1, DatasourceIds: "[7]"}
	require.NoError(t, c.DB.Create(&rule).Error)

	err := rule.UpdateColumn(c, "datasource_ids", make(chan int))

	require.ErrorContains(t, err, "unsupported type: chan int")
	var stored AlertRule
	require.NoError(t, c.DB.First(&stored, 1).Error)
	require.Equal(t, "[7]", stored.DatasourceIds)
}
