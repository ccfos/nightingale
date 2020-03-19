package identity

import (
	"log"

	"github.com/toolkits/pkg/sys"
)

var (
	Identity string
)

type IdentitySection struct {
	Specify string `yaml:"specify"`
	Shell   string `yaml:"shell"`
}

func Init(identity IdentitySection) {
	if identity.Specify != "" {
		Identity = identity.Specify
		return
	}

	var err error
	Identity, err = sys.CmdOutTrim("bash", "-c", identity.Shell)
	if err != nil {
		log.Fatalln("[F] cannot get identity")
	}
}
