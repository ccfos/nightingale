package rsa

import (
	"os"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/ctx"
	"github.com/ccfos/nightingale/v6/pkg/httpx"
	"github.com/ccfos/nightingale/v6/pkg/secu"

	"github.com/pkg/errors"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

func InitRSAConfig(ctx *ctx.Context, rsaConfig *httpx.RSAConfig) error {

	// 1.Load RSA keys from Database
	var (
		hasPrivateKey bool
		hasPublicKey  bool
	)
	val, err := models.ConfigsGet(ctx, models.RSA_PRIVATE_KEY)
	if err != nil {
		return errors.WithMessagef(err, "cannot query config(%s)", models.RSA_PRIVATE_KEY)
	}
	if hasPrivateKey = val != ""; hasPrivateKey {
		rsaConfig.RSAPrivateKey = []byte(val)
	}
	val, err = models.ConfigsGet(ctx, models.RSA_PUBLIC_KEY)
	if err != nil {
		return errors.WithMessagef(err, "cannot query config(%s)", models.RSA_PUBLIC_KEY)
	}
	if hasPublicKey = val != ""; hasPublicKey {
		rsaConfig.RSAPublicKey = []byte(val)
	}

	if hasPrivateKey && hasPublicKey {
		return nil
	}

	// 2.Read RSA configuration from file if exists
	if file.IsExist(rsaConfig.RSAPrivateKeyPath) && file.IsExist(rsaConfig.RSAPublicKeyPath) {
		err = readConfigFile(rsaConfig)
		if err != nil {
			return errors.WithMessage(err, "failed to read rsa config from file")
		}
		return nil
	}
	// 3.Generate RSA keys if not exist
	err = initRSAKeyPairs(ctx, rsaConfig)
	if err != nil {
		return errors.WithMessage(err, "failed to generate rsa key pair")
	}

	return nil
}

func initRSAKeyPairs(ctx *ctx.Context, rsaConfig *httpx.RSAConfig) (err error) {

	// Generate RSA keys

	// Generate RSA password
	if rsaConfig.RSAPassWord != "" {
		logger.Debug("Using existing RSA password")
	} else {
		rsaConfig.RSAPassWord, err = models.InitRSAPassWord(ctx)
		if err != nil {
			return errors.WithMessage(err, "failed to generate rsa password")
		}
	}
	privateByte, publicByte, err := secu.GenerateRsaKeyPair(rsaConfig.RSAPassWord)
	if err != nil {
		return errors.WithMessage(err, "failed to generate rsa key pair")
	}
	// Save generated RSA keys
	err = models.ConfigsSet(ctx, models.RSA_PRIVATE_KEY, string(privateByte))
	if err != nil {
		return errors.WithMessagef(err, "failed to set config(%s)", models.RSA_PRIVATE_KEY)
	}
	err = models.ConfigsSet(ctx, models.RSA_PUBLIC_KEY, string(publicByte))
	if err != nil {
		return errors.WithMessagef(err, "failed to set config(%s)", models.RSA_PUBLIC_KEY)
	}
	rsaConfig.RSAPrivateKey = privateByte
	rsaConfig.RSAPublicKey = publicByte
	return nil
}

func readConfigFile(rsaConfig *httpx.RSAConfig) error {
	publicBuf, err := os.ReadFile(rsaConfig.RSAPublicKeyPath)
	if err != nil {
		return errors.WithMessagef(err, "could not read RSAPublicKeyPath %q", rsaConfig.RSAPublicKeyPath)
	}
	rsaConfig.RSAPublicKey = publicBuf
	privateBuf, err := os.ReadFile(rsaConfig.RSAPrivateKeyPath)
	if err != nil {
		return errors.WithMessagef(err, "could not read RSAPrivateKeyPath %q", rsaConfig.RSAPrivateKeyPath)
	}
	rsaConfig.RSAPrivateKey = privateBuf
	return nil
}
