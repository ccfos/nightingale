package statik

import (
	"os"
	"strings"
	"testing"

	statikfs "github.com/rakyll/statik/fs"
)

func TestEmbeddedFrontendKeepsPortInCategrafInstallFlow(t *testing.T) {
	hfs, err := statikfs.New()
	if err != nil {
		t.Fatalf("create statik fs: %v", err)
	}

	var texts []string
	err = statikfs.Walk(hfs, "/", func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		data, err := statikfs.ReadFile(hfs, path)
		if err != nil {
			return err
		}
		texts = append(texts, string(data))
		return nil
	})
	if err != nil {
		t.Fatalf("walk statik fs: %v", err)
	}

	joined := strings.Join(texts, "\n")
	for _, want := range []string{"/prometheus/v1/write", "/v1/n9e/heartbeat"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("embedded asset missing %q", want)
		}
	}

	if !strings.Contains(joined, "http://${n9e_addr}/v1/n9e-plus/collects") {
		t.Fatalf("embedded asset missing updated n9e collect endpoint placeholder")
	}

	for _, bad := range []string{
		"http://127.0.0.1:17000/prometheus/v1/write",
		"http://127.0.0.1:17000/v1/n9e/heartbeat",
		"http://127.0.0.1:17000/v1/n9e-plus/collects",
		"http://${n9e_ip}:17000/v1/n9e-plus/collects",
	} {
		if strings.Contains(joined, bad) {
			t.Fatalf("embedded asset still hardcodes %q", bad)
		}
	}
}
