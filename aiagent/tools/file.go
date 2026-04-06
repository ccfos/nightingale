package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
)

func init() {
	register("list_files", aiagent.AgentTool{
		Name:        "list_files",
		Description: "列出技能或集成模板目录下的文件和子目录",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "base", Type: "string", Description: "基础目录名: 技能名(如 n9e-create-dashboard)或 integrations/分类(如 integrations/Linux)", Required: true},
			{Name: "path", Type: "string", Description: "相对子路径，如 panels 或 dashboards。不传则列出根目录", Required: false},
		},
	}, listFiles)

	register("read_file", aiagent.AgentTool{
		Name:        "read_file",
		Description: "读取技能文档或集成模板文件内容",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "base", Type: "string", Description: "基础目录名: 技能名(如 n9e-create-dashboard)或 integrations/分类(如 integrations/Linux)", Required: true},
			{Name: "path", Type: "string", Description: "相对文件路径，如 panels/timeseries.md 或 dashboards/categraf-detail.json", Required: true},
		},
	}, readFile)

	register("grep_files", aiagent.AgentTool{
		Name:        "grep_files",
		Description: "在技能或集成模板目录下搜索包含指定关键词的文件和行",
		Type:        aiagent.ToolTypeBuiltin,
		Parameters: []aiagent.ToolParameter{
			{Name: "base", Type: "string", Description: "基础目录名: 技能名(如 n9e-create-dashboard)或 integrations/分类(如 integrations/Linux)", Required: true},
			{Name: "pattern", Type: "string", Description: "搜索关键词（不区分大小写）", Required: true},
			{Name: "path", Type: "string", Description: "相对搜索路径，不传则搜索整个目录", Required: false},
		},
	}, grepFiles)
}

// resolveBasePath 解析基础目录路径，支持 skill 目录和 integrations 目录
// base 可以是技能名(如 "n9e-create-dashboard")或 "integrations/分类"(如 "integrations/Linux")
func resolveBasePath(base, subPath string) (string, error) {
	skillsPath := aiagent.GetSkillsPath()
	if skillsPath == "" {
		return "", fmt.Errorf("skills path not configured")
	}

	// skillsPath 的父目录就是项目根目录（skill 和 integrations 同级）
	projectRoot := filepath.Dir(skillsPath)

	var baseDir string
	if strings.HasPrefix(base, "integrations/") || base == "integrations" {
		baseDir = filepath.Join(projectRoot, base)
	} else {
		baseDir = filepath.Join(skillsPath, base)
	}

	baseDir = filepath.Clean(baseDir)

	// 安全检查：必须在项目根目录下
	if !strings.HasPrefix(baseDir, projectRoot) {
		return "", fmt.Errorf("invalid base: %s", base)
	}

	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return "", fmt.Errorf("directory not found: %s", base)
	}

	if subPath == "" {
		return baseDir, nil
	}

	fullPath := filepath.Join(baseDir, filepath.Clean(subPath))

	// 防止路径逃逸
	if !strings.HasPrefix(fullPath, baseDir) {
		return "", fmt.Errorf("invalid path: %s", subPath)
	}

	return fullPath, nil
}

func listFiles(_ context.Context, args map[string]interface{}, _ map[string]string) (string, error) {
	base := getArgString(args, "base")
	if base == "" {
		return "", fmt.Errorf("base is required")
	}

	dirPath, err := resolveBasePath(base, getArgString(args, "path"))
	if err != nil {
		return "", err
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return "", fmt.Errorf("failed to list directory: %v", err)
	}

	var sb strings.Builder
	for _, entry := range entries {
		if entry.IsDir() {
			sb.WriteString(entry.Name())
			sb.WriteString("/\n")
		} else {
			info, _ := entry.Info()
			if info != nil {
				sb.WriteString(fmt.Sprintf("%-40s %d bytes\n", entry.Name(), info.Size()))
			} else {
				sb.WriteString(entry.Name())
				sb.WriteString("\n")
			}
		}
	}

	if sb.Len() == 0 {
		return "(empty directory)", nil
	}
	return sb.String(), nil
}

func readFile(_ context.Context, args map[string]interface{}, _ map[string]string) (string, error) {
	base := getArgString(args, "base")
	if base == "" {
		return "", fmt.Errorf("base is required")
	}

	path := getArgString(args, "path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	filePath, err := resolveBasePath(base, path)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// 限制大文件返回
	const maxSize = 64 * 1024
	if len(data) > maxSize {
		return string(data[:maxSize]) + "\n\n... (truncated, file too large)", nil
	}

	return string(data), nil
}

func grepFiles(_ context.Context, args map[string]interface{}, _ map[string]string) (string, error) {
	base := getArgString(args, "base")
	if base == "" {
		return "", fmt.Errorf("base is required")
	}

	pattern := getArgString(args, "pattern")
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	patternLower := strings.ToLower(pattern)

	searchDir, err := resolveBasePath(base, getArgString(args, "path"))
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
