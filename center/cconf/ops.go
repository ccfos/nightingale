package cconf

import (
	"path"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/runner"
)

var Operations = Operation{}

type Operation struct {
	Ops []Ops `yaml:"ops"`
}

type Ops struct {
	Name  string   `yaml:"name" json:"name"`
	Cname string   `yaml:"cname" json:"cname"`
	Ops   []string `yaml:"ops" json:"ops"`
}

func LoadOpsYaml(opsYamlFile string) error {
	fp := opsYamlFile
	if fp == "" {
		fp = path.Join(runner.Cwd, "etc", "ops.yaml")
	}
	if !file.IsExist(fp) {
		return nil
	}
	return file.ReadYaml(fp, &Operations)
}

func GetAllOps(ops []Ops) []string {
	var ret []string
	for _, op := range ops {
		ret = append(ret, op.Ops...)
	}
	return ret
}
