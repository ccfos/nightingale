package upgrade

import (
	"bytes"
	"path"

	"github.com/ccfos/nightingale/v6/pkg/cfg"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/ccfos/nightingale/v6/pkg/tlsx"
	"github.com/koding/multiconfig"
)

type Config struct {
	DB       ormx.DBConfig
	Clusters []ClusterOptions
}

type ClusterOptions struct {
	Name string
	Prom string

	BasicAuthUser string
	BasicAuthPass string

	Headers []string

	Timeout     int64
	DialTimeout int64

	UseTLS bool
	tlsx.ClientConfig

	MaxIdleConnsPerHost int
}

func Parse(fpath string, configPtr interface{}) error {
	var (
		tBuf []byte
	)
	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}
	s := cfg.NewFileScanner()

	s.Read(path.Join(fpath))
	tBuf = append(tBuf, s.Data()...)
	tBuf = append(tBuf, []byte("\n")...)

	if s.Err() != nil {
		return s.Err()
	}

	if len(tBuf) != 0 {
		loaders = append(loaders, &multiconfig.TOMLLoader{Reader: bytes.NewReader(tBuf)})
	}

	m := multiconfig.DefaultLoader{
		Loader:    multiconfig.MultiLoader(loaders...),
		Validator: multiconfig.MultiValidator(&multiconfig.RequiredValidator{}),
	}
	return m.Load(configPtr)
}
