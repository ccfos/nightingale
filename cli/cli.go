package cli

import (
	"github.com/ccfos/nightingale/v6/cli/upgrade"
)

func Upgrade(configFile string) error {
	return upgrade.Upgrade(configFile)
}
