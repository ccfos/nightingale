package clickhouse

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/ccfos/nightingale/v6/dskit/types"
)

func Test_Timeseries(t *testing.T) {
	ck := &Clickhouse{
		Nodes:    []string{"127.0.0.1:8123"},
		User:     "default",
		Password: "123456",
	}

	err := ck.InitCli()
	if err != nil {
		t.Fatal(err)
	}

	data, err := ck.QueryTimeseries(context.TODO(), &QueryParam{
		Sql:        `select * from default.student limit 20`,
		From:       time.Now().Unix() - 300,
		To:         time.Now().Unix(),
		TimeField:  "created_at",
		TimeFormat: "datetime",
		Keys: types.Keys{
			LabelKey: "age",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	bs, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(string(bs))
}
