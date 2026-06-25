package models

import (
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

// SandboxExecutionRecord audits one Skill script execution in the sandbox
// (design §15). It mirrors the notification_record style: a flat, append-only
// row written after each run. Stdout/stderr are stored as truncated samples
// (the full output goes to the LLM as a fenced tool_result, not the DB).
type SandboxExecutionRecord struct {
	Id              int64  `json:"id" gorm:"primaryKey;type:bigint;autoIncrement"`
	ExecId          string `json:"exec_id" gorm:"type:varchar(64);not null;index:idx_exec;comment:unique execution id"`
	UserId          int64  `json:"user_id" gorm:"type:bigint;index:idx_user;comment:acting user id"`
	Username        string `json:"username" gorm:"type:varchar(64);comment:acting username"`
	SessionId       string `json:"session_id" gorm:"type:varchar(128);comment:chat/session id"`
	SkillName       string `json:"skill_name" gorm:"type:varchar(255);not null;comment:skill name"`
	Entrypoint      string `json:"entrypoint" gorm:"type:varchar(1024);comment:resolved entry script"`
	Argv            string `json:"argv" gorm:"type:varchar(2048);comment:command argv"`
	Engine          string `json:"engine" gorm:"type:varchar(64);comment:isolation engine"`
	NetworkPolicy   string `json:"network_policy" gorm:"type:varchar(32);comment:none/proxy/direct"`
	TriggerType     string `json:"trigger_type" gorm:"type:varchar(32);comment:llm_tool/api/test"`
	ExitCode        int    `json:"exit_code" gorm:"type:int;comment:process exit code"`
	Timeout         bool   `json:"timeout" gorm:"type:bool;comment:killed by timeout"`
	KilledBy        string `json:"killed_by" gorm:"type:varchar(64);comment:timeout/oom/pids/seccomp:x"`
	DurationMs      int64  `json:"duration_ms" gorm:"type:bigint;comment:wall-clock duration ms"`
	StdoutSample    string `json:"stdout_sample" gorm:"type:text;comment:truncated stdout sample"`
	StderrSample    string `json:"stderr_sample" gorm:"type:text;comment:truncated stderr sample"`
	StdoutTruncated bool   `json:"stdout_truncated" gorm:"type:bool"`
	StderrTruncated bool   `json:"stderr_truncated" gorm:"type:bool"`
	ErrorMsg        string `json:"error_msg" gorm:"type:varchar(2048);comment:setup/run error"`
	CreatedAt       int64  `json:"created_at" gorm:"type:bigint;not null;index:idx_time;comment:create time"`
}

func (r *SandboxExecutionRecord) TableName() string {
	return "sandbox_execution_record"
}

func (r *SandboxExecutionRecord) Add(ctx *ctx.Context) error {
	return Insert(ctx, r)
}

func SandboxExecutionRecordsGet(ctx *ctx.Context, where string, args ...interface{}) ([]*SandboxExecutionRecord, error) {
	var lst []*SandboxExecutionRecord
	err := DB(ctx).Where(where, args...).Order("id desc").Find(&lst).Error
	if err != nil {
		return nil, err
	}
	return lst, nil
}
