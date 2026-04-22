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

		skillPath := filepath.Join(r.skillsPath, entry.Name())
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

// 从 SKILL.md 文件加载元数据
func (r *SkillRegistry) loadMetadataFromFile(filePath string) (*SkillMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// 解析 YAML frontmatter
	scanner := bufio.NewScanner(file)
	var inFrontmatter bool
	var frontmatterLines []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "---" {
			if !inFrontmatter {
				inFrontmatter = true
				continue
			} else {
				// frontmatter 结束
				break
			}
		}

		if inFrontmatter {
			frontmatterLines = append(frontmatterLines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan file: %v", err)
	}

	if len(frontmatterLines) == 0 {
		return nil, fmt.Errorf("no frontmatter found in %s", filePath)
	}

	// 解析 YAML
	frontmatter := strings.Join(frontmatterLines, "\n")
	var metadata SkillMetadata
	if err := yaml.Unmarshal([]byte(frontmatter), &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %v", err)
	}

	if metadata.Name == "" {
		return nil, fmt.Errorf("skill name is required in frontmatter")
	}

	return &metadata, nil
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

// buildSelectionPrompt 构建技能选择提示词
func (s *LLMSkillSelector) buildSelectionPrompt(availableSkills []*SkillMetadata, maxSkills int) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf(`你是一个技能选择器。根据以下任务上下文，选择最合适的技能（可选择 1-%d 个）。

## 可用技能

`, maxSkills))

	for i, skill := range availableSkills {
		sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, skill.Name))
		sb.WriteString(fmt.Sprintf("   %s\n\n", skill.Description))
	}

	sb.WriteString(`## 输出格式

请以 JSON 数组格式返回选中的技能名称，例如：
` + "```json\n" + `["skill-name-1", "skill-name-2"]
` + "```" + `

## 选择原则

1. 选择与任务最相关的技能
2. 如果任务涉及多个领域，可以选择多个技能
3. 优先选择更具体、更专业的技能
4. 如果没有合适的技能，返回空数组 []

请返回技能名称数组：`)

	return sb.String()
}

// parseSelectionResponse 解析 LLM 的选择响应
func (s *LLMSkillSelector) parseSelectionResponse(response string) []string {
	// 尝试从 JSON 代码块中提取
	response = strings.TrimSpace(response)

	// 查找 JSON 数组
	start := strings.Index(response, "[")
	end := strings.LastIndex(response, "]")

	if start < 0 || end <= start {
		return nil
	}

	jsonStr := response[start : end+1]

	var skillNames []string
	if err := json.Unmarshal([]byte(jsonStr), &skillNames); err != nil {
		logger.Warningf("Failed to parse skill selection response: %v", err)
		return nil
	}

	return skillNames
}
