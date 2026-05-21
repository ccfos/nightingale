package aiagent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	skillpkg "github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/toolkits/pkg/logger"
	"gopkg.in/yaml.v3"
)

const (
	// SkillFileName 技能主文件名
	SkillFileName = "SKILL.md"
	// SkillToolsDir 技能工具目录名
	SkillToolsDir = "skill_tools"

	// 默认配置
	DefaultMaxSkills = 2
)

// SkillConfig 技能配置（在 AIAgentConfig 中使用）
// 技能目录路径通过全局配置 Plus.AIAgentSkillsPath 设置
type SkillConfig struct {
	// 技能选择配置（优先级：SkillNames > LLM 选择 > DefaultSkills）
	AutoSelect    bool     `json:"auto_select,omitempty"`    // 是否让 LLM 自动选择技能（默认 true）
	SkillNames    []string `json:"skill_names,omitempty"`    // 直接指定技能名列表（手动模式）
	MaxSkills     int      `json:"max_skills,omitempty"`     // LLM 最多选择几个技能（默认 2）
	DefaultSkills []string `json:"default_skills,omitempty"` // 默认技能列表（LLM 无法选择时使用）
}

// SkillMetadata 技能元数据（Level 1 - 总是在内存中）
type SkillMetadata struct {
	// 核心字段（与 Anthropic 官方一致）
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`

	// Examples 是触发本 skill 的典型用户问法，会被 selector prompt 当作 few-shot
	// 用——比单看 description 文字相似度强很多，特别能区分"批量导入 vs 单条创建"
	// 这种相邻意图。每条 SKILL.md frontmatter 写 3-5 句即可。
	Examples []string `yaml:"examples,omitempty" json:"examples,omitempty"`

	// 可选扩展字段
	RecommendedTools []string `yaml:"recommended_tools,omitempty" json:"recommended_tools,omitempty"`
	BuiltinTools     []string `yaml:"builtin_tools,omitempty" json:"builtin_tools,omitempty"` // 内置工具列表

	// MaxIterations 覆盖 Agent 默认的 ReAct 迭代上限。多步 skill（如创建仪表盘
	// 需要 list_busi_groups → list_datasources → list_files → list_metrics →
	// read_file → create_dashboard 至少 7-10 步）应声明更高的值，避免在正常
	// 流程末尾被 DefaultMaxIterations=10 截断。为 0 时使用 Agent 默认值。
	MaxIterations int `yaml:"max_iterations,omitempty" json:"max_iterations,omitempty"`

	// 内部字段
	Path     string    `json:"-"` // 技能目录路径
	LoadedAt time.Time `json:"-"` // 加载时间
}

// SkillContent 技能内容（Level 2 - 匹配时加载）
type SkillContent struct {
	Metadata    *SkillMetadata `json:"metadata"`
	MainContent string         `json:"main_content"` // SKILL.md 正文
}

// SkillTool Skill 专用工具（Level 3 - 按需加载）
type SkillTool struct {
	Name        string                 `yaml:"name" json:"name"`               // 工具名称
	Type        string                 `yaml:"type" json:"type"`               // 处理器类型：annotation_qd, script, callback 等
	Description string                 `yaml:"description" json:"description"` // 工具描述
	Config      map[string]interface{} `yaml:"config" json:"config"`           // 处理器配置

	// 参数定义（可选）
	Parameters []ToolParameter `yaml:"parameters,omitempty" json:"parameters,omitempty"`
}

// SkillResources 技能扩展资源（Level 3 - 按需加载）
type SkillResources struct {
	SkillTools map[string]*SkillTool `json:"skill_tools"` // 工具名 -> 工具定义
	References map[string]string     `json:"references"`  // 引用文件内容
}

// SkillRegistry 技能注册表
type SkillRegistry struct {
	skillsPath   string                           // 技能目录路径
	skills       map[string]*SkillMetadata        // name -> metadata
	contentCache map[string]*SkillContent         // name -> content (LRU cache)
	toolsCache   map[string]map[string]*SkillTool // skillName -> toolName -> tool
	mu           sync.RWMutex
}

// NewSkillRegistry 创建技能注册表
func NewSkillRegistry(skillsPath string) *SkillRegistry {
	registry := &SkillRegistry{
		skillsPath:   skillsPath,
		skills:       make(map[string]*SkillMetadata),
		contentCache: make(map[string]*SkillContent),
		toolsCache:   make(map[string]map[string]*SkillTool),
	}

	// 初始加载所有技能元数据
	if err := registry.loadAllMetadata(); err != nil {
		logger.Warningf("Failed to load skill metadata: %v", err)
	}

	return registry
}

// loadAllMetadata 加载所有技能的元数据（Level 1）
func (r *SkillRegistry) loadAllMetadata() error {
	if r.skillsPath == "" {
		return nil
	}

	// 检查目录是否存在
	if _, err := os.Stat(r.skillsPath); os.IsNotExist(err) {
		logger.Debugf("Skills directory does not exist: %s", r.skillsPath)
		return nil
	}

	// 遍历技能目录
	entries, err := os.ReadDir(r.skillsPath)
	if err != nil {
		return fmt.Errorf("failed to read skills directory: %v", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		skillPath := filepath.Join(r.skillsPath, name)

		// builtin skill 优先走进程级共享缓存（embed 已在启动期 Once 解析）。
		// 只有不带 .fromdb 标记的目录才算 builtin —— 用户 skill 即便重名也会
		// 走下面的磁盘解析路径，沿用 ExtractBuiltin 中 DB skill 胜出的语义。
		if !skillpkg.IsFromDB(skillPath) {
			if fm, ok := skillpkg.LookupBuiltin(name); ok {
				metadata := skillMetadataFromFrontmatter(fm)
				metadata.Path = skillPath
				metadata.LoadedAt = time.Now()
				r.skills[metadata.Name] = metadata
				logger.Debugf("Loaded builtin skill metadata from cache: %s", metadata.Name)
				continue
			}
		}

		skillFile := filepath.Join(skillPath, SkillFileName)

		// 检查 SKILL.md 是否存在
		if _, err := os.Stat(skillFile); os.IsNotExist(err) {
			continue
		}

		// 加载元数据
		metadata, err := r.loadMetadataFromFile(skillFile)
		if err != nil {
			logger.Warningf("Failed to load skill metadata from %s: %v", skillFile, err)
			continue
		}

		metadata.Path = skillPath
		metadata.LoadedAt = time.Now()
		r.skills[metadata.Name] = metadata

		logger.Debugf("Loaded skill metadata: %s from %s", metadata.Name, skillPath)
	}

	logger.Infof("Loaded %d skills from %s", len(r.skills), r.skillsPath)
	return nil
}

// skillMetadataFromFrontmatter 把 skill 子包的只读 Frontmatter 转成本包运行期
// 使用的 SkillMetadata。slice 字段做浅拷贝，避免 SkillRegistry 后续对 metadata
// 的局部修改污染到进程级共享缓存。Path/LoadedAt 由调用方在拿到结果后补齐。
func skillMetadataFromFrontmatter(fm skillpkg.Frontmatter) *SkillMetadata {
	m := &SkillMetadata{
		Name:          fm.Name,
		Description:   fm.Description,
		MaxIterations: fm.MaxIterations,
	}
	if len(fm.RecommendedTools) > 0 {
		m.RecommendedTools = append([]string(nil), fm.RecommendedTools...)
	}
	if len(fm.BuiltinTools) > 0 {
		m.BuiltinTools = append([]string(nil), fm.BuiltinTools...)
	}
	if len(fm.Examples) > 0 {
		m.Examples = append([]string(nil), fm.Examples...)
	}
	return m
}

// loadMetadataFromFile 读取 SKILL.md 顶部 YAML frontmatter，转成 SkillMetadata。
//
// 历史实现自己用 bufio.Scanner 逐行 TrimSpace 拼 frontmatter，结果把多行 block
// scalar 的缩进吃掉了——`description: |\n  ...` 这种形式会被压扁成
// `description: |\n...`，后续 YAML 解析在第一行非空字符处报 "could not find
// expected ':'". 现在直接复用 skill.ParseMarkdown（subpackage 里已经写过的、
// 也是 router 导入路径用的那一份），保证两个入口对同一个 SKILL.md 解析一致。
func (r *SkillRegistry) loadMetadataFromFile(filePath string) (*SkillMetadata, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}
	fm, _, ok := skillpkg.ParseMarkdown(string(content))
	if !ok {
		return nil, fmt.Errorf("invalid frontmatter or missing name in %s", filePath)
	}
	return skillMetadataFromFrontmatter(fm), nil
}

// GetByName 根据名称获取技能元数据
func (r *SkillRegistry) GetByName(name string) *SkillMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.skills[name]
}

// ListAll 列出所有技能元数据
func (r *SkillRegistry) ListAll() []*SkillMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*SkillMetadata, 0, len(r.skills))
	for _, metadata := range r.skills {
		result = append(result, metadata)
	}
	return result
}

// LoadContent 加载技能内容（Level 2）
func (r *SkillRegistry) LoadContent(metadata *SkillMetadata) (*SkillContent, error) {
	if metadata == nil {
		return nil, fmt.Errorf("metadata is nil")
	}

	// 检查缓存
	r.mu.RLock()
	if cached, ok := r.contentCache[metadata.Name]; ok {
		r.mu.RUnlock()
		return cached, nil
	}
	r.mu.RUnlock()

	// 加载内容
	skillFile := filepath.Join(metadata.Path, SkillFileName)
	content, err := r.loadContentFromFile(skillFile)
	if err != nil {
		return nil, err
	}

	skillContent := &SkillContent{
		Metadata:    metadata,
		MainContent: content,
	}

	// 缓存
	r.mu.Lock()
	r.contentCache[metadata.Name] = skillContent
	r.mu.Unlock()

	return skillContent, nil
}

// loadContentFromFile 从 SKILL.md 文件加载正文内容
func (r *SkillRegistry) loadContentFromFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var inFrontmatter bool
	var frontmatterEnded bool
	var contentLines []string

	for scanner.Scan() {
		line := scanner.Text()

		if strings.TrimSpace(line) == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			} else {
				frontmatterEnded = true
				continue
			}
		}

		if frontmatterEnded {
			contentLines = append(contentLines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to scan file: %v", err)
	}

	return strings.TrimSpace(strings.Join(contentLines, "\n")), nil
}

// LoadSkillTool 加载单个 skill_tool（Level 3 - 完整配置）
func (r *SkillRegistry) LoadSkillTool(skillName, toolName string) (*SkillTool, error) {
	// 检查缓存
	r.mu.RLock()
	if skillTools, ok := r.toolsCache[skillName]; ok {
		if tool, ok := skillTools[toolName]; ok {
			r.mu.RUnlock()
			return tool, nil
		}
	}
	r.mu.RUnlock()

	// 获取技能元数据
	metadata := r.GetByName(skillName)
	if metadata == nil {
		return nil, fmt.Errorf("skill '%s' not found", skillName)
	}

	// 加载工具
	toolFile := filepath.Join(metadata.Path, SkillToolsDir, toolName+".yaml")
	tool, err := r.loadToolFromFile(toolFile)
	if err != nil {
		return nil, err
	}

	// 缓存
	r.mu.Lock()
	if r.toolsCache[skillName] == nil {
		r.toolsCache[skillName] = make(map[string]*SkillTool)
	}
	r.toolsCache[skillName][toolName] = tool
	r.mu.Unlock()

	return tool, nil
}

// LoadSkillToolDescription 加载 skill_tool 的描述信息（轻量级，只读取 name 和 description）
func (r *SkillRegistry) LoadSkillToolDescription(skillName, toolName string) (string, error) {
	// 检查缓存 - 如果已经加载了完整工具，直接返回 description
	r.mu.RLock()
	if skillTools, ok := r.toolsCache[skillName]; ok {
		if tool, ok := skillTools[toolName]; ok {
			r.mu.RUnlock()
			return tool.Description, nil
		}
	}
	r.mu.RUnlock()

	// 获取技能元数据
	metadata := r.GetByName(skillName)
	if metadata == nil {
		return "", fmt.Errorf("skill '%s' not found", skillName)
	}

	// 只加载 description（不缓存完整工具，保持延迟加载特性）
	toolFile := filepath.Join(metadata.Path, SkillToolsDir, toolName+".yaml")
	tool, err := r.loadToolFromFile(toolFile)
	if err != nil {
		return "", err
	}

	return tool.Description, nil
}

// LoadAllSkillToolDescriptions 加载技能目录下所有 skill_tools 的描述
func (r *SkillRegistry) LoadAllSkillToolDescriptions(skillName string) (map[string]string, error) {
	metadata := r.GetByName(skillName)
	if metadata == nil {
		return nil, fmt.Errorf("skill '%s' not found", skillName)
	}

	toolsDir := filepath.Join(metadata.Path, SkillToolsDir)

	// 检查目录是否存在
	if _, err := os.Stat(toolsDir); os.IsNotExist(err) {
		return make(map[string]string), nil
	}

	// 遍历 skill_tools 目录
	entries, err := os.ReadDir(toolsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read skill_tools directory: %v", err)
	}

	descriptions := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// 只处理 .yaml 文件
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		toolFile := filepath.Join(toolsDir, name)
		tool, err := r.loadToolFromFile(toolFile)
		if err != nil {
			logger.Warningf("Failed to load skill tool %s: %v", toolFile, err)
			continue
		}

		descriptions[tool.Name] = tool.Description
	}

	return descriptions, nil
}

// loadToolFromFile 从文件加载工具定义
func (r *SkillRegistry) loadToolFromFile(filePath string) (*SkillTool, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read tool file: %v", err)
	}

	var tool SkillTool
	if err := yaml.Unmarshal(data, &tool); err != nil {
		return nil, fmt.Errorf("failed to parse tool file: %v", err)
	}

	return &tool, nil
}

// LoadReference 加载引用文件（Level 3）
func (r *SkillRegistry) LoadReference(metadata *SkillMetadata, refName string) (string, error) {
	if metadata == nil {
		return "", fmt.Errorf("metadata is nil")
	}

	refFile := filepath.Join(metadata.Path, refName)
	data, err := os.ReadFile(refFile)
	if err != nil {
		return "", fmt.Errorf("failed to read reference file: %v", err)
	}

	return string(data), nil
}

// Reload 重新加载所有技能元数据
func (r *SkillRegistry) Reload() error {
	r.mu.Lock()
	r.skills = make(map[string]*SkillMetadata)
	r.contentCache = make(map[string]*SkillContent)
	r.toolsCache = make(map[string]map[string]*SkillTool)
	r.mu.Unlock()

	return r.loadAllMetadata()
}

// SkillSelector 技能选择器接口
type SkillSelector interface {
	// SelectMultiple 让 LLM 根据任务内容选择最合适的技能（可多选）
	SelectMultiple(ctx context.Context, taskContext string, availableSkills []*SkillMetadata, maxSkills int) ([]*SkillMetadata, error)
}

// LLMSkillSelector 基于 LLM 的技能选择器
type LLMSkillSelector struct {
	llmCaller func(ctx context.Context, messages []ChatMessage) (string, error)
}

// NewLLMSkillSelector 创建 LLM 技能选择器
func NewLLMSkillSelector(llmCaller func(ctx context.Context, messages []ChatMessage) (string, error)) *LLMSkillSelector {
	return &LLMSkillSelector{
		llmCaller: llmCaller,
	}
}

// SelectMultiple 使用 LLM 选择技能
func (s *LLMSkillSelector) SelectMultiple(ctx context.Context, taskContext string, availableSkills []*SkillMetadata, maxSkills int) ([]*SkillMetadata, error) {
	if len(availableSkills) == 0 {
		return nil, nil
	}

	if maxSkills <= 0 {
		maxSkills = DefaultMaxSkills
	}

	// 短路：用户**明确引用**了 skill 完整 name 时跳过 LLM 直接命中。
	//
	// 早先用 strings.Contains 太宽——用户粘 release notes、错误日志、文档片段里
	// 顺手出现的 skill 名字也会触发，比如用户问 "怎么用 n9e-import-prom-rule
	// 这个 skill" 是真要用，但贴一段日志含 "[ERROR] n9e-import-prom-rule timeout"
	// 又问别的事就被误触发。
	//
	// 现在只在 name 周围有"引用标记"时短路：单/双/反引号成对包裹、或紧贴在 / @
	// 之后。这是常见的"我在引用 skill 名"语义，普通文本里很少出现。命中失败就
	// 老老实实走 LLM selector，让 examples + description 决定。
	var explicit []*SkillMetadata
	seen := make(map[string]struct{}, len(availableSkills))
	for _, sk := range availableSkills {
		if sk.Name == "" {
			continue
		}
		if _, dup := seen[sk.Name]; dup {
			continue
		}
		if isExplicitSkillReference(taskContext, sk.Name) {
			explicit = append(explicit, sk)
			seen[sk.Name] = struct{}{}
		}
	}
	if len(explicit) > 0 {
		if len(explicit) > maxSkills {
			explicit = explicit[:maxSkills]
		}
		logger.Debugf("[SkillSelector] explicit name match: %d skill(s) referenced in user message, skipping LLM", len(explicit))
		return explicit, nil
	}

	// 构建提示词
	systemPrompt := s.buildSelectionPrompt(availableSkills, maxSkills)

	messages := []ChatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: taskContext},
	}

	// 调用 LLM
	response, err := s.llmCaller(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %v", err)
	}

	// 解析响应
	selectedNames := s.parseSelectionResponse(response)
	logger.Debugf("[SkillSelector] availableSkills=%d, response=%q, parsed=%v", len(availableSkills), response, selectedNames)
	if len(selectedNames) == 0 {
		return nil, nil
	}

	// 限制数量
	if len(selectedNames) > maxSkills {
		selectedNames = selectedNames[:maxSkills]
	}

	// 转换为 SkillMetadata
	skillMap := make(map[string]*SkillMetadata)
	for _, skill := range availableSkills {
		skillMap[skill.Name] = skill
	}

	var result []*SkillMetadata
	for _, name := range selectedNames {
		if skill, ok := skillMap[name]; ok {
			result = append(result, skill)
		}
	}

	return result, nil
}

// isExplicitSkillReference 判断 text 里是否**明确引用**了 name。
//
// 命中规则（任一即可）：
//   - 紧贴 `name`、'name'、"name" 三种引号包裹
//   - 紧贴在 / 或 @ 之后（如 /n9e-import-prom-rule、@n9e-import-prom-rule）
//
// 只看是否引号 / 前缀符号紧贴，不要求一定到边界——即 "use `n9e-import-prom-rule`!"
// 也命中（!= 反引号闭合后任意字符）。这是常见的"我在引用 skill 名"语义。
// 普通文本里不会无意出现，避免被粘贴的文档 / 日志误触发。
func isExplicitSkillReference(text, name string) bool {
	if name == "" {
		return false
	}
	// 引号包裹三种
	for _, q := range []string{"`", "'", "\""} {
		if strings.Contains(text, q+name+q) {
			return true
		}
	}
	// / 或 @ 前缀
	if strings.Contains(text, "/"+name) || strings.Contains(text, "@"+name) {
		return true
	}
	return false
}

// buildSelectionPrompt 构建技能选择提示词
//
// 设计要点（按 ROI 排序，对应 Anthropic Skills 与业界 RouterChain 实践）：
//
//  1. 每个 skill 渲染 description 之后，把 frontmatter 里的 examples 列出来作为
//     few-shot。"用户这么问 → 选这个 skill" 的对比示例比单看 description 文本
//     相似度强很多——尤其能区分"批量导入 vs 单条创建"这种相邻意图。
//  2. 选择原则强调"读完所有 description 再判断"，并显式警告不要只看名词重叠
//     （selector LLM 常见的失败模式：用户说"导入告警规则"被"告警规则"吸去
//     create-alert-rule，忽略了真正的动词"导入"）。
//  3. 要求 LLM 先输出选择理由再输出 JSON。CoT 形式比直接 JSON 显著提高准确率，
//     parseSelectionResponse 已经能容忍前缀文本（用 LastIndex("]") 定位）。
func (s *LLMSkillSelector) buildSelectionPrompt(availableSkills []*SkillMetadata, maxSkills int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`你是夜莺(n9e)的技能选择器。任务：根据用户的输入选择 1-%d 个最合适的技能。

## 选择原则（关键）

1. **看用户意图的"动词"，不要被"名词"误导**。例如用户说"导入告警规则"——动词是"导入"，名词是"告警规则"。应选导入类 skill，不选创建类 skill。
2. **读完每个 skill 的 description 和 examples 再判断**。description 里如果有 "⚠️" 警告（如"不要用这个 skill 做 X，用 Y"），认真对待。
3. **examples 是同类用户问法**，如果用户提问跟某个 skill 的 example 措辞接近，强信号。
4. 用户提到了 skill 完整名称时，必选该 skill（虽然代码层已经短路了，仍然遵循）。
5. 没有合适的技能就返回 []，不要硬选。

## 可用技能

`, maxSkills))

	for i, sk := range availableSkills {
		sb.WriteString(fmt.Sprintf("### %d. `%s`\n", i+1, sk.Name))
		sb.WriteString(sk.Description)
		if !strings.HasSuffix(sk.Description, "\n") {
			sb.WriteString("\n")
		}
		if len(sk.Examples) > 0 {
			sb.WriteString("典型问法：\n")
			for _, ex := range sk.Examples {
				sb.WriteString(fmt.Sprintf("  - %q → 选 %s\n", ex, sk.Name))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString(`## 输出格式

先用一行中文写出"选择理由"，再单独一行输出 JSON 数组。示例：

选择理由：用户要把 URL 指向的 YAML 文件批量导入，应该用导入类 skill。
` + "```json\n" + `["n9e-import-prom-rule"]
` + "```" + `

现在请基于用户输入做选择：`)

	return sb.String()
}

// parseSelectionResponse 解析 LLM 的选择响应。
//
// 容忍 LLM 在 JSON 之前先输出 CoT 选择理由（新版 selector prompt 鼓励这个）。
// 策略：优先 ```json ... ``` 围栏块；没有就回退到"最后一个 [ ... ] 对"——
// LastIndex 双向锚定避免被 CoT 中可能出现的中括号干扰（理由里如果出现
// "[node-exporter.yml]" 之类的引用，旧实现会把第一个 [ 当成 JSON 起点）。
func (s *LLMSkillSelector) parseSelectionResponse(response string) []string {
	response = strings.TrimSpace(response)

	if jsonStr := extractFencedJSONArray(response); jsonStr != "" {
		if names := tryUnmarshalStringArray(jsonStr); names != nil {
			return names
		}
	}

	// 回退：取最后一对 [ ... ]
	end := strings.LastIndex(response, "]")
	if end < 0 {
		return nil
	}
	start := strings.LastIndex(response[:end], "[")
	if start < 0 {
		return nil
	}
	if names := tryUnmarshalStringArray(response[start : end+1]); names != nil {
		return names
	}
	return nil
}

// extractFencedJSONArray 从 ```json ... ``` 或 ``` ... ``` 围栏里抠出第一段 JSON 数组。
// 返回空串表示没找到围栏块。
func extractFencedJSONArray(s string) string {
	const fence = "```"
	for {
		i := strings.Index(s, fence)
		if i < 0 {
			return ""
		}
		rest := s[i+len(fence):]
		// 可选语言标记，如 "json\n"
		if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
			rest = rest[nl+1:]
		}
		j := strings.Index(rest, fence)
		if j < 0 {
			return ""
		}
		block := strings.TrimSpace(rest[:j])
		if strings.HasPrefix(block, "[") && strings.HasSuffix(block, "]") {
			return block
		}
		s = rest[j+len(fence):]
	}
}

func tryUnmarshalStringArray(jsonStr string) []string {
	var names []string
	if err := json.Unmarshal([]byte(jsonStr), &names); err != nil {
		logger.Warningf("Failed to parse skill selection response: %v", err)
		return nil
	}
	return names
}
