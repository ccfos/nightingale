package cfg

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/koding/multiconfig"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

func LoadConfigByDir(configDir string, configPtr interface{}) error {
	var (
		tBuf []byte
	)

	loaders := []multiconfig.Loader{
		&multiconfig.TagLoader{},
		&multiconfig.EnvironmentLoader{},
	}

	if !file.IsExist(configDir) {
		logger.Errorf("dir %s not exist\n", configDir)
		os.Exit(1)
	}

	files, err := file.FilesUnder(configDir)
	if err != nil {
		return fmt.Errorf("failed to list files under: %s : %v", configDir, err)
	}

	var found bool
	err = filepath.Walk(configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			ext := filepath.Ext(path)
			if ext == ".toml" || ext == ".yaml" || ext == ".json" {
				found = true
				return nil
			}
		}
		return nil
	})
	if err != nil || !found {
		logger.Errorf("fail to found config file, config dir path: %v and err is %v\n", configDir, err)
		os.Exit(1)
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
