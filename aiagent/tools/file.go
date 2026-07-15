package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
)

func init() {
	register(defs.ListFiles, listFiles)
	register(defs.ReadFile, readFile)
	register(defs.GrepFiles, grepFiles)
}

// within 报告 p 是否等于 root 或落在 root 目录之内（两者都应已 filepath.Clean）。
// 用清理后的实际路径关系判定归属，避免 base 里的 .. 穿越骗过前缀分类。
func within(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, "../"))
}

// resolveBasePath 解析基础目录路径，支持 skill 目录和 integrations 目录
// base 可以是技能名(如 "create-dashboard")或 "integrations/分类"(如 "integrations/Linux")
func resolveBasePath(deps *aiagent.ToolDeps, base, subPath string) (string, error) {
	if deps == nil {
		return "", fmt.Errorf("skills path not configured")
	}
	skillsPath := deps.SkillsPath
	if skillsPath == "" {
		return "", fmt.Errorf("skills path not configured")
	}

	// skillsPath 的父目录就是项目根目录（skill 和 integrations 同级）
	skillsPath = filepath.Clean(skillsPath)
	projectRoot := filepath.Dir(skillsPath)
	integrationsRoot := filepath.Clean(filepath.Join(projectRoot, "integrations"))

	var baseDir string
	if strings.HasPrefix(base, "integrations/") || base == "integrations" {
		baseDir = filepath.Join(projectRoot, base)
	} else {
		baseDir = filepath.Join(skillsPath, base)
	}
	baseDir = filepath.Clean(baseDir)

	// 归属判定与访问控制基于「清理后的实际落点」，而非清理前的 base 前缀——否则
	// base="integrations/../skills/<priv>" 会被当成 integration 从而绕过可见性门。
	if within(skillsPath, baseDir) {
		// baseDir 落在 skills 目录内：强制可见性门（与 load_skill/get_skill/run_skill_script
		// 同一道，deps.IsSkillHidden 含 fail-closed 的 deny-all）。
		rel, _ := filepath.Rel(skillsPath, baseDir)
		relSlash := filepath.ToSlash(rel)
		if relSlash == "." {
			// 禁止直接访问 skills 根目录，否则可枚举全部技能目录
			return "", fmt.Errorf("invalid base: %s", base)
		}
		skillName := relSlash
		if i := strings.IndexByte(relSlash, '/'); i >= 0 {
			skillName = relSlash[:i]
		}
		if deps.IsSkillHidden(skillName) {
			return "", fmt.Errorf("directory not found: %s", base)
		}
	} else if !within(integrationsRoot, baseDir) {
		// 既不在 skills 也不在 integrations 内（含 .. 穿越逃逸）：拒绝
		return "", fmt.Errorf("invalid base: %s", base)
	}

	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return "", fmt.Errorf("directory not found: %s", base)
	}

	if subPath == "" {
		return baseDir, nil
	}

	fullPath := filepath.Join(baseDir, filepath.Clean(subPath))

	// 防止路径逃逸：用清理后的实际路径关系判定（而非字符串前缀）——否则
	// path="../foo-private/SKILL.md" 从已授权的 skills/foo 逃到私有 skills/foo-private，
	// 因 "/skills/foo-private" 字符串上以 "/skills/foo" 开头而骗过前缀检查。
	if !within(baseDir, fullPath) {
		return "", fmt.Errorf("invalid path: %s", subPath)
	}

	return fullPath, nil
}

func listFiles(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	base := getArgString(args, "base")
	if base == "" {
		return "", fmt.Errorf("base is required")
	}

	dirPath, err := resolveBasePath(deps, base, getArgString(args, "path"))
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return "", fmt.Errorf("failed to list directory: %v", err)
	}

	var sb strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		// 隐藏 .source / .* 等元信息文件，避免泄漏到 LLM 可见的 listing 中
		if strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() {
			sb.WriteString(name)
			sb.WriteString("/\n")
		} else {
			info, _ := entry.Info()
			if info != nil {
				sb.WriteString(fmt.Sprintf("%-40s %d bytes\n", name, info.Size()))
			} else {
				sb.WriteString(name)
				sb.WriteString("\n")
			}
		}
	}

	if sb.Len() == 0 {
		return "(empty directory)", nil
	}
	return sb.String(), nil
}

func readFile(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	base := getArgString(args, "base")
	if base == "" {
		return "", fmt.Errorf("base is required")
	}

	path := getArgString(args, "path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	filePath, err := resolveBasePath(deps, base, path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// 限制大文件返回
	if len(data) > aiagent.FileReadMaxBytes {
		return string(data[:aiagent.FileReadMaxBytes]) + "\n\n... (truncated, file too large)", nil
	}

	return string(data), nil
}

func grepFiles(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	base := getArgString(args, "base")
	if base == "" {
		return "", fmt.Errorf("base is required")
	}

	pattern := getArgString(args, "pattern")
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	patternLower := strings.ToLower(pattern)

	searchDir, err := resolveBasePath(deps, base, getArgString(args, "path"))
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	matchCount := 0
	const maxMatches = 100

	err = filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || matchCount >= maxMatches {
			return nil
		}

		// 只搜索文本文件
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".yaml" && ext != ".yml" && ext != ".json" && ext != ".txt" && ext != "" {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		relPath, _ := filepath.Rel(searchDir, path)
		scanner := bufio.NewScanner(file)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), patternLower) {
				sb.WriteString(fmt.Sprintf("%s:%d: %s\n", relPath, lineNum, line))
				matchCount++
				if matchCount >= maxMatches {
					sb.WriteString(fmt.Sprintf("\n... (stopped at %d matches)", maxMatches))
					return filepath.SkipAll
				}
			}
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("search failed: %v", err)
	}

	if sb.Len() == 0 {
		return fmt.Sprintf("no matches found for '%s'", pattern), nil
	}
	return sb.String(), nil
}
