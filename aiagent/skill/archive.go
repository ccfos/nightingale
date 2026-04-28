// Package skill 承载 Skill 包（.zip / .tar.gz）的解压、归档走读与 SKILL.md 解析等纯 IO
// 逻辑。和 aiagent 包里的 SkillRegistry / SkillContent（运行期技能加载）职责不同：
//
//   - aiagent 包 SkillRegistry：已部署 skill 目录的运行期遍历 / 加载 / 元数据查询；
//   - aiagent/skill 子包（本包）：上传流程，解压用户提交的归档、校验结构、读取内容。
//
// 这样 router 只需做 "解析 multipart → 调 skill.* → 落库"，不再内嵌数百行归档代码。
package skill

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// 归档安全限制：防止 zip bomb / 过大单文件
const (
	MaxFileCount      = 100              // 单个归档最多文件数（不含 SKILL.md 本身的上限余量）
	MaxTotalExtracted = 50 * 1024 * 1024 // 解压后总大小上限
	MaxSingleFile     = 16 * 1024 * 1024 // 单文件上限（对齐 MEDIUMTEXT）
	MaxSkillMD        = 64 * 1024        // SKILL.md 本身的上限（对齐 TEXT）
)

// ExtractZip 将 data 中的 zip 归档解压到 destDir。做了两层保护：
//  1. 按 header 声明的大小预扫描（防止文件数过多 / 单文件过大 / 解压总量过大）；
//  2. 实际拷贝时再用 LimitReader 兜底，防止 header 被伪造。
func ExtractZip(data []byte, destDir string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}

	// 预扫描
	var fileCount int
	var declaredTotal uint64
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		fileCount++
		if fileCount > MaxFileCount+1 {
			return fmt.Errorf("too many files in archive, max %d", MaxFileCount)
		}
		if f.UncompressedSize64 > uint64(MaxSingleFile) {
			return fmt.Errorf("file %s exceeds %dMB limit (%d bytes)", f.Name, MaxSingleFile/1024/1024, f.UncompressedSize64)
		}
		declaredTotal += f.UncompressedSize64
		if declaredTotal > uint64(MaxTotalExtracted) {
			return fmt.Errorf("total extracted size exceeds %dMB limit", MaxTotalExtracted/1024/1024)
		}
	}

	// 实际解压
	var actualTotal int64
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not allowed in archive: %s", f.Name)
		}

		relPath := filepath.Clean(f.Name)
		if strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
			return fmt.Errorf("invalid path in archive: %s", f.Name)
		}

		destPath := filepath.Join(destDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return err
		}

		n, err := io.Copy(outFile, io.LimitReader(rc, MaxSingleFile+1))
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
		if n > MaxSingleFile {
			return fmt.Errorf("file %s exceeds %dMB limit (declared size forged)", f.Name, MaxSingleFile/1024/1024)
		}

		actualTotal += n
		if actualTotal > MaxTotalExtracted {
			return fmt.Errorf("total extracted size exceeds %dMB limit", MaxTotalExtracted/1024/1024)
		}
	}
	return nil
}

// ExtractTarGz 将 r 中的 tar.gz 归档解压到 destDir，保护逻辑与 ExtractZip 一致。
func ExtractTarGz(r io.Reader, destDir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to open gzip: %w", err)
	}
	defer gz.Close()

	var fileCount int
	var totalSize int64

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			continue
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("symlink/hardlink not allowed in archive: %s", hdr.Name)
		case tar.TypeReg:
			// 继续下面的处理
		default:
			continue
		}

		fileCount++
		if fileCount > MaxFileCount+1 {
			return fmt.Errorf("too many files in archive, max %d", MaxFileCount)
		}

		if hdr.Size > MaxSingleFile {
			return fmt.Errorf("file %s exceeds %dMB limit (%d bytes)", hdr.Name, MaxSingleFile/1024/1024, hdr.Size)
		}
		if totalSize+hdr.Size > MaxTotalExtracted {
			return fmt.Errorf("total extracted size exceeds %dMB limit", MaxTotalExtracted/1024/1024)
		}

		relPath := filepath.Clean(hdr.Name)
		if strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
			return fmt.Errorf("invalid path in archive: %s", hdr.Name)
		}

		destPath := filepath.Join(destDir, relPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		outFile, err := os.Create(destPath)
		if err != nil {
			return err
		}

		n, err := io.Copy(outFile, io.LimitReader(tr, MaxSingleFile+1))
		outFile.Close()
		if err != nil {
			return err
		}
		if n > MaxSingleFile {
			return fmt.Errorf("file %s exceeds %dMB limit (declared size forged)", hdr.Name, MaxSingleFile/1024/1024)
		}

		totalSize += n
		if totalSize > MaxTotalExtracted {
			return fmt.Errorf("total extracted size exceeds %dMB limit", MaxTotalExtracted/1024/1024)
		}
	}
	return nil
}
