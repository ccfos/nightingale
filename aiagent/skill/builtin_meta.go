package skill

import (
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/ccfos/nightingale/v6/aiagent/skill/embedded"
	"github.com/toolkits/pkg/logger"
)

// builtin skill 的 frontmatter 在进程内只解析一次。embedded.FS 是只读的，
// 解析结果在进程生命周期内不变，所以用 sync.Once 缓存安全且高效。
//
// 缓存同时服务两条调用路径：
//  1. A2A AgentCard 发现端点：取 Name+Description+Tags+Examples
//  2. SkillRegistry 初始化（每个 Agent 实例）：取全部字段构造 SkillMetadata
//
// 没有缓存前，这两条路径都各自跑一遍 19 次 YAML 解析；现在共用一份。
var (
	builtinOnce sync.Once
	builtinList []Frontmatter
	builtinByID map[string]Frontmatter
)

// ListBuiltinFrontmatters 返回所有内置 skill 的 frontmatter，按 name 排序。
// 切片是只读语义——调用方不应修改返回值或其中的 slice 字段，缓存与调用方共享底层数组。
func ListBuiltinFrontmatters() []Frontmatter {
	loadBuiltin()
	return builtinList
}

// LookupBuiltin 按 name 查内置 skill frontmatter，未命中返回 (zero, false)。
// 同样是只读语义。
func LookupBuiltin(name string) (Frontmatter, bool) {
	loadBuiltin()
	fm, ok := builtinByID[name]
	return fm, ok
}

// BuiltinInstructions 返回内置 skill 的 SKILL.md 正文（去掉 frontmatter）。
// 列表页详情用：内置 skill 不进 DB，正文直接从 embed 读取。读取失败返回 ("", false)。
// 内置 SKILL.md 的 frontmatter 在 loadBuiltin 阶段已校验合法，这里只取 body。
func BuiltinInstructions(name string) (string, bool) {
	p := BuiltinEmbedRoot + "/" + name + "/" + skillMDName
	data, err := fs.ReadFile(embedded.FS, p)
	if err != nil {
		return "", false
	}
	_, body, _ := ParseMarkdown(string(data))
	return body, true
}

// BuiltinFile 描述内置 skill 的一个文件（含 SKILL.md 与子目录附件，
// 如 reference.md、datasources/mysql.md）。RelPath 相对 skill 目录，可带 "/"。
type BuiltinFile struct {
	Skill   string
	RelPath string
	Size    int64
}

var (
	builtinFilesOnce sync.Once
	builtinFiles     []BuiltinFile
)

// ListBuiltinFiles 返回所有内置 skill 的全部文件，按 (skill, relPath) 稳定排序，
// 进程内缓存一次。切片下标即全局序号——接口层据此给内置文件分配稳定负 id，
// 让只读详情复用现有的 /ai-skill-file/:fileId 取内容路径（见 router_ai_skill.go）。
func ListBuiltinFiles() []BuiltinFile {
	builtinFilesOnce.Do(func() {
		var list []BuiltinFile
		for _, fm := range ListBuiltinFrontmatters() {
			dir := BuiltinEmbedRoot + "/" + fm.Name
			err := fs.WalkDir(embedded.FS, dir, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				rel := strings.TrimPrefix(p, dir+"/")
				var size int64
				if info, ierr := d.Info(); ierr == nil {
					size = info.Size()
				}
				list = append(list, BuiltinFile{Skill: fm.Name, RelPath: rel, Size: size})
				return nil
			})
			if err != nil {
				logger.Warningf("skill: walk builtin %s files failed: %v", fm.Name, err)
			}
		}
		sort.Slice(list, func(i, j int) bool {
			if list[i].Skill != list[j].Skill {
				return list[i].Skill < list[j].Skill
			}
			return list[i].RelPath < list[j].RelPath
		})
		builtinFiles = list
	})
	return builtinFiles
}

// BuiltinFileContent 读取指定内置 skill 下相对路径文件的内容。relPath 先 Clean
// 归一化，吃掉 ".." 防越界；未命中返回 ("", false)。
func BuiltinFileContent(skillName, relPath string) (string, bool) {
	rel := strings.TrimPrefix(path.Clean("/"+relPath), "/")
	if rel == "" || rel == "." {
		return "", false
	}
	data, err := fs.ReadFile(embedded.FS, BuiltinEmbedRoot+"/"+skillName+"/"+rel)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func loadBuiltin() {
	builtinOnce.Do(func() {
		entries, err := fs.ReadDir(embedded.FS, BuiltinEmbedRoot)
		if err != nil {
			logger.Warningf("skill: read builtin embed root failed: %v", err)
			return
		}

		list := make([]Frontmatter, 0, len(entries))
		byID := make(map[string]Frontmatter, len(entries))
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			path := BuiltinEmbedRoot + "/" + e.Name() + "/" + skillMDName
			data, err := fs.ReadFile(embedded.FS, path)
			if err != nil {
				logger.Warningf("skill: read %s failed: %v", path, err)
				continue
			}
			meta, _, ok := ParseMarkdown(string(data))
			if !ok {
				logger.Warningf("skill: %s frontmatter invalid or name empty", path)
				continue
			}
			list = append(list, meta)
			byID[meta.Name] = meta
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Name < list[j].Name })

		builtinList = list
		builtinByID = byID
	})
}

// skillMDName 是 SKILL.md 文件名。aiagent 包里有同名常量 SkillFileName，但本
// 子包不能反向引入 aiagent 包；在此重声明一份，二者必须保持一致。
const skillMDName = "SKILL.md"
