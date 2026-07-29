package conf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/tsdb/tconf"
)

func TestEmbeddedTSDBWriter(t *testing.T) {
	config := &ConfigType{
		HTTP:         httpx.Config{Host: "0.0.0.0", Port: 17000},
		EmbeddedTSDB: tconf.EmbeddedTSDB{Enable: true, BasicAuthUser: "u", BasicAuthPass: "p"},
	}

	w := embeddedTSDBWriter(config)
	if w.Url != "http://127.0.0.1:17000/prometheus/api/v1/write" {
		t.Fatalf("unexpected url: %s", w.Url)
	}
	if w.BasicAuthUser != "u" || w.BasicAuthPass != "p" {
		t.Fatalf("basic auth not propagated: %+v", w)
	}
	if w.UseTLS {
		t.Fatal("UseTLS should be false without cert")
	}

	config.HTTP.Host = "10.1.2.3"
	config.HTTP.CertFile = "cert.pem"
	config.HTTP.KeyFile = "key.pem"
	w = embeddedTSDBWriter(config)
	if w.Url != "https://10.1.2.3:17000/prometheus/api/v1/write" {
		t.Fatalf("unexpected tls url: %s", w.Url)
	}
	if !w.UseTLS || !w.InsecureSkipVerify {
		t.Fatalf("tls options not set: %+v", w.ClientConfig)
	}
}

// only center mounts /prometheus/api/v1/write, so only center may inject the
// writer pointing at it. edge / alert / pushgw default to the same etc dir and
// would otherwise forward every sample to a 404 on their own port.
func TestEmbeddedTSDBWriterInjectedForCenterOnly(t *testing.T) {
	dir := t.TempDir()
	content := `
[HTTP]
Host = "0.0.0.0"
Port = 17000

[EmbeddedTSDB]
Enable = true
Dir = "data/tsdb"
`
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write config fail: %v", err)
	}

	center, err := InitCenterConfig(dir, "")
	if err != nil {
		t.Fatalf("InitCenterConfig fail: %v", err)
	}
	if !center.EmbeddedTSDB.Enable {
		t.Fatal("center should keep EmbeddedTSDB enabled")
	}
	if len(center.Pushgw.Writers) != 1 {
		t.Fatalf("center should get exactly one writer, got %d", len(center.Pushgw.Writers))
	}
	if center.Pushgw.Writers[0].Url != "http://127.0.0.1:17000/prometheus/api/v1/write" {
		t.Fatalf("unexpected center writer url: %s", center.Pushgw.Writers[0].Url)
	}

	other, err := InitConfig(dir, "")
	if err != nil {
		t.Fatalf("InitConfig fail: %v", err)
	}
	if other.EmbeddedTSDB.Enable {
		t.Fatal("non-center process should have EmbeddedTSDB disabled")
	}
	if len(other.Pushgw.Writers) != 0 {
		t.Fatalf("non-center process should get no writer, got %+v", other.Pushgw.Writers)
	}
}
