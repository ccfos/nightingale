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
	GlobalOps    opsStruct
	LocalOps     opsStruct
	LocalOpsList []string
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

	m := map[string]struct{}{}
	for _, v := range lc {
		for _, v2 := range v.Groups {
			for _, v3 := range v2.Ops {
				m[v3.En] = struct{}{}
			}
		}
	}
	LocalOpsList = []string{}
	for k, _ := range m {
		LocalOpsList = append(LocalOpsList, k)
	}

	return nil
}
