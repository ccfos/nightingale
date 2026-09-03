package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFetchTempFile_RejectsBadPaths(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"empty", "", "is empty"},
		{"outside tmpdir", "/etc/passwd", "must live under"},
		{"wrong prefix", filepath.Join(os.TempDir(), "evil.yml"), "basename must start with"},
		{"nonexistent prefixed", filepath.Join(os.TempDir(), HTTPFetchTempFilePrefix+"nope.yml"), "open payload_file"},
	}
	for _, c := range cases {
		_, err := ReadFetchTempFile(c.path)
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("[%s] got err=%v, want substring %q", c.name, err, c.wantErr)
		}
	}
}

func TestReadFetchTempFile_AcceptsValidPath(t *testing.T) {
	f, err := os.CreateTemp("", HTTPFetchTempFilePrefix+"*.yml")
	if err != nil { t.Fatal(err) }
	defer os.Remove(f.Name())
	want := "alert: A\nexpr: up == 0\n"
	if _, err := f.WriteString(want); err != nil { t.Fatal(err) }
	f.Close()

	got, err := ReadFetchTempFile(f.Name())
	if err != nil { t.Fatalf("err: %v", err) }
	if string(got) != want {
		t.Errorf("got %q, want %q", string(got), want)
	}
}
