package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ccfos/nightingale/v6/aiagent/skill/embedded"
	"github.com/toolkits/pkg/logger"
)

// FromDBMarker 是一个空文件，位于 DB 同步下来的 skill 目录根部。
// 有它 = "这是用户 skill，启动流程不要动"；没有 = "内置 skill 或残留，启动时会被清理"。
//
// **DB 同步写入约定**：在创建一个 DB skill 目录时，`.fromdb` 必须是写入的第一个文件
// （用 MarkFromDB），再写 SKILL.md / skill_tools 等内容。否则同步中途崩溃会留下一个
// 没有 marker 的目录，下次启动会被当成内置残留清掉。
const FromDBMarker = ".fromdb"

const embedRoot = "builtin"

// MarkFromDB 在 skillDir 下创建 .fromdb 空文件。
// DB 同步在创建新 skill 目录时应第一步调用此函数。
func MarkFromDB(skillDir string) error {
	f, err := os.Create(filepath.Join(skillDir, FromDBMarker))
	if err != nil {
		return err
	}
	return f.Close()
}

// IsFromDB 判断 skillDir 是不是 DB 同步下来的用户 skill（存在 .fromdb 即为是）。
func IsFromDB(skillDir string) bool {
	_, err := os.Stat(filepath.Join(skillDir, FromDBMarker))
	return err == nil
}

// IsBuiltinName 判断 name 是否与某个内置 skill 重名。
// 接口层在"创建用户 skill"时应调用此函数做前置校验，拒绝重名请求。
func IsBuiltinName(name string) bool {
	if name == "" {
		return false
	}
	_, err := fs.Stat(embedded.FS, embedRoot+"/"+name)
	return err == nil
}

// ExtractBuiltin 把 embed 中的内置 skill 解压到 skillsPath。
// 两步走：
//  1. 清理：遍历 skillsPath 下所有目录，没有 .fromdb 的一律 RemoveAll
//     （包括上次解压的内置 skill、已从 embed 中移除的旧 skill、半残目录）。
//  2. 解压：把 embed 里的每个 skill 目录原样拷贝到 skillsPath 下。
//
// DB 同步下来的用户 skill（带 .fromdb）全程不被触碰。
//
// **错误处理策略**：best-effort。前置步骤（MkdirAll skillsPath / ReadDir /
// fs.Sub 等）失败视为致命，立即返回。两个循环内的单个 skill 出错只 log 并继续，
// 最后用 errors.Join 汇总返回。即便某个 skill 失败，其它 skill 依然能完成解压，
// SkillRegistry 不会因为一颗老鼠屎丢掉全部。调用方（agent.go）本就把 err 当作
// 非致命 warning 处理，magic 由本函数完成。
func ExtractBuiltin(skillsPath string) error {
	if skillsPath == "" {
		return fmt.Errorf("skillsPath is empty")
	}

	if err := os.MkdirAll(skillsPath, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", skillsPath, err)
	}

	var errs []error

	// Step 1: 清理所有非 DB skill 目录
	existing, err := os.ReadDir(skillsPath)
	if err != nil {
		return fmt.Errorf("read skills dir: %w", err)
	}
	for _, e := range existing {
		if !e.IsDir() {
			continue
		}
		dst := filepath.Join(skillsPath, e.Name())
		if IsFromDB(dst) {
			continue
		}
		if err := os.RemoveAll(dst); err != nil {
			logger.Warningf("remove stale skill dir %s failed: %v", dst, err)
			errs = append(errs, fmt.Errorf("remove %s: %w", e.Name(), err))
		}
	}

	// Step 2: 解压内置 skill
	root, err := fs.Sub(embedded.FS, embedRoot)
	if err != nil {
		return fmt.Errorf("sub embed fs: %w", err)
	}
	entries, err := fs.ReadDir(root, ".")
	if err != nil {
		return fmt.Errorf("read embed root: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		dst := filepath.Join(skillsPath, name)

		// 罕见情况：embed 里的 skill 和某个 DB skill 同名。DB skill 已带 .fromdb
		// 在 Step 1 被保留，这里跳过解压、让 DB skill 胜出，避免覆盖用户数据。
		// 业务侧（UI / API 创建 skill）应使用 IsBuiltinName 做前置校验，
		// 不让此情况进入 DB。这里的 warning 是兜底提醒。
		if IsFromDB(dst) {
			logger.Warningf("builtin skill %q masked by a same-named user skill, builtin copy skipped", name)
			continue
		}

		if err := extractDir(root, name, dst); err != nil {
			logger.Warningf("extract builtin skill %q failed: %v", name, err)
			errs = append(errs, fmt.Errorf("extract %s: %w", name, err))
			// 半解压的 dst 留在原地也不怕：无 .fromdb → 下次启动 Step 1 还会清掉重来
		}
	}

	return errors.Join(errs...)
}

// extractDir 把 root 下的 subPath 子树复制到 dst。
func extractDir(root fs.FS, subPath, dst string) error {
	return fs.WalkDir(root, subPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(subPath, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
