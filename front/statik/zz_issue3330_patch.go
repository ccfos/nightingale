package statik

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"

	statikfs "github.com/rakyll/statik/fs"
)

var issue3330Replacer = strings.NewReplacer(
	"http://127.0.0.1:17000/prometheus/v1/write", "/prometheus/v1/write",
	"http://127.0.0.1:17000/v1/n9e/heartbeat", "/v1/n9e/heartbeat",
	"http://${n9e_ip}:17000/v1/n9e-plus/collects", "http://${n9e_addr}/v1/n9e-plus/collects",
	"${n9e_ip}", "${n9e_addr}",
)

func init() {
	hfs, err := statikfs.New()
	if err != nil {
		panic(err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
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

		w, err := zw.Create(strings.TrimPrefix(path, "/"))
		if err != nil {
			return err
		}

		_, err = w.Write([]byte(issue3330Replacer.Replace(string(data))))
		return err
	})
	if err != nil {
		panic(err)
	}
	if err := zw.Close(); err != nil {
		panic(err)
	}

	statikfs.Register(buf.String())
}
