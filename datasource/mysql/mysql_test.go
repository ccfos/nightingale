package mysql

import (
	"testing"

	dsmysql "github.com/ccfos/nightingale/v6/dskit/mysql"
)

func newMySQL(shard dsmysql.Shard) *MySQL {
	m := new(MySQL)
	m.Shards = []dsmysql.Shard{shard}
	return m
}

func TestEqualDetectsDsnExtraParamsChange(t *testing.T) {
	base := dsmysql.Shard{Addr: "127.0.0.1:3306", User: "root"}

	old := newMySQL(base)
	if !old.Equal(newMySQL(base)) {
		t.Fatal("identical shards should be equal")
	}

	changed := base
	changed.DsnExtraParams = "sessionVariables=ob_read_consistency=WEAK"
	if old.Equal(newMySQL(changed)) {
		t.Fatal("changing dsn_extra_params must invalidate the cached datasource")
	}
}
