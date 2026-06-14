package cconf

import (
	"time"

	"github.com/ccfos/nightingale/v6/pkg/httpx"
)

type Center struct {
	Plugins                   []Plugin
	MetricsYamlFile           string
	OpsYamlFile               string
	BuiltinIntegrationsDir    string
	I18NHeaderKey             string
	MetricDesc                MetricDescType
	AnonymousAccess           AnonymousAccess
	UseFileAssets             bool
	FlashDuty                 FlashDuty
	EventHistoryGroupView     bool
	CleanNotifyRecordDay      int
	CleanPipelineExecutionDay int
	MigrateBusiGroupLabel     bool
	RSA                       httpx.RSAConfig
	AIAgent                   AIAgent
}

type AIAgent struct {
	SkillsPath string `toml:"SkillsPath"`

	// SkillSyncInterval controls how often the DB→FS skill materializer
	// re-scans ai_skill / ai_skill_file and refreshes on-disk copies. The
	// periodic loop is the only trigger — CRUD writes don't fire per-write
	// syncs, which keeps the logic simple and (more importantly) lets every
	// Center replica in a multi-node deployment self-heal against the same DB
	// regardless of which replica served the write.
	//
	// Defaults to 60s. Set to 0 (or a negative value) to disable the periodic
	// loop and only run once at startup — appropriate for environments where
	// skill content is fully baked at deploy time.
	SkillSyncInterval time.Duration `toml:"SkillSyncInterval"`

	// MaxFilesPerSkill caps how many files a single skill may hold (the row
	// count of ai_skill_file for one skill, SKILL.md included). It is the single
	// source of truth shared by two enforcement points: the DB writers in the
	// models package and the archive extractor in aiagent/skill (zip/tar.gz
	// upload). Defaults to 1000; a value <= 0 falls back to the default.
	MaxFilesPerSkill int `toml:"MaxFilesPerSkill"`
}

type Plugin struct {
	Id       int64  `json:"id"`
	Category string `json:"category"`
	Type     string `json:"plugin_type"`
	TypeName string `json:"plugin_type_name"`
}

type FlashDuty struct {
	Api     string
	Headers map[string]string
	Timeout time.Duration
}

type AnonymousAccess struct {
	PromQuerier bool
	AlertDetail bool
}

func (c *Center) PreCheck() {
	if len(c.Plugins) == 0 {
		c.Plugins = Plugins
	}
	if c.AIAgent.SkillsPath == "" {
		// 默认使用项目根路径下的 skill 目录（与 integrations 同级）
		c.AIAgent.SkillsPath = "skill"
	}
	// Only apply the default when unset (zero value). A negative value is an
	// explicit "disable periodic sync" signal and must round-trip unchanged.
	if c.AIAgent.SkillSyncInterval == 0 {
		c.AIAgent.SkillSyncInterval = 60 * time.Second
	}
	if c.AIAgent.MaxFilesPerSkill <= 0 {
		c.AIAgent.MaxFilesPerSkill = 1000
	}
}
