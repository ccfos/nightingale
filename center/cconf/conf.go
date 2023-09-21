package cconf

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ccfos/nightingale/v6/pkg/secu"
)

type Center struct {
	Plugins                []Plugin
	MetricsYamlFile        string
	OpsYamlFile            string
	BuiltinIntegrationsDir string
	I18NHeaderKey          string
	MetricDesc             MetricDescType
	AnonymousAccess        AnonymousAccess
	UseFileAssets          bool
	Encryption             RSAEncryption
}

type Plugin struct {
	Id       int64  `json:"id"`
	Category string `json:"category"`
	Type     string `json:"plugin_type"`
	TypeName string `json:"plugin_type_name"`
}

type AnonymousAccess struct {
	PromQuerier bool
	AlertDetail bool
}
type RSAEncryption struct {
	RSAPublicKey      []byte
	RSAPublicKeyPath  string
	RSAPrivateKey     []byte
	RSAPrivateKeyPath string
	RSAPassWord       string
}

func (c *Center) PreCheck() {
	if len(c.Plugins) == 0 {
		c.Plugins = Plugins
	}
}

func (c *Center) InitRSAEncryption() {
	initRSAFile(c.Encryption)
	publicBuf, err := os.ReadFile(c.Encryption.RSAPublicKeyPath)
	if err != nil {
		panic(fmt.Errorf("could not read Center.Encryption.RSAPublicKeyPath %q: %v", c.Encryption.RSAPublicKeyPath, err))
	}
	c.Encryption.RSAPublicKey = publicBuf
	privateBuf, err := os.ReadFile(c.Encryption.RSAPrivateKeyPath)
	if err != nil {
		panic(fmt.Errorf("could not read Center.Encryption.RSAPrivateKeyPath %q: %v", c.Encryption.RSAPrivateKeyPath, err))
	}
	c.Encryption.RSAPrivateKey = privateBuf
}

func initRSAFile(encryption RSAEncryption) {
	dirPath := filepath.Dir(encryption.RSAPrivateKeyPath)
	// Check if the directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		err := os.MkdirAll(dirPath, os.ModePerm)
		if err != nil {
			panic(fmt.Errorf("could not create directory for Center.Encryption %q: %v", dirPath, err))
			return
		}
	}
	// Check if the file exists
	if _, err := os.Stat(encryption.RSAPrivateKeyPath); os.IsNotExist(err) {
		err := secu.GenerateKeyWithPassword(encryption.RSAPrivateKeyPath, encryption.RSAPublicKeyPath, encryption.RSAPassWord)
		if err != nil {
			panic(fmt.Errorf("could not create file for Center.Encryption %+v: %v", encryption, err))
			return
		}
	}
}
