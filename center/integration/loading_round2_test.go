package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func round2IntegrationStore(t *testing.T) (*ctx.Context, string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(
		&models.BuiltinComponent{}, &models.BuiltinPayload{}, &models.BuiltinMetric{}, &models.Configs{},
	))
	old := BuiltinPayloadInFile
	t.Cleanup(func() { BuiltinPayloadInFile = old })

	dir, err := os.MkdirTemp("/tmp", "n9e-integration-round2-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(dir)) })
	// Stage real repository assets so loader write-back can never modify the checkout.
	for _, relative := range []string{
		"icon/mysql.png", "markdown/README.md", "markdown/README.en_US.md", "i18n/en_US.json",
		"alerts/mysql_by_categraf.json", "dashboards/mysql_by_categraf.json", "metrics/categraf-base.json",
	} {
		content, err := os.ReadFile(filepath.Join("..", "..", "integrations", "MySQL", relative))
		require.NoError(t, err)
		target := filepath.Join(dir, "MySQL", relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(t, os.WriteFile(target, content, 0o600))
	}
	return ctx.NewContext(context.Background(), db, true), dir
}

func TestInit_LoadsRepositoryComponentAndLocalizedPayloads(t *testing.T) {
	c, dir := round2IntegrationStore(t)

	Init(c, dir)

	component, err := models.BuiltinComponentGet(c, "ident = ?", "MySQL")
	require.NoError(t, err)
	require.NotNil(t, component)
	require.Equal(t, "/api/n9e/integrations/icon/MySQL/mysql.png", component.Logo)
	require.NotEmpty(t, component.Readme)
	source, err := os.ReadFile(filepath.Join(dir, "MySQL", "alerts", "mysql_by_categraf.json"))
	require.NoError(t, err)
	var rules []models.AlertRule
	require.NoError(t, json.Unmarshal(source, &rules))
	require.NotEmpty(t, rules)
	payloads, err := BuiltinPayloadInFile.GetBuiltinPayload("alert", "mysql_by_categraf", "", component.ID, LangSource)
	require.NoError(t, err)
	require.Len(t, payloads, len(rules))
	for _, rule := range rules {
		payload := BuiltinPayloadInFile.GetByUUID(rule.UUID, LangSource)
		require.NotNil(t, payload)
		require.Equal(t, rule.Name, payload.Name)
		require.Equal(t, component.ID, payload.ComponentID)
		require.Equal(t, rule.UUID, payload.ID)
		require.Equal(t, strings.Join(rule.AppendTagsJSON, " "), payload.Tags)
		require.Equal(t, SYSTEM, payload.CreatedBy)
		encoded, err := json.Marshal(rule)
		require.NoError(t, err)
		require.JSONEq(t, string(encoded), payload.Content)
		translated := BuiltinPayloadInFile.GetByUUID(rule.UUID, LangEnUS)
		require.NotNil(t, translated)
		dict := BuiltinPayloadInFile.dicts[component.ID][LangEnUS]
		require.Equal(t, Translate(dict, rule.Name), translated.Name)
	}
	boards, err := BuiltinPayloadInFile.GetBuiltinPayload("dashboard", "", "", component.ID, LangEnUS)
	require.NoError(t, err)
	require.Len(t, boards, 1)
	require.NotEmpty(t, boards[0].Content)
	require.NotEmpty(t, BuiltinPayloadInFile.BuiltinMetrics)
	englishReadme, err := os.ReadFile(filepath.Join(dir, "MySQL", "markdown", "README.en_US.md"))
	require.NoError(t, err)
	require.Equal(t, string(englishReadme), BuiltinPayloadInFile.Readme(LangEnUS, "MySQL"))
}

func TestInit_PreservesUserEditedComponentReadme(t *testing.T) {
	c, dir := round2IntegrationStore(t)
	component := &models.BuiltinComponent{ID: 1, Ident: "MySQL", Readme: "custom instructions", UpdatedBy: "alice"}
	require.NoError(t, c.DB.Create(component).Error)

	Init(c, dir)

	got, err := models.BuiltinComponentGet(c, "ident = ?", "MySQL")
	require.NoError(t, err)
	require.Equal(t, "custom instructions", got.Readme)
	require.Equal(t, "alice", got.UpdatedBy)
	require.NotEmpty(t, BuiltinPayloadInFile.IndexData[LangSource])
}

func TestInit_RefreshesSystemComponentReadme(t *testing.T) {
	c, dir := round2IntegrationStore(t)
	component := &models.BuiltinComponent{ID: 1, Ident: "MySQL", Readme: "outdated", UpdatedBy: SYSTEM}
	require.NoError(t, c.DB.Create(component).Error)
	expected, err := os.ReadFile(filepath.Join(dir, "MySQL", "markdown", "README.md"))
	require.NoError(t, err)

	Init(c, dir)

	got, err := models.BuiltinComponentGet(c, "ident = ?", "MySQL")
	require.NoError(t, err)
	require.Equal(t, string(expected), got.Readme)
	require.Equal(t, component.ID, got.ID)
}

func TestInit_DisableFlagSkipsFileLoading(t *testing.T) {
	c, dir := round2IntegrationStore(t)
	require.NoError(t, c.DB.Create(&models.Configs{Ckey: "disable_integration_init", Cval: "1"}).Error)

	Init(c, dir)

	require.Empty(t, BuiltinPayloadInFile.Data)
	var count int64
	require.NoError(t, c.DB.Model(&models.BuiltinComponent{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestRenderVariant_DashboardTranslatesMetadataAndPreservesSource(t *testing.T) {
	payload := &models.BuiltinPayload{
		Type: "dashboard", Name: "大盘", Note: "说明", Tags: "数据库",
		Content: `{"name":"大盘","configs":{"panels":[{"name":"连接数"}]}}`,
	}
	dict := ComponentDict{"大盘": "Overview", "说明": "Notes", "数据库": "Database", "连接数": "Connections"}

	got := renderVariant(payload, dict)

	require.NotSame(t, payload, got)
	require.Equal(t, "Overview", got.Name)
	require.Equal(t, "Notes", got.Note)
	require.Equal(t, "Database", got.Tags)
	require.JSONEq(t, `{"name":"Overview","configs":{"panels":[{"name":"Connections"}]}}`, got.Content)
	require.Equal(t, "大盘", payload.Name)
	require.Contains(t, payload.Content, "连接数")
}

func TestRenderVariant_InvalidJSONPreservesOriginal(t *testing.T) {
	payload := &models.BuiltinPayload{Type: "alert", Name: "规则", Content: "{"}

	got := renderVariant(payload, ComponentDict{"规则": "Rule"})

	require.Same(t, payload, got)
	require.Equal(t, "{", got.Content)
}
