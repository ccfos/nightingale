package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ccfos/nightingale/v6/pkg/dsauth"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/tsdb/tconf"
)

func TestDatasourceAuthPolicyConfig(t *testing.T) {
	previousPolicy := dsauth.GetPolicy()
	t.Cleanup(func() { dsauth.SetPolicy(previousPolicy) })
	for _, process := range []struct {
		name string
		init func(string, string) (*ConfigType, error)
	}{
		{"center", InitCenterConfig},
		{"edge-alert", InitConfig},
	} {
		for _, restricted := range []bool{false, true} {
			name := "default"
			if restricted {
				name = "restricted"
			}
			t.Run(process.name+"/"+name, func(t *testing.T) {
				dir := t.TempDir()
				content := "[Alert.Heartbeat]\nIP = \"127.0.0.1\"\n"
				if restricted {
					content += `
[DatasourceAuthPolicy]
DisableAmbientCredentials = true
DisableAssumeRole = true
SigV4AllowedServices = ["es"]
`
				}
				if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
				if _, err := process.init(dir, ""); err != nil {
					t.Fatal(err)
				}

				checks := []struct {
					name string
					cfg  dsauth.SigV4Config
					want string
				}{
					{"ambient", dsauth.SigV4Config{Region: "us-east-1", Service: "es"}, "DatasourceAuthPolicy.DisableAmbientCredentials"},
					{"assume-role", dsauth.SigV4Config{Region: "us-east-1", Service: "es", CredentialType: "keys", AccessKey: "key", SecretKey: "secret", RoleArn: "arn:aws:iam::123456789012:role/query"}, "DatasourceAuthPolicy.DisableAssumeRole"},
					{"service", dsauth.SigV4Config{Region: "us-east-1", Service: "aoss", CredentialType: "keys", AccessKey: "key", SecretKey: "secret"}, "not allowed"},
				}
				for _, check := range checks {
					err := dsauth.Validate(dsauth.Config{Type: dsauth.TypeSigV4, SigV4: &check.cfg})
					if restricted {
						if err == nil || !strings.Contains(err.Error(), check.want) {
							t.Errorf("%s: expected restriction %q, got %v", check.name, check.want, err)
						}
					} else if err != nil {
						t.Errorf("%s: policy should be optional, got %v", check.name, err)
					}
				}
				if err := dsauth.Validate(dsauth.Config{Type: dsauth.TypeSigV4, SigV4: &dsauth.SigV4Config{
					Region: "us-east-1", Service: "es", CredentialType: "keys", AccessKey: "key", SecretKey: "secret",
				}}); err != nil {
					t.Errorf("explicit credentials for an allowed service should remain usable: %v", err)
				}
			})
		}
	}
}

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
