package router

import "testing"

func TestVerifyCollectContent(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name: "categraf toml",
			content: `[[instances]]
targets = ["127.0.0.1:9090"]
interval = "15s"`,
		},
		{
			name: "single doc yaml",
			content: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus-agent`,
		},
		{
			// yaml.Unmarshal 对 --- 文档流只校验第一个文档，后续文档不做校验，
			// 这里仅保证这类内容能被接受
			name: "multi doc yaml",
			content: `apiVersion: v1
kind: ConfigMap
metadata:
  name: scrape-config
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prometheus-agent
spec:
  template:
    spec:
      volumes:
      - emptyDir: {}
        name: prometheus-wal`,
		},
		{
			name:    "leading separator",
			content: "---\nkind: ConfigMap\nmetadata:\n  name: categraf-config",
		},
		{
			name:    "comment only is valid toml",
			content: "# just a comment",
		},
		{
			name:    "neither toml nor yaml",
			content: "foo: bar\n\tbaz = [",
			wantErr: true,
		},
		{
			name:    "bare scalar is not a config",
			content: "hello world",
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := verifyCollectContent(c.content)
			if c.wantErr && err == nil {
				t.Fatal("expect error, got nil")
			}

			if !c.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
