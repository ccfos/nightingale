package loggrep

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeLog creates a log file whose lines are timestamped from base onward,
// with the keyword on every markEvery-th line, and stamps its mtime.
func writeLog(t *testing.T, dir, name string, base time.Time, lines, markEvery int, keyword string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	var sb strings.Builder
	for i := 0; i < lines; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		if markEvery > 0 && i%markEvery == 0 {
			fmt.Fprintf(&sb, "%s INFO %s payload %d\n", ts.Format("2006-01-02 15:04:05.000000"), keyword, i)
		} else {
			fmt.Fprintf(&sb, "%s INFO unrelated %d\n", ts.Format("2006-01-02 15:04:05.000000"), i)
		}
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}

	mtime := base.Add(time.Duration(lines) * time.Second)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}

	return path
}

func TestGrepLogDirFindsMatchesAcrossRotations(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-3 * time.Hour)

	writeLog(t, dir, "INFO.log.000", base, 100, 10, "hash-a")
	writeLog(t, dir, "INFO.log", base.Add(time.Hour), 100, 10, "hash-a")

	res := GrepLogDir(context.Background(), dir, "hash-a", GrepOptions{})

	if res.Truncated {
		t.Fatalf("expected a complete result, got truncated (%s)", res.Reason)
	}
	if len(res.Logs) != 20 {
		t.Fatalf("expected 20 matches across both files, got %d", len(res.Logs))
	}
	// newest first
	if res.Logs[0] < res.Logs[len(res.Logs)-1] {
		t.Fatalf("expected logs sorted newest first, got %q before %q", res.Logs[0], res.Logs[len(res.Logs)-1])
	}
}

// A chatty level must not starve a quiet one: the limit is applied per
// rotation chain while scanning, so ERROR lines survive a flood of INFO lines.
func TestGrepLogDirKeepsOtherLevelsWhenOneFloods(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)

	writeLog(t, dir, "INFO.log", base, 400, 1, "hash-b")
	writeLog(t, dir, "ERROR.log", base.Add(30*time.Minute), 10, 1, "hash-b")

	res := GrepLogDir(context.Background(), dir, "hash-b", GrepOptions{Limit: 50})

	if !res.Truncated || res.Reason != ReasonLimit {
		t.Fatalf("expected a limit-truncated result, got truncated=%v reason=%q", res.Truncated, res.Reason)
	}
	if len(res.Logs) != 50 {
		t.Fatalf("expected exactly the limit of 50 lines, got %d", len(res.Logs))
	}

	var errLines int
	for _, l := range res.Logs {
		if strings.Contains(l, "ERROR.log") || strings.Contains(l, "payload") && strings.Contains(l, "ERROR") {
			errLines++
		}
	}
	// the ERROR file is newer, so its lines must be present in a newest-first result
	found := false
	for _, l := range res.Logs {
		if strings.HasPrefix(l, base.Add(30*time.Minute).Format("2006-01-02 15:04")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected lines from the quiet ERROR stream to survive the INFO flood")
	}
}

// The reported failure mode: a log dir too big to scan must degrade to a
// partial result, not to an error and not to an unbounded scan.
func TestGrepLogDirTimeoutReturnsPartialNotError(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-2 * time.Hour)

	for i := 0; i < 6; i++ {
		writeLog(t, dir, fmt.Sprintf("INFO.log.%03d", i), base.Add(time.Duration(i)*10*time.Minute), 2000, 100, "hash-c")
	}

	start := time.Now()
	res := GrepLogDir(context.Background(), dir, "hash-c", GrepOptions{
		Deadline: time.Now().Add(-time.Second),
	})
	elapsed := time.Since(start)

	if !res.Truncated || res.Reason != ReasonTimeout {
		t.Fatalf("expected a timeout-truncated result, got truncated=%v reason=%q", res.Truncated, res.Reason)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("expected the scan to give up quickly, took %v", elapsed)
	}
}

// A single size-rotated file can be hundreds of MB, so the budget has to be
// enforced inside the scan rather than only between files.
func TestGrepFileStopsMidFileOnDeadline(t *testing.T) {
	dir := t.TempDir()
	path := writeLog(t, dir, "INFO.log", time.Now().Add(-time.Hour), 50000, 10, "hash-g")

	lines, stopped := grepFile(context.Background(), path, "hash-g", time.Now().Add(-time.Second))

	if !stopped {
		t.Fatal("expected the scan to stop on the deadline")
	}
	if len(lines) == 0 {
		t.Fatal("expected the lines matched before the deadline to be returned")
	}
	if len(lines) >= 5000 {
		t.Fatalf("expected to stop early, but got %d of the 5000 matches", len(lines))
	}
}

// Skipping files last written before the cutoff is what keeps an event lookup
// from opening the multi-hundred-MB historical files.
func TestGrepLogDirSinceSkipsOlderFiles(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-4 * time.Hour)

	writeLog(t, dir, "INFO.log.000", base, 100, 10, "hash-d")
	writeLog(t, dir, "INFO.log", base.Add(3*time.Hour), 100, 10, "hash-d")

	res := GrepLogDir(context.Background(), dir, "hash-d", GrepOptions{
		Since: base.Add(2 * time.Hour),
	})

	if len(res.Logs) != 10 {
		t.Fatalf("expected only the 10 matches from the recent file, got %d", len(res.Logs))
	}
	for _, l := range res.Logs {
		if strings.HasPrefix(l, base.Format("2006-01-02 15:04")) {
			t.Fatalf("expected the pre-cutoff file to be skipped, got line %q", l)
		}
	}
}

func TestGrepLogDirCancelledContext(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	writeLog(t, dir, "INFO.log", base, 50000, 100, "hash-e")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := GrepLogDir(ctx, dir, "hash-e", GrepOptions{})
	if !res.Truncated || res.Reason != ReasonTimeout {
		t.Fatalf("expected a cancelled scan to report truncation, got truncated=%v reason=%q", res.Truncated, res.Reason)
	}
}

func TestGrepLatestLogFilesIgnoresRotated(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-2 * time.Hour)

	writeLog(t, dir, "INFO.log.2026072711", base, 100, 10, "hash-f")
	writeLog(t, dir, "INFO.log", base.Add(time.Hour), 100, 10, "hash-f")

	res := GrepLatestLogFiles(context.Background(), dir, "hash-f", GrepOptions{})
	if len(res.Logs) != 10 {
		t.Fatalf("expected only the current file's 10 matches, got %d", len(res.Logs))
	}
}

func TestRotationGroup(t *testing.T) {
	cases := map[string]string{
		"/logs/INFO.log":            "INFO.log",
		"/logs/INFO.log.000":        "INFO.log",
		"/logs/INFO.log.2026072711": "INFO.log",
		"/logs/ERROR.log":           "ERROR.log",
		"/logs/stdout":              "stdout",
	}
	for path, want := range cases {
		if got := rotationGroup(path); got != want {
			t.Errorf("rotationGroup(%q) = %q, want %q", path, got, want)
		}
	}
}
