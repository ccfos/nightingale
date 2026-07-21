//go:build qa_code_embed

package skill

import (
	"os"
	"path/filepath"
	"testing"
)

// 真资产冒烟测试：只在 -tags qa_code_embed（且 codeassets 已由
// scripts/build-qa-code-assets.sh 产出）时编译运行，验证 embed 接线与三个
// 真 tarball 能完整解压。CI 的 goreleaser 构建路径不跑测试，本测试面向
// 本地/流水线的 `go test -tags qa_code_embed ./aiagent/skill/`。
func TestExtractCodeCorpusRealAssets(t *testing.T) {
	root := t.TempDir()
	if err := ExtractCodeCorpus(root); err != nil {
		t.Fatalf("extract real assets: %v", err)
	}
	for _, p := range []string{
		filepath.Join(root, "code", "n9e", "TREE.md"),
		filepath.Join(root, "code", "n9e", "models", "alert_rule.go"),
		filepath.Join(root, "code", "categraf", "TREE.md"),
		filepath.Join(root, "code", "categraf", "conf", "config.toml"),
		filepath.Join(root, "code", "fe", "TREE.md"),
		filepath.Join(root, "code", "manifest.json"),
		filepath.Join(root, "code", corpusHashMarker),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s: %v", p, err)
		}
	}
	// 商业代码红线：fe 语料里绝不能有 src/plus
	if _, err := os.Stat(filepath.Join(root, "code", "fe", "src", "plus")); !os.IsNotExist(err) {
		t.Fatal("fe corpus must NOT contain src/plus")
	}
	// 幂等冒烟
	if err := ExtractCodeCorpus(root); err != nil {
		t.Fatalf("second extract should be a no-op: %v", err)
	}
}
