package conf

import (
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
