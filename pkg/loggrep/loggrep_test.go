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

	lines, stopped, incomplete := grepFile(context.Background(), path, "hash-g", time.Now().Add(-time.Second), MaxLogLines)

	if !stopped || !incomplete {
		t.Fatalf("expected the scan to stop on the deadline, got stopped=%v incomplete=%v", stopped, incomplete)
	}
	if len(lines) >= 5000 {
		t.Fatalf("expected to stop early, but got %d of the 5000 matches", len(lines))
	}
}

// The whole point of the windowed backward scan: a scan that cannot read the
// whole file must give back the newest lines, not the oldest. Before the fix
// the file was read forward, so a budget that ran out mid file returned its
// oldest lines - and the event being investigated, which sits at the end, was
// exactly what got dropped.
func TestGrepFileReturnsNewestMatchesFirst(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)
	// 400 lines, every 4th matches -> 100 matches spread over the whole file
	path := writeLog(t, dir, "INFO.log", base, 400, 4, "hash-h")

	// a window per handful of lines, so the scan really does walk the file in
	// pieces the way it does on a multi-hundred-MB production log
	defer withScanWindow(200)()

	lines, stopped, incomplete := grepFile(context.Background(), path, "hash-h", time.Now().Add(time.Minute), 5)

	if stopped || incomplete {
		t.Fatalf("a limit stop is not a truncated read of the file, got stopped=%v incomplete=%v", stopped, incomplete)
	}
	// it stops as soon as it is past the limit, so nowhere near all 100
	if len(lines) <= 5 || len(lines) > 20 {
		t.Fatalf("expected to stop just past the limit of 5, got %d of the 100 matches", len(lines))
	}

	// every returned line must come from the newest end of the file: the
	// oldest of them is still newer than the ~80 matches left unread
	oldestKept := lines[0]
	for _, l := range lines {
		if l < oldestKept {
			oldestKept = l
		}
	}
	cutoff := base.Add(300 * time.Second).Format("2006-01-02 15:04:05")
	if oldestKept < cutoff {
		t.Fatalf("expected only matches from the tail of the file, got %q (cutoff %q)", oldestKept, cutoff)
	}
}

// The windows have to tile the file exactly: a line straddling a boundary must
// be returned once, by the window it starts in, and never dropped or doubled.
func TestGrepFileWindowsTileTheFile(t *testing.T) {
	dir := t.TempDir()
	path := writeLog(t, dir, "INFO.log", time.Now().Add(-time.Hour), 500, 1, "hash-i")

	want, _, _ := grepFile(context.Background(), path, "hash-i", time.Now().Add(time.Minute), MaxLogLines)
	if len(want) != 500 {
		t.Fatalf("expected all 500 matches in a single window, got %d", len(want))
	}

	// every window size, including ones that land mid line, must agree
	for _, w := range []int64{7, 13, 64, 199, 1024} {
		t.Run(fmt.Sprintf("window=%d", w), func(t *testing.T) {
			defer withScanWindow(w)()

			got, _, _ := grepFile(context.Background(), path, "hash-i", time.Now().Add(time.Minute), MaxLogLines)
			if len(got) != len(want) {
				t.Fatalf("window %d returned %d lines, want %d", w, len(got), len(want))
			}

			seen := make(map[string]int, len(got))
			for _, l := range got {
				seen[l]++
			}
			for _, l := range want {
				if seen[l] != 1 {
					t.Fatalf("window %d: line %q appeared %d times, want exactly 1", w, l, seen[l])
				}
			}
		})
	}
}

// An over-long line used to blow the bufio.Scanner buffer and silently
// abandon everything after it in that file.
func TestGrepFileOverLongLineDoesNotDropTheRest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "INFO.log")

	base := time.Now().Add(-time.Hour)
	stamp := func(d time.Duration) string {
		return base.Add(d).Format("2006-01-02 15:04:05.000000")
	}
	body := strings.Repeat("x", 4096)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s INFO hash-j before\n", stamp(0))
	fmt.Fprintf(&sb, "%s INFO %s hash-j past-the-cap\n", stamp(time.Second), body)
	fmt.Fprintf(&sb, "%s INFO hash-j after\n", stamp(2*time.Second))

	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	defer withMaxLineBytes(64)()

	lines, stopped, incomplete := grepFile(context.Background(), path, "hash-j", time.Now().Add(time.Minute), MaxLogLines)
	if stopped || incomplete {
		t.Fatalf("an over-long line is not a failed read, got stopped=%v incomplete=%v", stopped, incomplete)
	}

	// the giant line's keyword sits past the cap so it cannot match, but the
	// lines on either side of it must both be found
	var before, after bool
	for _, l := range lines {
		if strings.Contains(l, "before") {
			before = true
		}
		if strings.Contains(l, "after") {
			after = true
		}
	}
	if !before || !after {
		t.Fatalf("expected the lines around the over-long one to survive, got before=%v after=%v (%d lines)", before, after, len(lines))
	}
}

func withScanWindow(n int64) func() {
	prev := scanWindow
	scanWindow = n
	return func() { scanWindow = prev }
}

func withMaxLineBytes(n int) func() {
	prev := maxLineBytes
	maxLineBytes = n
	return func() { maxLineBytes = prev }
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
