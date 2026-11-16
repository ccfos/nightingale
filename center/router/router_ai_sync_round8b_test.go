package router

import (
	"github.com/ccfos/nightingale/v6/models"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestR8bSkillSyncMaterializesFilesAndRemovesOrphans(t *testing.T) {
	rt := r8bRouter(t, &models.AISkill{}, &models.AISkillFile{})
	rt.Center.AIAgent.SkillsPath = t.TempDir()
	row := &models.AISkill{Name: "r8b-triage", Description: "Triage", Instructions: "Inspect metrics", Enabled: true}
	require.NoError(t, rt.Ctx.DB.Create(row).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.AISkillFile{SkillId: row.Id, Name: "references/guide.md", Content: "Check saturation"}).Error)
	require.NoError(t, rt.Ctx.DB.Create(&models.AISkillFile{SkillId: row.Id, Name: "SKILL.md", Content: "---\nname: r8b-triage\ndescription: Triage\n---\nInspect metrics"}).Error)
	rt.runAISkillSyncLoop(0)
	data, err := os.ReadFile(filepath.Join(rt.Center.AIAgent.SkillsPath, row.Name, "SKILL.md"))
	require.NoError(t, err)
	require.Contains(t, string(data), "Inspect metrics")
	data, err = os.ReadFile(filepath.Join(rt.Center.AIAgent.SkillsPath, row.Name, "references/guide.md"))
	require.NoError(t, err)
	require.Equal(t, "Check saturation", string(data))
	require.NoError(t, rt.Ctx.DB.Model(row).Update("enabled", false).Error)
	rt.doSyncAllAISkillsFromDB()
	_, err = os.Stat(filepath.Join(rt.Center.AIAgent.SkillsPath, row.Name))
	require.True(t, os.IsNotExist(err))
}
