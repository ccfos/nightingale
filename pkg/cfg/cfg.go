package cfg

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/koding/multiconfig"
	"github.com/toolkits/pkg/file"
)

func LoadConfigByDir(configDir string, configPtr interface{}) error {
	var (
		tBuf []byte
	)

	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}

	files, err := file.FilesUnder(configDir)
	if err != nil {
		return fmt.Errorf("failed to list files under: %s : %v", configDir, err)
	}
	s := NewFileScanner()
	for _, fpath := range files {
		switch {
		case strings.HasSuffix(fpath, ".toml"):
			s.Read(path.Join(configDir, fpath))
			tBuf = append(tBuf, s.Data()...)
			tBuf = append(tBuf, []byte("\n")...)
		case strings.HasSuffix(fpath, ".json"):
			loaders = append(loaders, &multiconfig.JSONLoader{Path: path.Join(configDir, fpath)})
		case strings.HasSuffix(fpath, ".yaml") || strings.HasSuffix(fpath, ".yml"):
			loaders = append(loaders, &multiconfig.YAMLLoader{Path: path.Join(configDir, fpath)})
		}
		if s.Err() != nil {
			return s.Err()
		}
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
