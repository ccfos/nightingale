package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent"
	"github.com/ccfos/nightingale/v6/aiagent/skill"
	"github.com/ccfos/nightingale/v6/aiagent/tools/defs"
)

func init() {
	register(defs.ListCode, listCode)
	register(defs.SearchCode, searchCode)
	register(defs.ReadCode, readCode)
}

// 语料仓库白名单：与 embedded.CodeTarballs 的 key / 解压目录名一一对应。
// 白名单先于文件系统判定，repo 参数不落入路径拼接前就被过滤。
var codeRepos = map[string]struct{}{
	"n9e":      {},
	"categraf": {},
	"fe":       {},
}

const (
	// codeCorpusUnavailable 是语料缺失时的统一提示：默认构建（无 qa_code_embed）
	// 或解压失败都会走到。措辞刻意引导 LLM 降级回文档检索而不是反复重试。
	codeCorpusUnavailable = "code corpus not available in this build — do not retry code tools, answer from search_n9e_docs instead"

	// searchCodeMaxMatches 逐行命中上限；searchCodeMaxPathHits 路径命中列表上限。
	searchCodeMaxMatches  = 100
	searchCodeMaxPathHits = 20

	// searchCodeMaxOutputBytes 逐行命中正文的累计上限。条数上限管不住体积：
	// 100 条命中在 context_lines=5 时是 1100 行，语料实测 search_code(fe,
	// "icon", context_lines=5) 约 110KB。最终兜底在 agent 侧的观测截断
	// （aiagent.LiveObservationCapBytes），这里提前收口是为了不白构造再丢掉大
	// 半，更是为了能在结尾给出"收窄 path 重试"这种有语义的提示——被上游拦腰
	// 截断的话，模型只会看到半截输出，不知道该换个更窄的查询。
	searchCodeMaxOutputBytes = 32 << 10

	// searchCodeMaxFileBytes 参与内容搜索的单文件上限。语料是过滤后的源码，
	// 正常源文件远小于 1MB；超限的（生成物/超长数据文件）跳过内容搜索，
	// 也顺带保证 read-all-lines 的上下文实现内存安全。
	searchCodeMaxFileBytes = 1 << 20

	// searchCodeMaxLineRunes 单行输出上限。文件级的 1MB 门挡不住"小文件长行"：
	// fe 语料里有内联 SVG 常量单行 12k+ 字符，icon/doris 等文件也有多处单行
	// 超 2000 字符。命中这种行时 maxMatches 只限条数不限字节，context_lines
	// 还会把它放大到 (2n+1) 倍，单次工具返回可达数百 KB，足以撑爆 LLM 上下文。
	// 水位与 search_n9e_docs 的 n9eDocContentsMaxRunes 同理（那边是整篇 6000
	// rune，这里是每行 500 rune × 最多 100 条）。按 rune 截断而非 byte：语料
	// 含中文注释，byte 截断会切碎 UTF-8。
	searchCodeMaxLineRunes = 500
)

// truncateCodeLine 把超长行截到 searchCodeMaxLineRunes 个 rune 并加尾注，
// 让 LLM 知道这行被截过（而不是以为源码本来就长这样）。
func truncateCodeLine(line string) string {
	if len(line) <= searchCodeMaxLineRunes {
		return line // 字节数已不超限时 rune 数必然不超限，省掉转换
	}
	runes := []rune(line)
	if len(runes) <= searchCodeMaxLineRunes {
		return line
	}
	return string(runes[:searchCodeMaxLineRunes]) + " ...(line truncated)"
}

// resolveCodeRoot 返回语料根目录 <projectRoot>/code。锚点推导与 file.go 的
// resolveBasePath 同法：SkillsPath 的父目录即运行目录（skill 与 integrations/
// code 同级）。目录不存在视为"本构建无语料"，返回统一降级提示。
func resolveCodeRoot(deps *aiagent.ToolDeps) (string, error) {
	if deps == nil || deps.SkillsPath == "" {
		return "", fmt.Errorf("skills path not configured")
	}
	codeRoot := filepath.Join(filepath.Dir(filepath.Clean(deps.SkillsPath)), skill.CodeCorpusDirName)
	// 目录存在还不够：必须带完成标记才算可用。无标记 = 用户自建目录或残缺
	// 语料，检索它会把"没搜到"误传为"该标识符不存在"，比不可用更糟。
	if st, err := os.Stat(codeRoot); err != nil || !st.IsDir() || !skill.CodeCorpusComplete(codeRoot) {
		return "", fmt.Errorf("%s", codeCorpusUnavailable)
	}
	return codeRoot, nil
}

// resolveCodePath 解析 repo + 相对子路径到语料内的绝对路径，repo 白名单 +
// within() 防穿越（复用 file.go 的同一判定）。
func resolveCodePath(deps *aiagent.ToolDeps, repo, subPath string) (string, error) {
	if _, ok := codeRepos[repo]; !ok {
		return "", fmt.Errorf("unknown repo %q, must be one of: n9e, categraf, fe", repo)
	}
	codeRoot, err := resolveCodeRoot(deps)
	if err != nil {
		return "", err
	}
	repoDir := filepath.Join(codeRoot, repo)
	if st, err := os.Stat(repoDir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("%s", codeCorpusUnavailable)
	}
	if subPath == "" {
		return repoDir, nil
	}
	fullPath := filepath.Join(repoDir, filepath.Clean(subPath))
	if !within(repoDir, fullPath) {
		return "", fmt.Errorf("invalid path: %s", subPath)
	}
	return fullPath, nil
}

// corpusVersionLine 读 code/manifest.json 拼一行版本说明（如
// "code corpus: n9e@v9.0.0, categraf@v0.4.19, fe@v9.0.1"）。manifest 缺失或
// 解析失败返回空串——版本行是锦上添花，不能因为它让检索本身失败。
func corpusVersionLine(deps *aiagent.ToolDeps) string {
	codeRoot, err := resolveCodeRoot(deps)
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(codeRoot, "manifest.json"))
	if err != nil {
		return ""
	}
	var entries []struct {
		Repo string `json:"repo"`
		Ref  string `json:"ref"`
	}
	if json.Unmarshal(data, &entries) != nil || len(entries) == 0 {
		return ""
	}
	parts := make([]string, 0, len(entries))
	for _, e := range entries {
		parts = append(parts, fmt.Sprintf("%s@%s", e.Repo, e.Ref))
	}
	return "code corpus: " + strings.Join(parts, ", ")
}

func listCode(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	repo := getArgString(args, "repo")
	if repo == "" {
		return "", fmt.Errorf("repo is required")
	}

	dirPath, err := resolveCodePath(deps, repo, getArgString(args, "path"))
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
		// 隐藏 .corpus_hash 等元信息文件，与 list_files 的点文件过滤语义一致
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

func searchCode(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	repo := getArgString(args, "repo")
	if repo == "" {
		return "", fmt.Errorf("repo is required")
	}
	pattern := getArgString(args, "pattern")
	if pattern == "" {
		return "", fmt.Errorf("pattern is required")
	}
	patternLower := strings.ToLower(pattern)

	contextLines := getArgInt(args, "context_lines", 0)
	if contextLines < 0 {
		contextLines = 0
	}
	if contextLines > 5 {
		contextLines = 5
	}

	// repoRoot 与 searchDir 分开解析：path 参数只缩小 walk 范围，输出的命中
	// 路径始终以仓库根为基准——read_code 的 path 参照系就是仓库根，两个工具
	// 的路径必须可以直接互投，LLM 不该自己拼回被 path 吃掉的前缀。
	repoRoot, err := resolveCodePath(deps, repo, "")
	if err != nil {
		return "", err
	}
	searchDir := repoRoot
	if p := getArgString(args, "path"); p != "" {
		if searchDir, err = resolveCodePath(deps, repo, p); err != nil {
			return "", err
		}
	}

	var pathHits []string
	var matches strings.Builder
	matchCount := 0
	truncated := false

	walkErr := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") {
			if info.IsDir() && path != searchDir {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return nil
		}
		relPath = filepath.ToSlash(relPath)

		// pattern 同时匹配文件路径：按名定位（"找 ping 插件"）一次调用解决
		if strings.Contains(strings.ToLower(relPath), patternLower) && len(pathHits) < searchCodeMaxPathHits {
			pathHits = append(pathHits, relPath)
		}

		if truncated {
			return nil // 继续走完路径匹配，只停内容匹配
		}
		if info.Size() > searchCodeMaxFileBytes {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if !strings.Contains(strings.ToLower(line), patternLower) {
				continue
			}
			if contextLines == 0 {
				matches.WriteString(fmt.Sprintf("%s:%d: %s\n", relPath, i+1, truncateCodeLine(line)))
			} else {
				lo := i - contextLines
				if lo < 0 {
					lo = 0
				}
				hi := i + contextLines
				if hi > len(lines)-1 {
					hi = len(lines) - 1
				}
				for j := lo; j <= hi; j++ {
					marker := " "
					if j == i {
						marker = ">"
					}
					matches.WriteString(fmt.Sprintf("%s:%d:%s %s\n", relPath, j+1, marker, truncateCodeLine(lines[j])))
				}
				matches.WriteString("--\n")
			}
			matchCount++
			if matchCount >= searchCodeMaxMatches || matches.Len() >= searchCodeMaxOutputBytes {
				truncated = true
				break
			}
		}
		return nil
	})
	if walkErr != nil {
		return "", fmt.Errorf("search failed: %v", walkErr)
	}

	var sb strings.Builder
	if v := corpusVersionLine(deps); v != "" {
		sb.WriteString(v)
		sb.WriteString("\n\n")
	}
	if len(pathHits) > 0 {
		sort.Strings(pathHits)
		sb.WriteString(fmt.Sprintf("## files whose path contains %q\n", pattern))
		for _, p := range pathHits {
			sb.WriteString(p)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	if matchCount > 0 {
		sb.WriteString("## line matches\n")
		sb.WriteString(matches.String())
		if truncated {
			sb.WriteString(fmt.Sprintf("... (stopped at %d matches / %d bytes — there are more; narrow with the path argument or a more specific pattern)\n",
				matchCount, matches.Len()))
		}
	}
	if len(pathHits) == 0 && matchCount == 0 {
		return fmt.Sprintf("no matches found for %q in repo %s", pattern, repo), nil
	}
	return sb.String(), nil
}

func readCode(_ context.Context, deps *aiagent.ToolDeps, args map[string]interface{}, _ map[string]string) (string, error) {
	repo := getArgString(args, "repo")
	if repo == "" {
		return "", fmt.Errorf("repo is required")
	}
	path := getArgString(args, "path")
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	filePath, err := resolveCodePath(deps, repo, path)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	// 尾部换行产生的空尾元素不算一行，避免 "N 行文件" 显示成 N+1
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	total := len(lines)

	start := getArgInt(args, "start_line", 1)
	end := getArgInt(args, "end_line", total)
	if start < 1 {
		start = 1
	}
	if end > total {
		end = total
	}
	if start > end {
		return "", fmt.Errorf("invalid line range %d-%d (file has %d lines)", start, end, total)
	}

	// 输出带行号前缀，便于后续 read_code 按 search_code 命中的行号精确取段
	var sb strings.Builder
	for i := start; i <= end; i++ {
		sb.WriteString(fmt.Sprintf("%d\t%s\n", i, lines[i-1]))
		if sb.Len() > aiagent.FileReadMaxBytes {
			sb.WriteString(fmt.Sprintf("\n... (truncated at line %d of %d, use start_line/end_line to read a smaller range)", i, total))
			break
		}
	}
	return sb.String(), nil
}
