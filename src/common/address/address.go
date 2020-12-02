package address

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/runner"
)

type Module struct {
	HTTP      string   `yaml:"http"`
	RPC       string   `yaml:"rpc"`
	Addresses []string `yaml:"addresses"`
}

var (
	lock sync.Once
	mods map[string]Module
)

func GetHTTPListen(mod string) string {
	return getMod(mod).HTTP
}

func GetHTTPPort(mod string) int {
	return convPort(mod, getMod(mod).HTTP, "http")
}

func GetRPCListen(mod string) string {
	return getMod(mod).RPC
}

func GetRPCPort(mod string) int {
	return convPort(mod, getMod(mod).RPC, "rpc")
}

func convPort(module, listen, portType string) int {
	splitChar := ":"
	if IsIPv6(listen) {
		splitChar = "]:"
	}
	port, err := strconv.Atoi(strings.Split(listen, splitChar)[1])
	if err != nil {
		fmt.Printf("%s.%s invalid", module, portType)
		os.Exit(1)
	}

	return port
}

func GetHTTPAddresses(mod string) []string {
	modConf := getMod(mod)

	count := len(modConf.Addresses)
	if count == 0 {
		return []string{}
	}

	port := convPort(mod, modConf.HTTP, "http")

	addresses := make([]string, count)
	for i := 0; i < count; i++ {
		addresses[i] = fmt.Sprintf("%s:%d", modConf.Addresses[i], port)
	}

	return addresses
}

func GetAddresses(mod string) []string {
	modConf := getMod(mod)
	return modConf.Addresses
}

func GetRPCAddresses(mod string) []string {
	modConf := getMod(mod)

	count := len(modConf.Addresses)
	if count == 0 {
		return []string{}
	}

	port := convPort(mod, modConf.RPC, "rpc")

	addresses := make([]string, count)
	for i := 0; i < count; i++ {
		addresses[i] = fmt.Sprintf("%s:%d", modConf.Addresses[i], port)
	}

	return addresses
}

func getMod(modKey string) Module {
	lock.Do(func() {
		parseConf()
	})

	mod, has := mods[modKey]
	if !has {
		fmt.Printf("module(%s) configuration section not found", modKey)
		os.Exit(1)
	}

	return mod
}

func parseConf() {
	conf := getConf()

	var c map[string]Module
	err := file.ReadYaml(conf, &c)
	if err != nil {
		fmt.Println("cannot parse file:", conf)
		os.Exit(1)
	}

	mods = c
}

func getConf() string {
	conf := path.Join(runner.Cwd, "etc", "address.local.yml")
	if file.IsExist(conf) {
		return conf
	}

	conf = path.Join(runner.Cwd, "etc", "address.yml")
	if file.IsExist(conf) {
		return conf
	}

	fmt.Println("configuration file address.[local.]yml not found")
	os.Exit(1)
	return ""
}

func IsIPv6(address string) bool {
	return strings.Count(address, ":") >= 2
}
