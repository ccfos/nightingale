package config

import (
	"fmt"

	"github.com/toolkits/pkg/file"
)

type opsStruct []struct {
	System string `json:"system"`
	Groups []struct {
		Title string `json:"title"`
		Ops   []struct {
			En string `json:"en"`
			Cn string `json:"cn"`
		} `json:"ops"`
	} `json:"groups"`
}

var (
	GlobalOps opsStruct
	LocalOps  opsStruct
)

func parseOps() error {
	globalOpsFile := "etc/gop.local.yml"
	if !file.IsExist(globalOpsFile) {
		globalOpsFile = "etc/gop.yml"
	}

	if !file.IsExist(globalOpsFile) {
		return fmt.Errorf("%s not exists", globalOpsFile)
	}

	var gc opsStruct
	err := file.ReadYaml(globalOpsFile, &gc)
	if err != nil {
		return fmt.Errorf("parse %s fail: %v", globalOpsFile, err)
	}

	GlobalOps = gc

	localOpsFile := "etc/lop.local.yml"
	if !file.IsExist(localOpsFile) {
		localOpsFile = "etc/lop.yml"
	}

	if !file.IsExist(localOpsFile) {
		return fmt.Errorf("%s not exists", localOpsFile)
	}

	var lc opsStruct
	err = file.ReadYaml(localOpsFile, &lc)
	if err != nil {
		return fmt.Errorf("parse %s fail: %v", localOpsFile, err)
	}

	LocalOps = lc

	return nil
}
