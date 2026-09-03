package dscache

import (
	"testing"

	"github.com/ccfos/nightingale/v6/datasource"
	lokikit "github.com/ccfos/nightingale/v6/dskit/loki"
	"github.com/ccfos/nightingale/v6/models"
)

func TestLokiN9eToDatasourceInfoMergesSettingsJson(t *testing.T) {
	ds := &datasource.DatasourceInfo{}
	item := models.Datasource{
		Name:        "prod-loki",
		ClusterName: "edge-a",
		SettingsJson: map[string]interface{}{
			"custom":    "kept",
			"loki.addr": "http://old.example",
		},
		HTTPJson: models.HTTP{
			Url:                 "http://loki.example",
			Timeout:             1234,
			DialTimeout:         567,
			MaxIdleConnsPerHost: 8,
			Headers:             map[string]string{"X-Scope-OrgID": "team-a"},
			TLS: models.TLS{
				SkipTlsVerify: true,
			},
		},
		AuthJson: models.Auth{
			BasicAuthUser:     "user",
			BasicAuthPassword: "pass",
		},
	}

	lokiN9eToDatasourceInfo(ds, item)

	if got := ds.Settings["custom"]; got != "kept" {
		t.Fatalf("expected custom setting to be preserved, got %#v", got)
	}
	if got := ds.Settings["loki.addr"]; got != "http://loki.example" {
		t.Fatalf("expected http url to override settings addr, got %#v", got)
	}
	if got := ds.Settings["loki.cluster_name"]; got != "edge-a" {
		t.Fatalf("expected cluster name from datasource cluster_name, got %#v", got)
	}
	basic, ok := ds.Settings["loki.basic"].(lokikit.LokiBasicAuth)
	if !ok {
		t.Fatalf("expected loki.basic type, got %#v", ds.Settings["loki.basic"])
	}
	if basic.LokiUser != "user" || basic.LokiPass != "pass" {
		t.Fatalf("unexpected basic auth: %#v", basic)
	}
}

func TestLokiN9eToDatasourceInfoDoesNotInventMaxQueryRows(t *testing.T) {
	ds := &datasource.DatasourceInfo{}

	lokiN9eToDatasourceInfo(ds, models.Datasource{})

	if _, exists := ds.Settings["loki.max_query_rows"]; exists {
		t.Fatalf("loki.max_query_rows should not be invented by dscache conversion")
	}
}
