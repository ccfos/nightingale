package inf

import "github.com/ccfos/nightingale/v6/alert/aconf"

type EmailRebooter interface {
	Reset(smtp aconf.SMTPConfig)
}
