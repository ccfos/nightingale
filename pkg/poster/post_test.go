package poster

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
)

func init() {
	dir, errWD := os.Getwd()
	if errWD != nil {
		panic(errWD)
	}
	rootPath := filepath.Dir(filepath.Dir(dir))
	config, err := conf.InitConfig(rootPath+"/etc/edge", "")
	if err != nil {
		panic(err)
	}
	c = ctx.NewContext(context.Background(), nil, false, config.CenterApi)

}

var c *ctx.Context

type HeartbeatInfo struct {
	Instance      string `json:"instance"`
	EngineCluster string `json:"engine_cluster"`
	DatasourceId  int64  `json:"datasource_id"`
}

func TestPostByUrls(t *testing.T) {
	type args struct {
		ctx  *ctx.Context
		path string
		v    interface{}
	}

	info := HeartbeatInfo{
		Instance:      "instance",
		EngineCluster: "cluster",
		DatasourceId:  888,
	}
	tests := []struct {
		name string
		args args
	}{
		{"a", args{ctx: c, path: "/v1/n9e/server-heartbeat", v: info}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := PostByUrls(tt.args.ctx, tt.args.path, tt.args.v); err != nil {
				t.Errorf("PostByUrls() error = %v ", err)
			}
		})
	}
}

type AlertCurEvent_Temp struct {
	Id              int64  `json:"id" gorm:"primaryKey"`
	Cate            string `json:"cate"`
	Cluster         string `json:"cluster"`
	DatasourceId    int64  `json:"datasource_id"`
	GroupId         int64  `json:"group_id"`   // busi group id
	GroupName       string `json:"group_name"` // busi group name
	Hash            string `json:"hash"`       // rule_id + vector_key
	RuleId          int64  `json:"rule_id"`
	RuleName        string `json:"rule_name"`
	RuleNote        string `json:"rule_note"`
	RuleProd        string `json:"rule_prod"`
	RuleAlgo        string `json:"rule_algo"`
	Severity        int    `json:"severity"`
	PromForDuration int    `json:"prom_for_duration"`
	PromQl          string `json:"prom_ql"`
}

func TestPostByUrlsWithResp(t *testing.T) {
	type args struct {
		ctx  *ctx.Context
		path string
		v    interface{}
	}
	type testCase[T any] struct {
		name string
		args args
	}
	tests := []testCase[int64]{{
		"a-resp",
		args{ctx: c, path: "/v1/n9e/event-persist", v: AlertCurEvent_Temp{PromQl: "PromQl", Cate: "Cate"}},
	},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotT, err := PostByUrlsWithResp[int64](tt.args.ctx, tt.args.path, tt.args.v)
			if err != nil {
				t.Errorf("PostByUrlsWithResp() error = %v", err)
				return
			}
			t.Logf("PostByUrlsWithResp() gotT = %v", gotT)
		})
	}
}
