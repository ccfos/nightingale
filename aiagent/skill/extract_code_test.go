package skill

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// makeCorpusTarGz 按 name→content 构造一个语料风格的 tar.gz（平铺相对路径）。
func makeCorpusTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(content)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractCodeCorpusBasic(t *testing.T) {
	root := t.TempDir()
	tarballs := map[string][]byte{
		"n9e":      makeCorpusTarGz(t, map[string]string{"models/alert_rule.go": "package models\n", "TREE.md": "# n9e\n"}),
		"categraf": makeCorpusTarGz(t, map[string]string{"inputs/ping/ping.go": "package ping\n"}),
	}
	manifest := []byte(`[{"repo":"n9e","ref":"v9.0.0"}]`)

	if err := extractCodeCorpus(root, tarballs, manifest); err != nil {
		t.Fatalf("extract failed: %v", err)
	}

	for _, p := range []string{
		filepath.Join(root, "code", "n9e", "models", "alert_rule.go"),
		filepath.Join(root, "code", "n9e", "TREE.md"),
		filepath.Join(root, "code", "categraf", "inputs", "ping", "ping.go"),
		filepath.Join(root, "code", "manifest.json"),
		filepath.Join(root, "code", corpusHashMarker),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}
}

func TestExtractCodeCorpusIdempotent(t *testing.T) {
	root := t.TempDir()
	tarballs := map[string][]byte{"n9e": makeCorpusTarGz(t, map[string]string{"a.go": "v1\n"})}
	manifest := []byte(`[]`)

	if err := extractCodeCorpus(root, tarballs, manifest); err != nil {
		t.Fatal(err)
	}
	// 篡改一个已解压文件当哨兵：同 hash 重跑应整体跳过，哨兵原样保留
	sentinel := filepath.Join(root, "code", "n9e", "a.go")
	if err := os.WriteFile(sentinel, []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extractCodeCorpus(root, tarballs, manifest); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(sentinel); string(data) != "modified\n" {
		t.Errorf("same-hash re-run should skip extraction, sentinel overwritten: %q", data)
	}

	// 换资产（hash 变化）重跑应整体重建，哨兵被新内容替换
	tarballs["n9e"] = makeCorpusTarGz(t, map[string]string{"a.go": "v2\n"})
	if err := extractCodeCorpus(root, tarballs, manifest); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(sentinel); string(data) != "v2\n" {
		t.Errorf("hash-change re-run should re-extract, got %q", data)
	}
}

func TestExtractCodeCorpusRefusesForeignDir(t *testing.T) {
	root := t.TempDir()
	// 用户自建的 code/ 目录（无 .corpus_hash 标记）
	userFile := filepath.Join(root, "code", "mine.txt")
	if err := os.MkdirAll(filepath.Dir(userFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userFile, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}

	tarballs := map[string][]byte{"n9e": makeCorpusTarGz(t, map[string]string{"a.go": "x\n"})}
	if err := extractCodeCorpus(root, tarballs, nil); err != nil {
		t.Fatalf("foreign dir should be skipped without error, got: %v", err)
	}
	if data, _ := os.ReadFile(userFile); string(data) != "precious" {
		t.Errorf("user file must survive: %q", data)
	}
	if _, err := os.Stat(filepath.Join(root, "code", "n9e")); !os.IsNotExist(err) {
		t.Error("must not extract into a foreign dir")
	}
}

func TestExtractCodeCorpusRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	tarballs := map[string][]byte{"n9e": makeCorpusTarGz(t, map[string]string{"../evil.txt": "pwned"})}

	if err := extractCodeCorpus(root, tarballs, nil); err == nil {
		t.Fatal("traversal entry must fail extraction")
	}
	if _, err := os.Stat(filepath.Join(root, "evil.txt")); !os.IsNotExist(err) {
		t.Error("traversal file must not be written outside code dir")
	}
	// tmp+rename 流程下失败不产出 code/，tmp 残骸也被清掉
	if _, err := os.Stat(filepath.Join(root, "code")); !os.IsNotExist(err) {
		t.Error("failed extraction must not leave a code dir")
	}
	if _, err := os.Stat(filepath.Join(root, ".code.tmp")); !os.IsNotExist(err) {
		t.Error("failed extraction should clean up tmp dir")
	}
}

// 模拟解压中途被 kill -9：上一轮留下 .code.tmp 残骸、code/ 不存在。
// 下次启动应无条件清掉残骸并成功重建完整语料。
func TestExtractCodeCorpusRecoversFromCrashResidue(t *testing.T) {
	root := t.TempDir()
	junk := filepath.Join(root, ".code.tmp", "n9e", "half.go")
	if err := os.MkdirAll(filepath.Dir(junk), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(junk, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	tarballs := map[string][]byte{"n9e": makeCorpusTarGz(t, map[string]string{"a.go": "x\n"})}
	if err := extractCodeCorpus(root, tarballs, nil); err != nil {
		t.Fatalf("extract over crash residue failed: %v", err)
	}
	if !CodeCorpusComplete(filepath.Join(root, "code")) {
		t.Error("rebuilt corpus must carry the completion marker")
	}
	if _, err := os.Stat(filepath.Join(root, "code", "n9e", "half.go")); !os.IsNotExist(err) {
		t.Error("crash residue must not leak into the rebuilt corpus")
	}
	if _, err := os.Stat(filepath.Join(root, ".code.tmp")); !os.IsNotExist(err) {
		t.Error("tmp dir should be gone after successful rename")
	}
}

func TestExtractCodeCorpusNoopWithoutAssets(t *testing.T) {
	root := t.TempDir()
	if err := extractCodeCorpus(root, nil, nil); err != nil {
		t.Fatalf("no assets should be a no-op, got: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "code")); !os.IsNotExist(err) {
		t.Error("no-op must not create code dir")
	}
}
