package idents

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ccfos/nightingale/v6/conf"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/storage"
)

func init() {
	dir, errWD := os.Getwd()
	if errWD != nil {
		panic(errWD)
	}
	rootPath := filepath.Dir(filepath.Dir(dir))
	config, err := conf.InitConfig(rootPath+"/etc", "")
	db, err := storage.New(config.DB)
	if err != nil {
		panic(err)
	}
	ctx := ctx.NewContext(context.Background(), db, true)
	s = New(ctx)
}

var s *Set

func TestSet_updateTimestamp(t *testing.T) {

	t.Run("test1", func(t *testing.T) {

		s.updateTimestamp(map[string]*TargetHeartBeat{
			"ident3": &TargetHeartBeat{HostIp: "3.1.1.1"},
			"ident4": &TargetHeartBeat{HostIp: "4.2.2.2"},
		})
	})

}
