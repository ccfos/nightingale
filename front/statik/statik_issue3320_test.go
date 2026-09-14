package statik

import (
	"os"
	"strings"
	"testing"

	statikfs "github.com/rakyll/statik/fs"
)

func TestEmbeddedFrontendContainsVictoriaLogsContextSupport(t *testing.T) {
	hfs, err := statikfs.New()
	if err != nil {
		t.Fatalf("create statik fs: %v", err)
	}

	var found bool
	err = statikfs.Walk(hfs, "/", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".js") {
			return nil
		}
		data, err := statikfs.ReadFile(hfs, path)
		if err != nil {
			return err
		}
		text := string(data)
		if strings.Contains(text, "/victorialogs-context") && strings.Contains(text, "context_invalid_time") {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk statik fs: %v", err)
	}
	if !found {
		t.Fatal("embedded frontend is missing VictoriaLogs context support")
	}
}
