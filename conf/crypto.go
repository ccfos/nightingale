package conf

import (
	"fmt"

	"github.com/ccfos/nightingale/v6/pkg/secu"
)

func decryptConfig(config *ConfigType, cryptoKey string) error {
	decryptDsn, err := secu.DealWithDecrypt(config.DB.DSN, cryptoKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt the db dsn: %s", err)
	}

	config.DB.DSN = decryptDsn

	for k := range config.HTTP.BasicAuth {
		decryptPwd, err := secu.DealWithDecrypt(config.HTTP.BasicAuth[k], cryptoKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt http basic auth password: %s", err)
		}

		config.HTTP.BasicAuth[k] = decryptPwd
	}

	for i, v := range config.Pushgw.Writers {
		decryptWriterPwd, err := secu.DealWithDecrypt(v.BasicAuthPass, cryptoKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt writer basic auth password: %s", err)
		}

		config.Pushgw.Writers[i].BasicAuthPass = decryptWriterPwd
	}

	return nil
}
