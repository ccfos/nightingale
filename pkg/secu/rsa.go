package secu

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"

	"github.com/toolkits/pkg/logger"
)

func Decrypt(cipherText string, privateKeyByte []byte, password string) (decrypted string, err error) {
	decodeCipher, _ := base64.StdEncoding.DecodeString(cipherText)
	//pem解码
	block, _ := pem.Decode(privateKeyByte)
	var privateKey *rsa.PrivateKey
	var decryptedPrivateKeyBytes []byte
	if block == nil {
		return "", fmt.Errorf("private key block is nil")
	}
	decryptedPrivateKeyBytes, err = x509.DecryptPEMBlock(block, []byte(password))
	if err == nil {
		privateKey, err = x509.ParsePKCS1PrivateKey(decryptedPrivateKeyBytes)
	} else if password == "" { // has error. retry unencrypted
		privateKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	if err != nil {
		logger.Error("Failed to parse private key:", err)
		return "", err
	}
	decryptedByte, err := rsa.DecryptPKCS1v15(rand.Reader, privateKey, decodeCipher)
	if err != nil {
		logger.Error("Failed to decrypt data:", err)
		return "", err
	}
	return string(decryptedByte), err
}

func EncryptValue(value string, publicKeyData []byte) (string, error) {
	publicKeyBlock, _ := pem.Decode(publicKeyData)
	parsedPublicKey, err := x509.ParsePKIXPublicKey(publicKeyBlock.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse public key: %v", err)
	}
	publicKey, ok := parsedPublicKey.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("failed to assert parsed key as RSA public key")
	}

	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, publicKey, []byte(value))
	if err != nil {
		return "", fmt.Errorf("failed to encrypt value: %v", err)
	}
	return BASE64StdEncode(ciphertext), nil
}

func GenerateRsaKeyPair(password string) (privateByte, publicByte []byte, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		err = fmt.Errorf("failed to GenerateKey: %v", err)
		return
	}
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	var encryptedBlock *pem.Block
	if password != "" {
		encryptedBlock, err = x509.EncryptPEMBlock(rand.Reader, block.Type, block.Bytes, []byte(password), x509.PEMCipherAES256)
		if err != nil {
			err = fmt.Errorf("failed to EncryptPEMBlock: %v", err)
			return
		}
	} else {
		encryptedBlock = block
	}
	privateByte = pem.EncodeToMemory(encryptedBlock)

	publicKey := &privateKey.PublicKey
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		err = fmt.Errorf("failed to MarshalPKIXPublicKey: %v", err)
		return
	}
	block = &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	publicByte = pem.EncodeToMemory(block)

	return
}
