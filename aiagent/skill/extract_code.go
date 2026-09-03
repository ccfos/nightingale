package skill

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ccfos/nightingale/v6/aiagent/skill/embedded"
	"github.com/toolkits/pkg/logger"
)

// CodeCorpusDirName 是 QA 代码语料在运行目录下的落地目录名（与 skill/、
// integrations/ 同级）。list_code / search_code / read_code 工具以
// SkillsPath 的父目录 + 本常量为安全锚点，导出以避免魔法字符串散落。
const CodeCorpusDirName = "code"

// corpusHashMarker 是语料目录根部的幂等标记文件：内容为本次嵌入资产的
// sha256。它同时承担两个职责：
//  1. 幂等——启动时 hash 一致则整目录跳过重解压；
//  2. 归属——没有它的既存 code/ 目录视为用户自己的目录，拒绝 RemoveAll
//     （与 .fromdb 的"有标记才是我们的"防误删语义一致，方向相反）。
const corpusHashMarker = ".corpus_hash"

// 解压安全上限。语料实测 ~4500 文件 / 18MB，上限放百倍余量；不复用
// archive.go 的 MaxTotalExtracted/models.MaxFilesPerSkill——那是按"单个用户
// skill"标定的配额（50MB / 每 skill 行数上限），与整仓语料不是一个量级。
const (
	corpusMaxFiles      = 100_000
	corpusMaxTotalBytes = 200 << 20
)

// ExtractCodeCorpus 把 qa_code_embed 构建内嵌的三仓库代码语料解压到
// <projectRoot>/code/{n9e,categraf,fe}/，并在根部落 manifest.json（工具层
// 读版本用）与 .corpus_hash（幂等标记）。projectRoot 应传 SkillsPath 的父
// 目录（即运行目录，与 integrations/ 同级）。
//
// 默认构建（无 -tags qa_code_embed）内嵌为空，直接 no-op 返回 nil；调用方
// （center/router）把错误当 warning 处理，语料缺失只是 QA 降级、不是致命。
func ExtractCodeCorpus(projectRoot string) error {
	return extractCodeCorpus(projectRoot, embedded.CodeTarballs(), embedded.CodeManifest())
}

// extractCodeCorpus 是可注入资产的实现，便于单测不依赖 build tag。
func extractCodeCorpus(projectRoot string, tarballs map[string][]byte, manifest []byte) error {
	if len(tarballs) == 0 {
		return nil
	}
	if projectRoot == "" {
		return fmt.Errorf("projectRoot is empty")
	}

	codeDir := filepath.Join(projectRoot, CodeCorpusDirName)
	wantHash := corpusHash(tarballs, manifest)

	if st, err := os.Stat(codeDir); err == nil && st.IsDir() {
		got, err := os.ReadFile(filepath.Join(codeDir, corpusHashMarker))
		switch {
		case err == nil && strings.TrimSpace(string(got)) == wantHash:
			return nil // 同一份资产已解压，跳过
		case err == nil:
			// 我们的目录但资产变了（升级）：整体重建
			if err := os.RemoveAll(codeDir); err != nil {
				return fmt.Errorf("remove stale code corpus %s: %w", codeDir, err)
			}
		default:
			// 无标记：不是我们解压的目录，宁可不解压也绝不 RemoveAll 别人的
			// 数据。解压全程在 tmp 目录完成后原子 rename（见下），进程半途被
			// kill 也不会产出无标记的 code/，所以走到这里只可能是用户自建。
			// 工具层经 CodeCorpusComplete 同样把它判为不可用。
			logger.Warningf("dir %s exists without %s marker, refusing to overwrite; QA code tools will report corpus unavailable", codeDir, corpusHashMarker)
			return nil
		}
	}

	// 先在同级 tmp 目录组装完整语料，成功后一步 rename 到位：code/ 要么不存
	// 在、要么完整。中途 kill -9/OOM 只会留下 tmp 残骸，下次启动无条件清掉重来。
	tmpDir := filepath.Join(projectRoot, ".code.tmp")
	if err := os.RemoveAll(tmpDir); err != nil {
		return fmt.Errorf("clean stale corpus tmp dir: %w", err)
	}
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", tmpDir, err)
	}
	defer os.RemoveAll(tmpDir) // 成功时 rename 已把它移走，此时是 no-op

	for repo, data := range tarballs {
		if len(data) == 0 {
			continue
		}
		if err := extractCorpusTarGz(data, filepath.Join(tmpDir, repo)); err != nil {
			return fmt.Errorf("extract code corpus %s: %w", repo, err)
		}
	}

	if len(manifest) > 0 {
		if err := os.WriteFile(filepath.Join(tmpDir, "manifest.json"), manifest, 0o644); err != nil {
			return fmt.Errorf("write corpus manifest: %w", err)
		}
	}

	// 标记必须在 rename 之前写全：rename 后 code/ 一出现即带完成标记。
	if err := os.WriteFile(filepath.Join(tmpDir, corpusHashMarker), []byte(wantHash+"\n"), 0o644); err != nil {
		return fmt.Errorf("write corpus hash marker: %w", err)
	}

	if err := os.Rename(tmpDir, codeDir); err != nil {
		return fmt.Errorf("rename corpus into place: %w", err)
	}

	logger.Infof("QA code corpus extracted to %s (%d repos)", codeDir, len(tarballs))
	return nil
}

// CodeCorpusComplete 报告 codeDir 是否为一份完整解压的语料（存在完成标记）。
// 工具层用它做可用性判定：无标记目录（用户自建，或极端外力产生的残缺）一律
// 视为不可用，避免把残缺语料当完整语料检索——那会让"代码里查不到"被误读成
// "该标识符不存在"。
func CodeCorpusComplete(codeDir string) bool {
	_, err := os.Stat(filepath.Join(codeDir, corpusHashMarker))
	return err == nil
}

// corpusHash 计算内嵌资产的整体指纹。按 repo 名排序后串接，保证 map 遍历
// 顺序不影响结果。
func corpusHash(tarballs map[string][]byte, manifest []byte) string {
	names := make([]string, 0, len(tarballs))
	for name := range tarballs {
		names = append(names, name)
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(tarballs[name])
	}
	h.Write([]byte{0})
	h.Write(manifest)
	return hex.EncodeToString(h.Sum(nil))
}

// extractCorpusTarGz 把一个语料 tar.gz 解压到 destDir。归档由我们自己的
// 构建脚本产出（全部是普通文件、相对路径），这里仍按不可信输入防护：
// 路径穿越条目拒绝、绝对路径拒绝、symlink/硬链接等非常规类型跳过、
// 文件数与总量封顶。
func extractCorpusTarGz(data []byte, destDir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	tr := tar.NewReader(gz)
	var fileCount int
	var totalBytes int64
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}

		name := strings.TrimPrefix(filepath.ToSlash(hdr.Name), "./")
		if name == "" || isArchiveNoise(name) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			continue // 目录随文件按需创建
		case tar.TypeReg:
		default:
			// symlink/hardlink/device 等：语料里不该出现，跳过而非报错
			continue
		}

		if strings.HasPrefix(name, "/") {
			return fmt.Errorf("absolute path in archive: %s", hdr.Name)
		}
		target := filepath.Join(destDir, filepath.FromSlash(name))
		if rel, err := filepath.Rel(destDir, target); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("path traversal in archive: %s", hdr.Name)
		}

		fileCount++
		if fileCount > corpusMaxFiles {
			return fmt.Errorf("too many files in archive (> %d)", corpusMaxFiles)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		// LimitReader 兜底防伪造 header 的解压炸弹：按剩余总配额限读
		n, err := io.Copy(f, io.LimitReader(tr, corpusMaxTotalBytes-totalBytes+1))
		f.Close()
		if err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
		totalBytes += n
		if totalBytes > corpusMaxTotalBytes {
			return fmt.Errorf("archive too large (> %d bytes)", corpusMaxTotalBytes)
		}
	}
}
