package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/ginx"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// skillAuthScope 携带一次导入/远程安装/替换/更新请求提交的授权范围与团队。
type skillAuthScope struct {
	Private      int
	UserGroupIds []int64
}

// parseSkillAuthFromForm 从 multipart 表单读取 private 与 user_group_ids（JSON 数组），
// 用于 zip 上传/替换这类 multipart 请求。private 返回指针：nil 表示表单未提交该字段
// （区别于显式提交的 0）；提交了但非 0/1、或 user_group_ids 非法 JSON 直接报错，绝不
// 静默降级成公共。
func parseSkillAuthFromForm(c *gin.Context) (*int, []int64, error) {
	var teams []int64
	if raw := strings.TrimSpace(c.PostForm("user_group_ids")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &teams); err != nil {
			return nil, nil, fmt.Errorf("invalid user_group_ids")
		}
	}
	var privatePtr *int
	if raw := strings.TrimSpace(c.PostForm("private")); raw != "" {
		p, err := strconv.Atoi(raw)
		if err != nil {
			return nil, nil, fmt.Errorf("private flag must be 0 or 1")
		}
		privatePtr = &p
	}
	return privatePtr, teams, nil
}

// resolveSkillAuth 把一次导入/远程安装/替换/更新的授权输入解析成可直接落库的 auth，并做
// 与 aiSkillAdd/aiSkillPut 一致的校验（团队必填、private∈{0,1}、非管理员只能授权自己
// 所属团队——checkTeams 为需具备授权资格的团队集：新建=全部提交团队，编辑=相对既有新增
// 的团队）。current 非 nil 表示编辑既有 skill：privatePtr 为 nil（未提交）时沿用
// current.Private，绝不默认成公共；current 为 nil（新建）时必须显式提交 private。
// 校验失败直接 bomb。
func (rt *Router) resolveSkillAuth(c *gin.Context, privatePtr *int, teams []int64, current *models.AISkill, checkTeams []int64) skillAuthScope {
	private := 0
	if current != nil {
		private = current.Private
	}
	if privatePtr != nil {
		if *privatePtr != 0 && *privatePtr != 1 {
			ginx.Bomb(http.StatusBadRequest, "private flag must be 0 or 1")
		}
		private = *privatePtr
	} else if current == nil {
		ginx.Bomb(http.StatusBadRequest, "private flag is required")
	}
	if len(teams) == 0 {
		ginx.Bomb(http.StatusBadRequest, "user group ids is required")
	}
	if me := c.MustGet("user").(*models.User); !me.IsAdmin() {
		gids, err := models.MyGroupIds(rt.Ctx, me.Id)
		ginx.Dangerous(err)
		if !groupsSubset(gids, checkTeams) {
			ginx.Bomb(http.StatusForbidden, "forbidden")
		}
	}
	return skillAuthScope{Private: private, UserGroupIds: teams}
}

// upsertGeneratedSkillMD 把 AISkill 的字段合成一份 SKILL.md，upsert 进 ai_skill_file 表。
// 用于 JSON 创建 / JSON 更新路径 —— 调用方没有原始 SKILL.md 字节，必须由列反向生成
// 以维护"每个 skill 在 ai_skill_file 里必有一条 name=SKILL.md 记录"的不变式。
func upsertGeneratedSkillMD(c *ctx.Context, s *models.AISkill, username string) error {
	content, err := skill.BuildSkillMD(&skill.DBSkill{
		Name:          s.Name,
		Description:   s.Description,
		Instructions:  s.Instructions,
		License:       s.License,
		Compatibility: s.Compatibility,
		Metadata:      s.Metadata,
		AllowedTools:  s.AllowedTools,
	})
	if err != nil {
		return err
	}
	return models.AISkillFileBatchUpsert(c, s.Id, []*models.AISkillFile{{
		Name:      "SKILL.md",
		Content:   content,
		CreatedBy: username,
	}}, false)
}

// skillContentDiffers 判断内容字段是否变更（enabled / updated_by 等运维字段不计入）。
// 仅当内容变更时，JSON 更新路径才应重新合成 SKILL.md；否则保留上传时留下的原始字节
// ——前端切换 enable 开关会把所有字段原样回传，这里必须识别出来避免误覆盖。
func skillContentDiffers(cur, ref *models.AISkill) bool {
	return cur.Name != ref.Name ||
		cur.Description != ref.Description ||
		cur.Instructions != ref.Instructions ||
		cur.License != ref.License ||
		cur.Compatibility != ref.Compatibility ||
		cur.AllowedTools != ref.AllowedTools ||
		!metadataEqual(cur.Metadata, ref.Metadata)
}

// metadataEqual 归一化 nil / 空 map 再比较，避免前端传 `null` 和 `{}` 的差异误判为变更。
func metadataEqual(a, b map[string]string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// builtinSkillEntry 把内置 skill 的 frontmatter 合成一条只读 AISkill。
// id 用负数：列表与详情共用 skill.ListBuiltinFrontmatters() 的稳定排序，
// 列表项取 id = -(index+1)，详情页据此反查（见 aiSkillGet 的 id < 0 分支）。
// 内置 skill 不进 DB，created_by 固定 "system"（与 models.AISkill.Builtin 的判定一致）。
func builtinSkillEntry(fm skill.Frontmatter, id int64) *models.AISkill {
	return &models.AISkill{
		Id:            id,
		Name:          fm.Name,
		Description:   fm.Description,
		License:       fm.License,
		Compatibility: fm.Compatibility,
		Metadata:      fm.Metadata,
		AllowedTools:  fm.AllowedTools,
		Enabled:       true,
		CreatedBy:     "system",
		UpdatedBy:     "system",
		Builtin:       true,
	}
}

// skillSearchMatch 复刻 AISkillGets 里 name/description 的 LIKE 过滤，用于 overlay
// 的内置 skill。统一走大小写不敏感，避免不同 DB 的 LIKE 行为差异导致两段结果不一致。
func skillSearchMatch(search, name, desc string) bool {
	if search == "" {
		return true
	}
	s := strings.ToLower(search)
	return strings.Contains(strings.ToLower(name), s) ||
		strings.Contains(strings.ToLower(desc), s)
}

// ensureAISkillEditable bombs 403 unless the current user may edit obj. Every
// skill mutation handler (edit/delete/replace/import/git) funnels through this
// so team-based (and legacy owner-based) authorization can't be bypassed via a
// side path.
func (rt *Router) ensureAISkillEditable(c *gin.Context, obj *models.AISkill) {
	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)
	if !obj.CanBeEditedBy(me, gids) {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}
}

// ensureAISkillViewable bombs 403 unless the current user may read obj. Every
// skill read handler (detail / file content) funnels through this so private
// content can't be pulled by guessing an incremental id.
func (rt *Router) ensureAISkillViewable(c *gin.Context, obj *models.AISkill) {
	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)
	if !obj.CanBeViewedBy(me, gids) {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}
}

// groupsSubset reports whether every id in sub belongs to super.
func groupsSubset(super, sub []int64) bool {
	if len(sub) == 0 {
		return true
	}
	set := make(map[int64]struct{}, len(super))
	for _, s := range super {
		set[s] = struct{}{}
	}
	for _, x := range sub {
		if _, ok := set[x]; !ok {
			return false
		}
	}
	return true
}

// addedGroups returns the ids present in next but not in prev.
func addedGroups(prev, next []int64) []int64 {
	if len(next) == 0 {
		return nil
	}
	set := make(map[int64]struct{}, len(prev))
	for _, p := range prev {
		set[p] = struct{}{}
	}
	var out []int64
	for _, n := range next {
		if _, ok := set[n]; !ok {
			out = append(out, n)
		}
	}
	return out
}

func (rt *Router) aiSkillGets(c *gin.Context) {
	search := ginx.QueryStr(c, "search", "")
	lst, err := models.AISkillGets(rt.Ctx, search)
	ginx.Dangerous(err)

	rt.decorateAISkills(lst)

	// 页面接口：非管理员按团队过滤私有可见性，并给每条盖上按请求用户的 can_edit
	// 标记（供前端 gate 增删改按钮）。service 内部接口无 user（看全量、不盖标记），
	// 故用 c.Get 而非 MustGet。
	if v, exist := c.Get("user"); exist {
		if me, ok := v.(*models.User); ok {
			gids, err := models.MyGroupIds(rt.Ctx, me.Id)
			ginx.Dangerous(err)
			if !me.IsAdmin() {
				lst = models.FilterAISkillsVisible(lst, gids)
			}
			for _, s := range lst {
				s.CanEdit = s.CanBeEditedBy(me, gids)
			}
		}
	}

	// overlay：把随二进制打包的内置 skill 作为只读条目追加到列表尾部。embedded FS
	// 是内置 skill 的唯一真相源（不进 DB、不参与 dbsync），这里读取时叠加。
	if !rt.Center.AIAgent.HideBuiltinSkills {
		// 同名 DB skill 优先（用户覆盖了内置），丢弃对应 overlay 避免重复，
		// 与 dbsync "DB 胜出" 语义一致。
		existing := make(map[string]struct{}, len(lst))
		for _, s := range lst {
			existing[s.Name] = struct{}{}
		}
		// 注意：id = -(i+1) 按原始 index 分配，去重跳过的项不重新编号，
		// 保证 id↔index 在本次构建内稳定，详情页能据 id 反查到同一条。
		for i, fm := range skill.ListBuiltinFrontmatters() {
			if _, dup := existing[fm.Name]; dup {
				continue
			}
			if !skillSearchMatch(search, fm.Name, fm.Description) {
				continue
			}
			lst = append(lst, builtinSkillEntry(fm, -int64(i+1)))
		}
	}

	ginx.NewRender(c).Data(lst, nil)
}

func (rt *Router) aiSkillGet(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")

	// 负 id = 内置 skill 的只读详情（见 aiSkillGets 的 overlay）。从 embedded FS
	// 取 SKILL.md 正文作为 instructions 展示，不查 DB。
	if id < 0 {
		if rt.Center.AIAgent.HideBuiltinSkills {
			ginx.Bomb(http.StatusNotFound, "ai skill not found")
		}
		builtins := skill.ListBuiltinFrontmatters()
		idx := int(-id) - 1
		if idx < 0 || idx >= len(builtins) {
			ginx.Bomb(http.StatusNotFound, "ai skill not found")
		}
		fm := builtins[idx]
		obj := builtinSkillEntry(fm, id)
		if body, ok := skill.BuiltinInstructions(fm.Name); ok {
			obj.Instructions = body
		}
		// 附件（含子目录文件）以只读条目挂在 obj.Files 上。文件内容仍走现有的
		// /ai-skill-file/:fileId 懒加载——这里给每个内置文件分配稳定负 id
		// (= -(全局序号+1))，aiSkillFileGet 的负 id 分支据此反查 embed 取内容。
		for gi, bf := range skill.ListBuiltinFiles() {
			if bf.Skill != fm.Name {
				continue
			}
			obj.Files = append(obj.Files, &models.AISkillFile{
				Id:   -int64(gi + 1),
				Name: bf.RelPath,
				Size: bf.Size,
			})
		}
		ginx.NewRender(c).Data(obj, nil)
		return
	}

	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}

	// 私有 skill 详情仅授权团队可读：拦住「猜 id 直接拉私有内容」；同时盖上按请求
	// 用户的 can_edit 标记，供详情页 gate 增删改按钮（与后端 403 同一判定，无漂移）。
	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)
	if !obj.CanBeViewedBy(me, gids) {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}
	obj.CanEdit = obj.CanBeEditedBy(me, gids)

	if obj.CreatedBy == "system" {
		// 内置 skill 不需要看 skill 细节，只能看 readme 文档
		files, err := models.AISkillFileGets(rt.Ctx, id)
		ginx.Dangerous(err)
		for _, file := range files {
			if strings.EqualFold(file.Name, "readme.md") {
				filedetail, err := models.AISkillFileGetById(rt.Ctx, file.Id)
				ginx.Dangerous(err)
				if filedetail != nil {
					obj.Instructions = filedetail.Content
				}
				break
			}
		}
	} else {
		// Include associated files (without content)
		files, err := models.AISkillFileGets(rt.Ctx, id)
		ginx.Dangerous(err)
		obj.Files = files
	}

	rt.decorateAISkill(obj)
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiSkillAdd(c *gin.Context) {
	var obj models.AISkill
	ginx.BindJSON(c, &obj)
	ginx.Dangerous(obj.Verify())

	me := c.MustGet("user").(*models.User)
	// 授权团队必填：团队成员据此获得编辑权。非管理员只能授权给自己所在的团队——
	// 要求提交的团队全部属于自己（子集），而非仅有交集，防止把私有 skill 授权给
	// 自己无关的团队。
	if len(obj.UserGroupIds) == 0 {
		ginx.Bomb(http.StatusBadRequest, "user group ids is required")
	}
	if !me.IsAdmin() {
		gids, err := models.MyGroupIds(rt.Ctx, me.Id)
		ginx.Dangerous(err)
		if !groupsSubset(gids, obj.UserGroupIds) {
			ginx.Bomb(http.StatusForbidden, "forbidden")
		}
	}
	obj.SourceType = models.AISkillSourceLocal
	obj.CreatedBy = me.Username
	obj.UpdatedBy = me.Username

	err := models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}
		if err := obj.Create(tCtx); err != nil {
			return err
		}
		return upsertGeneratedSkillMD(tCtx, &obj, me.Username)
	})
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(obj.Id, nil)
}

func (rt *Router) aiSkillPut(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}

	var ref models.AISkill
	ginx.BindJSON(c, &ref)
	ginx.Dangerous(ref.Verify())

	me := c.MustGet("user").(*models.User)
	gids, err := models.MyGroupIds(rt.Ctx, me.Id)
	ginx.Dangerous(err)
	if !obj.CanBeEditedBy(me, gids) {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}
	// 非管理员可保留/移除既有授权团队，但新增的团队必须都属于自己，防止把私有
	// skill 悄悄授权给自己无关的团队（既有团队即便自己不在其中也可原样保留，
	// 以支持多团队 skill 由任一成员编辑/切换启用）。
	if !me.IsAdmin() && !groupsSubset(gids, addedGroups(obj.UserGroupIds, ref.UserGroupIds)) {
		ginx.Bomb(http.StatusForbidden, "forbidden")
	}
	// 强制迁移：编辑任意 skill 都必须带授权团队。私有空团队已由 Verify 挡住，这里补齐
	// 公共场景的服务端校验，防止绕过前端保存无团队 skill；旧 skill 借编辑逐步补齐授权。
	if len(ref.UserGroupIds) == 0 {
		ginx.Bomb(http.StatusBadRequest, "user group ids is required")
	}
	ref.UpdatedBy = me.Username

	// enable toggle 场景：前端 pick 全部字段原样回传，仅 enabled 变。此时保留上传
	// 路径留下的原始 SKILL.md 字节，不要重新合成覆盖（合成结果会丢失注释、非 schema
	// 字段、key 顺序等）。只有内容字段真的变了才重生成。
	contentChanged := skillContentDiffers(obj, &ref)

	err = models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}
		if err := obj.Update(tCtx, ref); err != nil {
			return err
		}
		if !contentChanged {
			return nil
		}
		// 用 ref 的内容字段覆盖 obj 得到合并后的视图，再合成 SKILL.md。
		merged := *obj
		merged.Name = ref.Name
		merged.Description = ref.Description
		merged.Instructions = ref.Instructions
		merged.License = ref.License
		merged.Compatibility = ref.Compatibility
		merged.AllowedTools = ref.AllowedTools
		merged.Metadata = ref.Metadata
		return upsertGeneratedSkillMD(tCtx, &merged, me.Username)
	})
	ginx.NewRender(c).Message(err)
}

func (rt *Router) aiSkillDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}

	rt.ensureAISkillEditable(c, obj)

	// Cascade delete skill files
	ginx.Dangerous(models.AISkillFileDeleteBySkillId(rt.Ctx, id))
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

// extractSkillArchive validates, extracts, and parses a skill archive upload.
// SKILL.md must contain valid YAML frontmatter with a non-empty name field.
// 归档/解压/走读/markdown 解析都委托给 aiagent/skill 子包。
func extractSkillArchive(c *gin.Context) (meta skill.Frontmatter, instructions string, files map[string]string) {
	file, header, err := c.Request.FormFile("file")
	ginx.Dangerous(err)
	defer file.Close()

	lowerName := strings.ToLower(header.Filename)
	isZip := strings.HasSuffix(lowerName, ".zip")
	isTarGz := strings.HasSuffix(lowerName, ".tar.gz") || strings.HasSuffix(lowerName, ".tgz")
	if !isZip && !isTarGz {
		ginx.Bomb(http.StatusBadRequest, "only .zip and .tar.gz/.tgz files are supported")
	}

	const maxArchiveSize = 10 * 1024 * 1024 // 10MB
	if header.Size > maxArchiveSize {
		ginx.Bomb(http.StatusBadRequest, "archive size exceeds 10MB limit")
	}

	// Use LimitReader to enforce size regardless of header.Size (which can be forged)
	data, err := io.ReadAll(io.LimitReader(file, maxArchiveSize+1))
	ginx.Dangerous(err)
	if int64(len(data)) > maxArchiveSize {
		ginx.Bomb(http.StatusBadRequest, "archive size exceeds 10MB limit")
	}

	tmpDir, err := os.MkdirTemp("", "skill-import-*")
	ginx.Dangerous(err)
	defer os.RemoveAll(tmpDir)

	if isZip {
		err = skill.ExtractZip(data, tmpDir)
	} else {
		err = skill.ExtractTarGz(bytes.NewReader(data), tmpDir)
	}
	ginx.Dangerous(err)

	files, err = skill.Walk(tmpDir)
	ginx.Dangerous(err)

	skillMD, ok := files["SKILL.md"]
	if !ok || skillMD == "" {
		ginx.Bomb(http.StatusBadRequest, "SKILL.md not found in archive root")
	}

	var parseOk bool
	meta, instructions, parseOk = skill.ParseMarkdown(skillMD)
	if !parseOk {
		ginx.Bomb(http.StatusBadRequest, "SKILL.md must contain valid YAML frontmatter with a non-empty 'name' field")
	}

	// Validate required fields (same rules as manual create/edit)
	m := models.AISkill{Name: meta.Name, Instructions: instructions}
	ginx.Dangerous(m.Verify())

	// files 里此时已经天然包含 SKILL.md 这条（Walk 不再剥离）。doSkillImport /
	// doSkillImportUpdate 通过 AISkillFileBatchUpsert 遍历 files 落库时，SKILL.md
	// 会作为普通一条记录落进 ai_skill_file 表 —— 原始字节保全。
	return
}

// doSkillImport creates a new skill inside a transaction and returns the new skill ID.
// auth 非空时设定新 skill 的授权范围与团队（导入/远程安装表单提交）；nil 时保持默认
// （公共、无团队——内部 service 路径）。
func (rt *Router) doSkillImport(meta skill.Frontmatter, instructions string, files map[string]string, username string, gitInfo *models.AISkillGitInfo, auth *skillAuthScope) (int64, error) {
	var skillId int64
	err := models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}
		sourceType := models.AISkillSourceLocal
		if gitInfo != nil {
			sourceType = models.AISkillSourceGit
		}

		skill := models.AISkill{
			Name:          meta.Name,
			Description:   meta.Description,
			Instructions:  instructions,
			License:       meta.License,
			Compatibility: meta.Compatibility,
			Metadata:      meta.Metadata,
			AllowedTools:  meta.AllowedTools,
			Enabled:       true,
			SourceType:    sourceType,
			GitInfo:       gitInfo,
			CreatedBy:     username,
			UpdatedBy:     username,
		}
		if auth != nil {
			skill.Private = auth.Private
			skill.UserGroupIds = auth.UserGroupIds
		}
		if err := skill.Create(tCtx); err != nil {
			return err
		}
		skillId = skill.Id

		return upsertSkillFiles(tCtx, skillId, files, username, false)
	})
	return skillId, err
}

// doSkillImportUpdate updates an existing skill inside a transaction.
// auth 非空时用它覆盖授权范围与团队（替换/更新表单显式提交）；nil 时保持既有授权
// 不变（内部 service 路径 / 内置 git 更新）。
func (rt *Router) doSkillImportUpdate(current *models.AISkill, meta skill.Frontmatter, instructions string, files map[string]string, username string, gitInfo *models.AISkillGitInfo, auth *skillAuthScope) error {
	return models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}

		ref := models.AISkill{
			Name:          meta.Name,
			Description:   meta.Description,
			Instructions:  instructions,
			License:       meta.License,
			Compatibility: meta.Compatibility,
			Metadata:      meta.Metadata,
			AllowedTools:  meta.AllowedTools,
			Enabled:       current.Enabled,
			GitInfo:       gitInfo,
			UpdatedBy:     username,
			// 默认不改变鉴权范围：带入既有授权团队与可见性，避免本地 skill 走 Update
			//（其 Select 含 user_group_ids/private）时把私有 skill 静默重置为公共并清空
			// 授权团队（对不 Select 这两列的 git 分支无副作用）。auth 非空时下面覆盖。
			Private:      current.Private,
			UserGroupIds: current.UserGroupIds,
		}
		if auth != nil {
			ref.Private = auth.Private
			ref.UserGroupIds = auth.UserGroupIds
		}
		if gitInfo != nil {
			if err := current.UpdateWithGit(tCtx, ref); err != nil {
				return err
			}
		} else if current.GitInfo != nil {
			gitInfo := *current.GitInfo
			gitInfo.CurrentCommit = ""
			ref.GitInfo = &gitInfo
			if err := current.UpdateWithGit(tCtx, ref); err != nil {
				return err
			}
		} else if err := current.Update(tCtx, ref); err != nil {
			return err
		}

		// git 分支走 UpdateWithGit，其 Select 不含 user_group_ids/private——auth 非空时
		// 在同一事务里单独持久化授权两列（本地 Update 分支已含这两列，无需重复）。
		if auth != nil && (gitInfo != nil || current.GitInfo != nil) {
			if err := current.UpdateAuthScope(tCtx, models.AISkill{
				Private:      auth.Private,
				UserGroupIds: auth.UserGroupIds,
				UpdatedBy:    username,
			}); err != nil {
				return err
			}
		}

		return upsertSkillFiles(tCtx, current.Id, files, username, true)
	})
}

func (rt *Router) aiSkillImport(c *gin.Context) {
	meta, instructions, files := extractSkillArchive(c)
	me := c.MustGet("user").(*models.User)
	privatePtr, teams, err := parseSkillAuthFromForm(c)
	ginx.Dangerous(err)
	auth := rt.resolveSkillAuth(c, privatePtr, teams, nil, teams)
	skillId, err := rt.doSkillImport(meta, instructions, files, me.Username, nil, &auth)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(skillId, nil)
}

func (rt *Router) aiSkillImportUpdate(c *gin.Context) {
	skillId := ginx.UrlParamInt64(c, "id")
	current, err := models.AISkillGetById(rt.Ctx, skillId)
	ginx.Dangerous(err)
	if current == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	rt.ensureAISkillEditable(c, current)
	meta, instructions, files := extractSkillArchive(c)
	me := c.MustGet("user").(*models.User)
	// 内置(system) skill 的替换不套用授权团队（保持既有行为，仅管理员可替换）；
	// 用户 skill 替换可一并管理授权范围与团队。
	var auth *skillAuthScope
	if current.CreatedBy != "system" {
		privatePtr, teams, err := parseSkillAuthFromForm(c)
		ginx.Dangerous(err)
		a := rt.resolveSkillAuth(c, privatePtr, teams, current, addedGroups(current.UserGroupIds, teams))
		auth = &a
	}
	ginx.Dangerous(rt.doSkillImportUpdate(current, meta, instructions, files, me.Username, nil, auth))
	ginx.NewRender(c).Data(skillId, nil)
}

func (rt *Router) aiSkillFileGet(c *gin.Context) {
	fileId := ginx.UrlParamInt64(c, "fileId")

	// 负 id = 内置 skill 的只读附件（见 aiSkillGet 给 obj.Files 分配的负 id）。
	// 按全局序号反查 embed 取内容，不查 DB。
	if fileId < 0 {
		if rt.Center.AIAgent.HideBuiltinSkills {
			ginx.Bomb(http.StatusNotFound, "file not found")
		}
		files := skill.ListBuiltinFiles()
		idx := int(-fileId) - 1
		if idx < 0 || idx >= len(files) {
			ginx.Bomb(http.StatusNotFound, "file not found")
		}
		bf := files[idx]
		content, ok := skill.BuiltinFileContent(bf.Skill, bf.RelPath)
		if !ok {
			ginx.Bomb(http.StatusNotFound, "file not found")
		}
		ginx.NewRender(c).Data(&models.AISkillFile{
			Id:        fileId,
			Name:      bf.RelPath,
			Content:   content,
			Size:      bf.Size,
			CreatedBy: "system",
			UpdatedBy: "system",
		}, nil)
		return
	}

	obj, err := models.AISkillFileGetById(rt.Ctx, fileId)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "file not found")
	}
	// 私有 skill 的文件内容仅授权团队可读：按 fileId 读取前先校验其父 skill。父 skill
	// 不存在（孤儿文件，AISkillGetById 对 NotFound 返回 nil,nil）时 fail-closed 拒绝，
	// 不放行未鉴权读取。
	parent, err := models.AISkillGetById(rt.Ctx, obj.SkillId)
	ginx.Dangerous(err)
	if parent == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	rt.ensureAISkillViewable(c, parent)
	ginx.NewRender(c).Data(obj, nil)
}

func (rt *Router) aiSkillFileDel(c *gin.Context) {
	fileId := ginx.UrlParamInt64(c, "fileId")
	obj, err := models.AISkillFileGetById(rt.Ctx, fileId)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "file not found")
	}
	// 删除文件属修改 skill：按 fileId 删除前先校验其父 skill 的编辑权限。父 skill
	// 不存在（孤儿文件）时 fail-closed 拒绝，不放行未鉴权删除。
	parent, err := models.AISkillGetById(rt.Ctx, obj.SkillId)
	ginx.Dangerous(err)
	if parent == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	rt.ensureAISkillEditable(c, parent)
	ginx.NewRender(c).Message(obj.Delete(rt.Ctx))
}

// aiSkillGetWithFileContents returns skill detail with all file contents included.
// Used by service API where the caller needs the full skill data in one request.
func (rt *Router) aiSkillGetWithFileContents(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	obj, err := models.AISkillGetById(rt.Ctx, id)
	ginx.Dangerous(err)
	if obj == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}

	files, err := models.AISkillFileGetContents(rt.Ctx, id)
	ginx.Dangerous(err)
	obj.Files = files

	rt.decorateAISkill(obj)
	ginx.NewRender(c).Data(obj, nil)
}

// ==================== Service API (v1) ====================

func (rt *Router) aiSkillImportByService(c *gin.Context) {
	meta, instructions, files := extractSkillArchive(c)
	skillId, err := rt.doSkillImport(meta, instructions, files, "system", nil, nil)
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(skillId, nil)
}

func (rt *Router) aiSkillImportUpdateByService(c *gin.Context) {
	skillId := ginx.UrlParamInt64(c, "id")
	current, err := models.AISkillGetById(rt.Ctx, skillId)
	ginx.Dangerous(err)
	if current == nil {
		ginx.Bomb(http.StatusNotFound, "ai skill not found")
	}
	meta, instructions, files := extractSkillArchive(c)
	ginx.Dangerous(rt.doSkillImportUpdate(current, meta, instructions, files, "system", nil, nil))
	ginx.NewRender(c).Data(skillId, nil)
}

func (rt *Router) aiSkillAddByService(c *gin.Context) {
	var obj models.AISkill
	ginx.BindJSON(c, &obj)

	if obj.SourceType == models.AISkillSourceGit {
		rt.aiSkillAddGitByService(c, obj)
		return
	}

	ginx.Dangerous(obj.Verify())

	obj.SourceType = models.AISkillSourceLocal
	obj.CreatedBy = "system"
	obj.UpdatedBy = "system"

	// Upsert: if skill with same name exists, update it; otherwise create.
	exist, err := models.AISkillGetByName(rt.Ctx, obj.Name)
	ginx.Dangerous(err)

	var resultId int64
	err = models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		tCtx := &ctx.Context{DB: tx, CenterApi: rt.Ctx.CenterApi, Ctx: rt.Ctx.Ctx, IsCenter: rt.Ctx.IsCenter}
		if exist != nil {
			if err := exist.UpdateWithGit(tCtx, obj); err != nil {
				return err
			}
			// 合并 exist + obj 得到更新后的视图，再合成 SKILL.md。
			merged := *exist
			merged.Name = obj.Name
			merged.Description = obj.Description
			merged.Instructions = obj.Instructions
			merged.License = obj.License
			merged.Compatibility = obj.Compatibility
			merged.AllowedTools = obj.AllowedTools
			merged.Metadata = obj.Metadata
			resultId = exist.Id
			return upsertGeneratedSkillMD(tCtx, &merged, "system")
		}
		if err := obj.Create(tCtx); err != nil {
			return err
		}
		resultId = obj.Id
		return upsertGeneratedSkillMD(tCtx, &obj, "system")
	})
	ginx.Dangerous(err)
	ginx.NewRender(c).Data(resultId, nil)
}
