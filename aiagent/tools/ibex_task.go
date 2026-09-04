package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	imodels "github.com/flashcatcloud/ibex/src/models"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
	"github.com/ccfos/nightingale/v6/alert/sender"
	n9emodels "github.com/ccfos/nightingale/v6/models"
	"github.com/toolkits/pkg/logger"
)

// 权限常量，与 center/router/router.go 中 /job-tasks 路由的 perm() 一致。
const (
	PermJobTasks    = "/job-tasks"
	PermJobTasksAdd = "/job-tasks/add"
)

func init() {
	register(defs.DispatchTaskStateless, dispatchTaskStateless)
	register(defs.GetTaskStatus, getTaskStatus)
	register(defs.ListTaskRecords, listTaskRecords)
}

// dispatchEnabledErr 在 ibex 未启用时给出清晰提示（对齐 run_skill_script 的写法），
// 避免未开启 ibex 的部署拿到一串 DB/Redis 报错。
func dispatchEnabledErr() error {
	return fmt.Errorf("ibex 任务下发未启用，请联系系统管理员开启 ibex；若已开启请确认配置无误")
}

// checkTargetsExist 校验目标机在平台中存在（对齐 router 的 CheckTargetsExistByIndent）。
func checkTargetsExist(deps *aiagent.ToolDeps, idents []string) error {
	notExists, err := n9emodels.TargetNoExistIdents(deps.DBCtx, idents)
	if err != nil {
		return err
	}
	if len(notExists) > 0 {
		return fmt.Errorf("targets not exist: %s", strings.Join(notExists, ", "))
	}
	return nil
}

// checkTargetPerm 校验用户对目标机有操作权限（对齐 router 的 CheckTargetPerm）。
func checkTargetPerm(deps *aiagent.ToolDeps, user *n9emodels.User, idents []string) error {
	nopri, err := user.NopriIdents(deps.DBCtx, idents)
	if err != nil {
		return err
	}
	if len(nopri) > 0 {
		return fmt.Errorf("forbidden: no permission on targets: %s", strings.Join(nopri, ", "))
	}
	return nil
}

// parseAuthLevelsStr 把逗号分隔的授权等级字符串解析为 []int，供 task_record 过滤。
func parseAuthLevelsStr(s string) []int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	levels := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if v, err := strconv.Atoi(p); err == nil {
			levels = append(levels, v)
		}
	}
	return levels
}

// =============================================================================
// dispatch_task_stateless：把 AI 现场生成的脚本下发到指定目标机执行。
//
// 高危写操作，强制两阶段人在环确认（update_proposal 范式）：
//   - 首次调用（无 proposal_id 且未 confirmed）：做完整只读校验（权限/业务组/目标机
//     存在性/脚本格式）后只展示脚本全文并中断等用户确认，不落库、不下发；
//   - 用户确认后，运行时以保留的 ResumeArgs 确定性重放本工具（confirmed=true +
//     proposal_id），真正 sender.TaskAdd 下发 + 写 task_record——确认环节零 LLM 参与，
//     杜绝模型幻觉确认或越权自执行。
//
// =============================================================================
func dispatchTaskStateless(ctx context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	if deps == nil || !deps.IbexEnabled {
		return "", dispatchEnabledErr()
	}

	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermJobTasksAdd); err != nil {
		return "", err
	}

	bgid := getArgInt64(args, "busi_group_id")
	host := strings.TrimSpace(getArgString(args, "host"))
	script := getArgString(args, "script")
	if bgid == 0 {
		return "", fmt.Errorf("busi_group_id is required")
	}
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	if strings.TrimSpace(script) == "" {
		return "", fmt.Errorf("script is required")
	}

	bg, err := n9emodels.BusiGroupGetById(deps.DBCtx, bgid)
	if err != nil || bg == nil {
		return "", fmt.Errorf("busi group not found: %d", bgid)
	}
	if err := checkBgRW(deps, user, bg); err != nil {
		return "", err
	}

	account := getArgString(args, "account")
	if account == "" {
		account = "root"
	}
	timeout := getArgInt(args, "timeout", 30)
	if timeout <= 0 {
		timeout = 30
	}
	if timeout > 3600*24*5 {
		timeout = 3600 * 24 * 5
	}
	authLevel := getArgInt(args, "auth_level", 0)
	if authLevel < 0 || authLevel > 3 {
		return "", fmt.Errorf("auth_level invalid, expect 0/1/2/3")
	}
	title := strings.TrimSpace(getArgString(args, "title"))
	if title == "" {
		title = fmt.Sprintf("AI execute: %s", host)
	}

	form := n9emodels.TaskForm{
		Title:     title,
		Account:   account,
		Batch:     getArgInt(args, "batch", 0),
		Tolerance: getArgInt(args, "tolerance", 0),
		Timeout:   timeout,
		Script:    script,
		Args:      getArgString(args, "args"),
		Stdin:     getArgString(args, "stdin"),
		Action:    "start",
		AuthLevel: authLevel,
		Hosts:     []string{host},
	}

	// 目标机存在性 + 操作权限校验（均为只读），在不真正下发的前提下尽早拦截。
	if err := checkTargetsExist(deps, []string{host}); err != nil {
		return "", err
	}
	if err := checkTargetPerm(deps, user, []string{host}); err != nil {
		return "", err
	}
	if err := form.Verify(); err != nil {
		return "", err
	}

	baseline := hashConfigs(fmt.Sprintf("%d\x00%s\x00%s", bgid, host, script))

	confirmed := getArgBool(args, "confirmed")
	if proposalID := getArgString(args, "proposal_id"); !confirmed && proposalID == "" {
		changeDescs := []string{
			fmt.Sprintf("目标机: `%s`", host),
			fmt.Sprintf("执行账号: `%s`", account),
			fmt.Sprintf("超时: %d 秒", timeout),
		}
		if v := getArgString(args, "args"); v != "" {
			changeDescs = append(changeDescs, fmt.Sprintf("命令行参数: `%s`", v))
		}
		changeDescs = append(changeDescs, fmt.Sprintf("脚本正文:\n```\n%s\n```", script))

		resumeArgs := make(map[string]interface{}, len(args))
		for k, v := range args {
			resumeArgs[k] = v
		}

		return proposeUpdateResume(ctx, deps, params, &updateProposal{
			Kind:         "ibex_task",
			TargetID:     bgid,
			BaselineHash: baseline,
			Changes:      changeDescs,
		}, renderUpdateProposalPrompt(params["lang"], fmt.Sprintf(
			aiagent.LangText(params["lang"],
				"在目标机 **%s** 上执行以下脚本（业务组 %d）",
				"run the following script on host **%s** (busi group %d)"), host, bgid), changeDescs), resumeArgs)
	}

	if _, err := confirmUpdateGate(ctx, deps, params, "dispatch_task_stateless", "ibex_task", bgid, getArgString(args, "proposal_id"), confirmed, baseline); err != nil {
		return "", err
	}

	taskID, err := sender.TaskAdd(form, user.Username, deps.DBCtx.IsCenter)
	if err != nil {
		return "", fmt.Errorf("ibex task add failed: %v", err)
	}

	record := n9emodels.TaskRecord{
		Id:        taskID,
		GroupId:   bgid,
		Title:     form.Title,
		Account:   form.Account,
		Batch:     form.Batch,
		Tolerance: form.Tolerance,
		Timeout:   form.Timeout,
		Script:    form.Script,
		Args:      form.Args,
		AuthLevel: form.AuthLevel,
		CreateAt:  time.Now().Unix(),
		CreateBy:  user.Username,
	}
	if err := record.Add(deps.DBCtx); err != nil {
		return "", fmt.Errorf("persist task record failed: %v", err)
	}

	logger.Infof("dispatch_task_stateless: user=%s, task_id=%d, host=%s, title=%s", user.Username, taskID, host, form.Title)

	// confirm 腿：下发成功后自动等待脚本执行完成（最多 wait_seconds，默认30s），
	// 把最终结果（含 stdout/stderr）直接带回来，避免"确认完就结束、还要用户再问
	// get_task_status"的断档。超时未完成则返回当前进度并提示用 get_task_status 继续看。
	waitSeconds := getArgInt(args, "wait_seconds", 30)
	if waitSeconds < 0 {
		waitSeconds = 0
	}
	if waitSeconds > 300 {
		waitSeconds = 300
	}
	return waitTaskResult(taskID, host, waitSeconds), nil
}

// ibexTerminalStatuses 是任务的终态集合（来自 ibex agentd/server 的 SetStatus 与
// 超时处理）。非终态为 waiting/running/killing。
var ibexTerminalStatuses = map[string]bool{
	"success": true, "failed": true, "killed": true, "killfailed": true, "timeout": true,
}

// waitTaskResult 在用户确认下发后轮询 ibex 直到任务终态或超时，返回给用户的可读
// 结果（markdown）。waitSeconds<=0 表示不等待、直接返回已下发的简要信息。
func waitTaskResult(taskID int64, host string, waitSeconds int) string {
	var sts []map[string]string
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	completed := false
	for {
		sts = ibexHostStatus(taskID)
		if len(sts) > 0 && taskAllTerminal(sts) {
			completed = true
		}
		if completed || waitSeconds <= 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Second)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "✅ 已在 `%s` 上执行（任务ID: %d）\n", host, taskID)
	if len(sts) == 0 {
		sb.WriteString("任务已下发，执行状态暂不可读（可稍后用 get_task_status 查看）。")
		return sb.String()
	}
	for _, s := range sts {
		fmt.Fprintf(&sb, "- `%s`: **%s**\n", s["host"], s["status"])
	}
	for _, key := range []string{"stdout", "stderr"} {
		if out := sts[0][key]; out != "" {
			label := "脚本输出"
			if key == "stderr" {
				label = "错误输出"
			}
			fmt.Fprintf(&sb, "\n%s：\n```\n%s\n```\n", label, out)
		}
	}
	if !completed {
		sb.WriteString("\n（任务仍在执行中，可稍后用 get_task_status 查看完整结果）")
	}
	return sb.String()
}

// taskAllTerminal 判断所有目标机的执行状态是否都已到达终态。
func taskAllTerminal(sts []map[string]string) bool {
	if len(sts) == 0 {
		return false
	}
	for _, s := range sts {
		if !ibexTerminalStatuses[s["status"]] {
			return false
		}
	}
	return true
}

// ibexHostStatus 读取指定任务各目标机的执行状态与脚本输出（stdout/stderr）。
// 用 imodels.TaskHostGets（而非 TaskHostStatus）以拿到完整字段。ibex 的存储 DB
// 在本进程内初始化（center 启动 ServerStart 时注入共享 db），读取为只读查询。
// 用 recover 兜底，避免 ibex 存储未初始化（如 headless/CLI 场景）时 nil 指针 panic。
// 超大输出会在 stdout/stderr 各截断到 maxOutputBytes，防止撑爆模型上下文。
func ibexHostStatus(taskID int64) (statuses []map[string]string) {
	const maxOutputBytes = 16 * 1024 // 单段输出上限 16KiB
	defer func() {
		if r := recover(); r != nil {
			statuses = nil
		}
	}()
	hosts, err := imodels.TaskHostGets(taskID)
	if err != nil {
		return nil
	}
	for _, h := range hosts {
		statuses = append(statuses, map[string]string{
			"host":   h.Host,
			"status": h.Status,
			"stdout": truncateUTF8(h.Stdout, maxOutputBytes),
			"stderr": truncateUTF8(h.Stderr, maxOutputBytes),
		})
	}
	return statuses
}

// truncateUTF8 按字节界截断字符串，并保证不会从 UTF-8 字符中间切断。
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && (s[maxBytes-1]&0xC0) == 0x80 {
		maxBytes--
	}
	return s[:maxBytes] + "...[truncated]"
}

// =============================================================================
// get_task_status：查询一次已下发任务记录的元数据及目标机实时执行状态。
// =============================================================================
func getTaskStatus(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermJobTasks); err != nil {
		return "", err
	}

	taskID := getArgInt64(args, "task_id")
	if taskID == 0 {
		return "", fmt.Errorf("task_id is required")
	}

	rec, err := n9emodels.TaskRecordGetById(deps.DBCtx, taskID)
	if err != nil {
		return "", fmt.Errorf("failed to get task record: %v", err)
	}
	if rec == nil {
		return fmt.Sprintf(`{"task_id":%d,"error":"task record not found"}`, taskID), nil
	}

	if !user.IsAdmin() {
		bgids, _, err := getUserBgids(deps, user)
		if err != nil {
			return "", err
		}
		if !int64SliceContains(bgids, rec.GroupId) {
			return "", fmt.Errorf("forbidden: no access to this task")
		}
	}

	result := map[string]interface{}{
		"task_id":    rec.Id,
		"group_id":   rec.GroupId,
		"title":      rec.Title,
		"account":    rec.Account,
		"timeout":    rec.Timeout,
		"auth_level": rec.AuthLevel,
		"create_at":  rec.CreateAt,
		"create_by":  rec.CreateBy,
	}

	// 执行状态存于 ibex 侧；仅 ibex 启用时可读，读不到不阻塞返回记录元数据。
	if deps != nil && deps.IbexEnabled {
		if sts := ibexHostStatus(rec.Id); len(sts) > 0 {
			result["hosts"] = sts
		} else {
			result["ibex_status_note"] = "执行状态暂不可读（任务可能未开始或 ibex 尚未回填）"
		}
	}

	out, _ := json.Marshal(result)
	return string(out), nil
}

// =============================================================================
// list_task_records：按业务组/关键词/时间窗列出用户可见的下发任务记录。
// =============================================================================
func listTaskRecords(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, params map[string]string) (string, error) {
	user, err := getUser(deps, params)
	if err != nil {
		return "", err
	}
	if err := checkPerm(deps, user, PermJobTasks); err != nil {
		return "", err
	}

	limit := getArgInt(args, "limit", 20)
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	days := getArgInt(args, "days", 7)
	if days < 1 {
		days = 7
	}
	query := getArgString(args, "query")
	authLevels := parseAuthLevelsStr(getArgString(args, "auth_level"))

	// 业务组粒度过滤：非管理员只能看自己有权限的组。
	var gids []int64
	if bgid := getArgInt64(args, "busi_group_id"); bgid != 0 {
		if !user.IsAdmin() {
			perms, _, err := getUserBgids(deps, user)
			if err != nil {
				return "", err
			}
			if !int64SliceContains(perms, bgid) {
				return "", fmt.Errorf("forbidden: no access to busi group %d", bgid)
			}
		}
		gids = []int64{bgid}
	} else if !user.IsAdmin() {
		var e error
		gids, _, e = getUserBgids(deps, user)
		if e != nil {
			return "", e
		}
		if len(gids) == 0 {
			return marshalList(0, []n9emodels.TaskRecord{}), nil
		}
	}

	beginTime := time.Now().Unix() - int64(days)*24*3600
	total, err := n9emodels.TaskRecordTotal(deps.DBCtx, gids, beginTime, "", query, authLevels)
	if err != nil {
		return "", fmt.Errorf("failed to count task records: %v", err)
	}
	list, err := n9emodels.TaskRecordGets(deps.DBCtx, gids, beginTime, "", query, authLevels, limit, 0)
	if err != nil {
		return "", fmt.Errorf("failed to list task records: %v", err)
	}
	return marshalList(int(total), list), nil
}
