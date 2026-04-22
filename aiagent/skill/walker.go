package skill

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Walk 走读已解压的 skill 目录，分离出 SKILL.md 和其它文件。
// 返回：
//   - skillMD:     根 SKILL.md 的原始内容（没找到则为空字符串）
//   - files:       relPath → content 的其它文件映射
//   - err:         走读或读文件出错时返回
//
// 实现会自动 unwrap 单层顶级目录（archive root，见 archiveRoot），
// 并跳过 .* 和 __MACOSX 等系统噪声条目。
func Walk(dir string) (skillMD string, files map[string]string, err error) {
	dir = archiveRoot(dir)
	files = make(map[string]string)

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		if relPath == "." {
			return nil
		}

		// 跳过隐藏文件 / macOS 归档元数据
		if strings.HasPrefix(filepath.Base(relPath), ".") || strings.HasPrefix(relPath, "__MACOSX") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not allowed: %s", relPath)
		}

		if d.IsDir() {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if relPath == "SKILL.md" {
			if len(content) > MaxSkillMD {
				return fmt.Errorf("SKILL.md exceeds %dKB limit (%d bytes)", MaxSkillMD/1024, len(content))
			}
			skillMD = string(content)
		} else {
			if int64(len(content)) > MaxSingleFile {
				return fmt.Errorf("file %s exceeds %dMB limit (%d bytes)", relPath, MaxSingleFile/1024/1024, len(content))
			}
			files[relPath] = string(content)
		}
		return nil
	})
	return
}

// archiveRoot 在归档中有单层顶级目录时返回那层目录，否则返回 dir。
// 这允许 zip 作者用 "my-skill/SKILL.md" 这种带 wrapper 的风格打包，
// 也允许裸 SKILL.md 直接放根目录。
func archiveRoot(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dir
	}

	// 根目录下若有任何非隐藏文件，说明这就是 skill 根，不需要 unwrap
	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			return dir
		}
	}

	var candidate string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "__MACOSX" {
			continue
		}
		if candidate != "" {
			// 多于一个真实顶层目录 —— 没有单层 wrapper
			return dir
		}
		candidate = name
	}

	if candidate != "" {
		return filepath.Join(dir, candidate)
	}
	return dir
}
