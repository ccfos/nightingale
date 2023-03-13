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

	for k := range config.HTTP.Alert.BasicAuth {
		decryptPwd, err := secu.DealWithDecrypt(config.HTTP.Alert.BasicAuth[k], cryptoKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt http basic auth password: %s", err)
		}

		config.HTTP.Alert.BasicAuth[k] = decryptPwd
	}

	for k := range config.HTTP.Pushgw.BasicAuth {
		decryptPwd, err := secu.DealWithDecrypt(config.HTTP.Pushgw.BasicAuth[k], cryptoKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt http basic auth password: %s", err)
		}

		config.HTTP.Pushgw.BasicAuth[k] = decryptPwd
	}

	for k := range config.HTTP.Heartbeat.BasicAuth {
		decryptPwd, err := secu.DealWithDecrypt(config.HTTP.Heartbeat.BasicAuth[k], cryptoKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt http basic auth password: %s", err)
		}

		config.HTTP.Heartbeat.BasicAuth[k] = decryptPwd
	}

	for k := range config.HTTP.Service.BasicAuth {
		decryptPwd, err := secu.DealWithDecrypt(config.HTTP.Service.BasicAuth[k], cryptoKey)
		if err != nil {
			return fmt.Errorf("failed to decrypt http basic auth password: %s", err)
		}
		config.HTTP.Service.BasicAuth[k] = decryptPwd
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
