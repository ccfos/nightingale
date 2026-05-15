package skill

import (
	"io/fs"
	"sort"
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
