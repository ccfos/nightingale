//go:build linux

package sandbox

import (
	"os"
	"testing"
)

// TestDumpSeccompBPF writes the compiled enforce-mode filter to $SECCOMP_DUMP
// for out-of-process verification (run the bytes through bwrap manually). Skips
// unless the env var is set, so it's inert in normal test runs.
func TestDumpSeccompBPF(t *testing.T) {
	path := os.Getenv("SECCOMP_DUMP")
	if path == "" {
		t.Skip("set SECCOMP_DUMP to dump the filter")
	}
	prog, ok := buildSeccompFilter(true)
	if !ok {
		t.Fatal("no seccomp profile for this arch")
	}
	if err := os.WriteFile(path, serializeSeccomp(prog), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d instructions (%d bytes) to %s", len(prog), len(prog)*8, path)
}
