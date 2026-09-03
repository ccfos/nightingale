package router

import (
	"time"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"

	"github.com/toolkits/pkg/logger"
)

// ensureAISkillsSynced blocks until the first full DB→FS sync has completed
// (or runs the first pass inline if the background loop hasn't started yet).
// Called from the chat handler so the first request after boot can't race the
// startup goroutine and see an empty skill registry.
//
// sync.Once makes this a cheap no-op after the first successful pass, so all
// subsequent chat requests pay only an uncontended mutex load.
func (rt *Router) ensureAISkillsSynced() {
	rt.aiSkillSyncOnce.Do(rt.doSyncAllAISkillsFromDB)
}

// runAISkillSyncLoop is the single long-lived goroutine that owns DB→FS skill
// materialization. It does one pass through sync.Once (so chat handlers can
// gate on it), then — if a positive interval is configured — loops on a ticker
// running the same full sync.
//
// Why periodic instead of event-driven CRUD hooks:
//   - Multi-instance safety: in a multi-replica Center deployment, only the
//     replica that served the POST/PUT/DELETE sees a CRUD hook. A ticker lets
//     every replica converge against the shared DB.
//   - Self-healing: a transient disk error during one tick is corrected by the
//     next; no state needs to be reconciled manually.
//   - Simplicity: one code path (full sync), no per-rename / per-delete
//     bookkeeping, no race between CRUD writes and a concurrent full-sweep's
//     orphan cleanup.
//
// The trade-off is a 1-interval lag between "skill saved in UI" and "skill
// visible to the agent". Skills are operator-authored content (minutes-to-days
// cadence), so a 60 s default is a reasonable floor.
func (rt *Router) runAISkillSyncLoop(interval time.Duration) {
	rt.ensureAISkillsSynced()

	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		rt.doSyncAllAISkillsFromDB()
	}
}

// doSyncAllAISkillsFromDB reads every enabled skill + its files in at most two
// queries (skills, then files WHERE skill_id IN ...) and materializes them on
// disk. Errors are logged-only: a partial failure shouldn't kill the loop, and
// the next tick gets another shot.
//
// aiSkillSyncMu guards the whole operation so a second tick (or the startup
// once-pass) can't interleave with the current write phase — important because
// the sync includes a ReadDir-based orphan cleanup step that would be racy
// against another write phase publishing new directories.
func (rt *Router) doSyncAllAISkillsFromDB() {
	skillsPath := rt.Center.AIAgent.SkillsPath
	if skillsPath == "" {
		return
	}

	rt.aiSkillSyncMu.Lock()
	defer rt.aiSkillSyncMu.Unlock()

	start := time.Now()

	rows, err := models.AISkillsEnabled(rt.Ctx)
	if err != nil {
		logger.Warningf("[AISkillSync] load enabled skills failed: %v", err)
		return
	}

	// Filter out DB rows whose name collides with a builtin before querying
	// files — a masked DB skill is dead weight and we shouldn't load its files.
	kept := make([]*models.AISkill, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, s := range rows {
		if skill.IsBuiltinName(s.Name) {
			logger.Warningf("[AISkillSync] skill %q collides with a builtin, skipping DB copy", s.Name)
			continue
		}
		kept = append(kept, s)
		ids = append(ids, s.Id)
	}

	filesByID, err := models.AISkillFilesBySkillIds(rt.Ctx, ids)
	if err != nil {
		logger.Warningf("[AISkillSync] batch-load skill files failed: %v", err)
		return
	}

	dbSkills := make([]skill.DBSkill, 0, len(kept))
	for _, s := range kept {
		rowFiles := filesByID[s.Id]
		dbFiles := make([]skill.DBSkillFile, 0, len(rowFiles))
		for _, f := range rowFiles {
			dbFiles = append(dbFiles, skill.DBSkillFile{Name: f.Name, Content: f.Content})
		}
		dbSkills = append(dbSkills, skill.DBSkill{
			Name:          s.Name,
			Description:   s.Description,
			Instructions:  s.Instructions,
			License:       s.License,
			Compatibility: s.Compatibility,
			Metadata:      s.Metadata,
			AllowedTools:  s.AllowedTools,
			Files:         dbFiles,
		})
	}

	if err := skill.SyncDBSkills(skillsPath, dbSkills); err != nil {
		logger.Warningf("[AISkillSync] SyncDBSkills partial failure: %v", err)
	} else {
		logger.Debugf("[AISkillSync] synced %d db skills to %s in %s", len(dbSkills), skillsPath, time.Since(start))
	}
}
