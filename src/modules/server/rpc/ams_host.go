package rpc

import (
	"fmt"

	"github.com/didi/nightingale/v4/src/models"
)

func (*Server) HostRegister(host models.HostRegisterForm, output *string) error {
	host.Validate()
	err := models.HostRegister(host)
	if err != nil {
		*output = fmt.Sprintf("%v", err)
	}

	return nil
}
