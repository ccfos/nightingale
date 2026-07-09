package aiagent

import (
	"bufio"
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
)

// SkillConfig 技能配置（在 AIAgentConfig 中使用）
// 技能目录路径通过全局配置 Plus.AIAgentSkillsPath 设置
//
// 技能供给只有两条路径（渐进披露，无 LLM 预选环节）：
//   - SkillNames 非空：确定性预载（action 的 RequiredSkills / agent 显式绑定），
//     SKILL.md 全文进系统提示词；
//   - SkillNames 为空：不预载。系统提示词常驻「可用技能目录」（名 + 一行描述，
//     见 appendSkillCatalog），模型按需经 load_skill 工具自取——目录稳定常驻
//     利于 prompt cache，加载结果作为工具结果轮 append-only 进上下文。
type SkillConfig struct {
	SkillNames []string `json:"skill_names,omitempty"` // 直接指定预载技能名列表；空 = 目录 + load_skill 自取
	// HiddenSkillNames 从「可用技能目录」里过滤掉的技能名——私有 skill 对非授权
	// 用户不可见。按请求用户动态计算，nil = 不过滤（不改变既有行为）。
	HiddenSkillNames []string `json:"-"`
}

// SkillMetadata 技能元数据（Level 1 - 总是在内存中）
type SkillMetadata struct {
	// 核心字段（与 Anthropic 官方一致）。Description 同时是「可用技能目录」里
	// 模型自选 load_skill 的唯一依据，应包含触发场景（"当用户要求…时使用"）。
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`

	// 可选扩展字段
	RecommendedTools []string `yaml:"recommended_tools,omitempty" json:"recommended_tools,omitempty"`
	BuiltinTools     []string `yaml:"builtin_tools,omitempty" json:"builtin_tools,omitempty"` // 内置工具列表

	// MaxIterations 覆盖 Agent 默认的工具循环迭代上限。多步 skill（如创建仪表盘
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
