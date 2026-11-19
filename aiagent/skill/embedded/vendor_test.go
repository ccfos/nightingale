package embedded_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Exercise the real asset generators and a downstream module without downloading
// corpora, running Docker, or modifying the checkout's generated assets.
// Vendoring inspects all build tags, including the Linux sandbox tags on macOS.
func TestVendorWithoutGeneratedAssets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the QA corpus builder requires bash and Unix tools")
	}
	for _, tool := range []string{"bash", "git", "tar", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is required: %v", tool, err)
		}
	}

	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	dependency := filepath.Join(root, "nightingale")
	embeddedDir := filepath.Join(dependency, "aiagent", "skill", "embedded")
	sandboxDir := filepath.Join(dependency, "pkg", "sandbox")
	sourceRoot := filepath.Clean(filepath.Join(packageDir, "..", "..", ".."))
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	copySource := func(src, dst string) {
		t.Helper()
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		write(dst, string(data))
	}
	for _, name := range []string{"embedded.go", "code_embed.go", "code_embed_off.go"} {
		copySource(filepath.Join(packageDir, name), filepath.Join(embeddedDir, name))
	}
	for _, name := range []string{"embed_off.go", "embed_linux_amd64.go", "embed_linux_arm64.go"} {
		copySource(filepath.Join(sourceRoot, "pkg", "sandbox", name), filepath.Join(sandboxDir, name))
	}
	write(filepath.Join(sandboxDir, "fixture.go"), `package sandbox

func FixtureAvailable() bool {
	return len(embeddedBwrap()) > 0 && len(embeddedBaseTarGz()) > 0 && len(embeddedInit()) > 0
}
`)
	write(filepath.Join(embeddedDir, "builtin", "example", "SKILL.md"), "builtin fixture\n")
	write(filepath.Join(dependency, "go.mod"), "module github.com/ccfos/nightingale/v6\n\ngo 1.25.0\n")
	write(filepath.Join(dependency, "src", "app.ts"), "export const fixture = true;\n")
	write(filepath.Join(dependency, "conf", "config.toml"), "fixture = true\n")
	for _, script := range []string{"build-qa-code-assets.sh", "generate-sandbox-embed.sh"} {
		copySource(filepath.Join(sourceRoot, "scripts", script), filepath.Join(dependency, "scripts", script))
	}

	run := func(dir string, extraEnv []string, args ...string) ([]byte, error) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local", "GOFLAGS=", "GOPROXY=off",
			"EXPECT_QA=0", "EXPECT_SANDBOX=0")
		cmd.Env = append(cmd.Env, extraEnv...)
		return cmd.CombinedOutput()
	}
	mustRun := func(dir string, extraEnv []string, args ...string) {
		t.Helper()
		if out, err := run(dir, extraEnv, args...); err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	consumer := filepath.Join(root, "consumer")
	write(filepath.Join(consumer, "go.mod"), fmt.Sprintf(`module example.com/consumer

go 1.25.0

require github.com/ccfos/nightingale/v6 v6.7.3

replace github.com/ccfos/nightingale/v6 => %q
`, filepath.ToSlash(dependency)))
	write(filepath.Join(consumer, "main.go"), `package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"

	"github.com/ccfos/nightingale/v6/aiagent/skill/embedded"
	"github.com/ccfos/nightingale/v6/pkg/sandbox"
)

func main() {
	if sandbox.FixtureAvailable() != (os.Getenv("EXPECT_SANDBOX") == "1") {
		panic("unexpected sandbox assets")
	}
	builtin, err := embedded.FS.ReadFile("builtin/example/SKILL.md")
	if err != nil || string(builtin) != "builtin fixture\n" {
		panic("builtin resources missing from vendor")
	}
	tarballs := embedded.CodeTarballs()
	if os.Getenv("EXPECT_QA") == "1" {
		if len(tarballs) != 3 || !json.Valid(embedded.CodeManifest()) {
			panic("QA corpus or manifest missing")
		}
		for _, data := range append([][]byte{embedded.DocsTarball()}, tarballs["n9e"], tarballs["categraf"], tarballs["fe"]) {
			reader, err := gzip.NewReader(bytes.NewReader(data))
			if err != nil { panic(err) }
			reader.Close()
		}
	} else if len(tarballs) != 0 || len(embedded.CodeManifest()) != 0 || len(embedded.DocsTarball()) != 0 {
		panic("default build unexpectedly embeds QA corpus")
	}
}
`)

	// This failed before the fix, despite qa_code_embed not being enabled.
	mustRun(consumer, nil, "go", "mod", "vendor")
	mustRun(consumer, []string{"EXPECT_QA=0"}, "go", "run", "-mod=vendor", ".")
	if out, err := run(consumer, nil, "go", "build", "-mod=vendor", "-tags=qa_code_embed", "."); err == nil {
		t.Fatal("QA build without generated assets unexpectedly succeeded")
	} else if !strings.Contains(string(out), "undefined: codeN9e") {
		t.Fatalf("QA build failed for an unexpected reason:\n%s", out)
	}

	// Build tiny local corpora with the production script. A failed generation
	// must not publish embed declarations that would break default vendoring.
	mustRun(dependency, nil, "git", "init", "-q")
	mustRun(dependency, nil, "git", "add", ".")
	mustRun(dependency, nil, "git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.com",
		"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-qm", "fixture")
	docs := filepath.Join(root, "docs")
	write(filepath.Join(docs, "docs", "example.md"), "# English fixture\n")
	write(filepath.Join(docs, "zh", "docs", "example.md"), "# Chinese fixture\n")
	buildEnv := []string{"CATEGRAF_LOCAL=" + dependency, "FE_LOCAL=" + dependency, "DOCS_LOCAL=" + docs}
	if out, err := run(dependency, buildEnv, "bash", "scripts/build-qa-code-assets.sh"); err == nil {
		t.Fatal("asset builder unexpectedly accepted an incomplete docs corpus")
	} else if !strings.Contains(string(out), "expected the built site") {
		t.Fatalf("asset builder failed for an unexpected reason:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(embeddedDir, "code_assets_generated.go")); !os.IsNotExist(err) {
		t.Fatalf("failed asset generation published embed declarations: %v", err)
	}
	mustRun(consumer, nil, "go", "mod", "vendor")
	mustRun(consumer, []string{"EXPECT_QA=0"}, "go", "run", "-mod=vendor", ".")

	for i := 0; i < 100; i++ {
		write(filepath.Join(docs, "docs", fmt.Sprintf("page-%03d.md", i)), "# Fixture page\n")
	}
	mustRun(dependency, buildEnv, "bash", "scripts/build-qa-code-assets.sh")
	mustRun(consumer, nil, "go", "mod", "vendor")
	mustRun(consumer, []string{"EXPECT_QA=0"}, "go", "run", "-mod=vendor", ".")
	mustRun(consumer, []string{"EXPECT_QA=1"}, "go", "run", "-mod=vendor", "-tags=qa_code_embed", ".")

	for _, arch := range []string{"amd64", "arm64"} {
		t.Logf("checking sandbox embedding for linux/%s", arch)
		archEnv := []string{"GOOS=linux", "GOARCH=" + arch, "CGO_ENABLED=0"}
		if out, err := run(consumer, archEnv, "go", "build", "-mod=vendor", "-tags=sandbox_embed", "."); err == nil {
			t.Fatal("sandbox build without generated assets unexpectedly succeeded")
		} else if !strings.Contains(string(out), "undefined: bwrapBin") {
			t.Fatalf("sandbox build failed for an unexpected reason:\n%s", out)
		}
		for _, asset := range []string{"bwrap", "python-base.tar.gz", "n9e-sandbox-init"} {
			if out, err := run(dependency, nil, "bash", "scripts/generate-sandbox-embed.sh", arch); err == nil {
				t.Fatalf("sandbox generator unexpectedly accepted missing %s", asset)
			} else if !strings.Contains(string(out), "missing sandbox asset:") {
				t.Fatalf("sandbox generator failed for an unexpected reason:\n%s", out)
			}
			write(filepath.Join(sandboxDir, "embedassets", "linux_"+arch, asset), "fixture for "+arch+"/"+asset)
		}
		mustRun(dependency, nil, "bash", "scripts/generate-sandbox-embed.sh", arch)
		mustRun(consumer, nil, "go", "mod", "vendor")
		mustRun(consumer, nil, "go", "run", "-mod=vendor", ".")
		mustRun(consumer, archEnv, "go", "build", "-mod=vendor", "-tags=sandbox_embed,qa_code_embed", ".")
		if runtime.GOOS == "linux" && runtime.GOARCH == arch {
			mustRun(consumer, []string{"EXPECT_QA=1", "EXPECT_SANDBOX=1"},
				"go", "run", "-mod=vendor", "-tags=sandbox_embed,qa_code_embed", ".")
		}
	}
}
