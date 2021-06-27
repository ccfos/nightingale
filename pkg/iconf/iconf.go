package iconf

// just for nightingale only

import (
	"path"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/runner"
)

func GetYmlFile(module string) string {
	confdir := path.Join(runner.Cwd, "etc")

	yml := path.Join(confdir, module+".local.yml")
	if file.IsExist(yml) {
		return yml
	}

	yml = path.Join(confdir, module+".yml")
	if file.IsExist(yml) {
		return yml
	}

	return ""
}
