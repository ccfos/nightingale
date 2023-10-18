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
	rsaPassWord, err := models.ConfigsGet(ctx, models.RSA_PASSWORD)
	if err != nil {
		return errors.WithMessagef(err, "cannot query config(%s)", models.RSA_PASSWORD)
	}
	privateKeyVal, err := models.ConfigsGet(ctx, models.RSA_PRIVATE_KEY)
	if err != nil {
		return errors.WithMessagef(err, "cannot query config(%s)", models.RSA_PRIVATE_KEY)
	}
	publicKeyVal, err := models.ConfigsGet(ctx, models.RSA_PUBLIC_KEY)
	if err != nil {
		return errors.WithMessagef(err, "cannot query config(%s)", models.RSA_PUBLIC_KEY)
	}
	if rsaPassWord != "" && privateKeyVal != "" && publicKeyVal != "" {
		rsaConfig.RSAPassWord = rsaPassWord
		rsaConfig.RSAPrivateKey = []byte(privateKeyVal)
		rsaConfig.RSAPublicKey = []byte(publicKeyVal)
		return nil
	}

	// 2.Read RSA configuration from file if exists
	if file.IsExist(rsaConfig.RSAPrivateKeyPath) && file.IsExist(rsaConfig.RSAPublicKeyPath) {
		//password already read from config
		rsaConfig.RSAPrivateKey, rsaConfig.RSAPublicKey, err = readConfigFile(rsaConfig)
		if err != nil {
			return errors.WithMessage(err, "failed to read rsa config from file")
		}
		return nil
	}
	// 3.Generate RSA keys if not exist
	rsaConfig.RSAPassWord, rsaConfig.RSAPrivateKey, rsaConfig.RSAPublicKey, err = initRSAKeyPairs(ctx, rsaConfig.RSAPassWord)
	if err != nil {
		return errors.WithMessage(err, "failed to generate rsa key pair")
	}
	return nil
}

func initRSAKeyPairs(ctx *ctx.Context, rsaPassWord string) (password string, privateByte, publicByte []byte, err error) {

	// Generate RSA keys

	// Generate RSA password
	if rsaPassWord != "" {
		logger.Debug("Using existing RSA password")
		password = rsaPassWord
		err = models.ConfigsSet(ctx, models.RSA_PASSWORD, password)
		if err != nil {
			err = errors.WithMessagef(err, "failed to set config(%s)", models.RSA_PASSWORD)
			return
		}
	} else {
		password, err = models.InitRSAPassWord(ctx)
		if err != nil {
			err = errors.WithMessage(err, "failed to generate rsa password")
			return
		}
	}
	privateByte, publicByte, err = secu.GenerateRsaKeyPair(password)
	if err != nil {
		err = errors.WithMessage(err, "failed to generate rsa key pair")
		return
	}
	// Save generated RSA keys
	err = models.ConfigsSet(ctx, models.RSA_PRIVATE_KEY, string(privateByte))
	if err != nil {
		err = errors.WithMessagef(err, "failed to set config(%s)", models.RSA_PRIVATE_KEY)
		return
	}
	err = models.ConfigsSet(ctx, models.RSA_PUBLIC_KEY, string(publicByte))
	if err != nil {
		err = errors.WithMessagef(err, "failed to set config(%s)", models.RSA_PUBLIC_KEY)
		return
	}
	return
}

func readConfigFile(rsaConfig *httpx.RSAConfig) (privateBuf, publicBuf []byte, err error) {
	publicBuf, err = os.ReadFile(rsaConfig.RSAPublicKeyPath)
	if err != nil {
		err = errors.WithMessagef(err, "could not read RSAPublicKeyPath %q", rsaConfig.RSAPublicKeyPath)
		return
	}
	privateBuf, err = os.ReadFile(rsaConfig.RSAPrivateKeyPath)
	if err != nil {
		err = errors.WithMessagef(err, "could not read RSAPrivateKeyPath %q", rsaConfig.RSAPrivateKeyPath)
	}
	return
}
