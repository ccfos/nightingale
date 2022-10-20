package secu

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"strings"
)

// BASE64StdEncode base64编码
func BASE64StdEncode(src []byte) string {
	return base64.StdEncoding.EncodeToString(src)
}

// BASE64StdDecode base64解码
func BASE64StdDecode(src string) ([]byte, error) {
	dst, err := base64.StdEncoding.DecodeString(src)
	if err != nil {
		return nil, err
	}
	return dst, nil
}

func PKCS7Padding(ciphertext []byte, blockSize int) []byte {
	padding := blockSize - len(ciphertext)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(ciphertext, padtext...)
}

func PKCS7UnPadding(originData []byte) []byte {
	length := len(originData)
	unpadding := int(originData[length-1])
	return originData[:(length - unpadding)]
}

//AES加密
func AesEncrypt(origData, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	//加密块填充
	blockSize := block.BlockSize()
	padOrigData := PKCS7Padding(origData, blockSize)
	//初始化CBC加密
	blockMode := cipher.NewCBCEncrypter(block, key[:blockSize])
	crypted := make([]byte, len(padOrigData))
	//加密
	blockMode.CryptBlocks(crypted, padOrigData)
	return crypted, nil
}

//AES解密
func AesDecrypt(crypted, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	blockSize := block.BlockSize()
	blockMode := cipher.NewCBCDecrypter(block, key[:blockSize])
	origData := make([]byte, len(crypted))
	//解密
	blockMode.CryptBlocks(origData, crypted)
	//去除填充
	origData = PKCS7UnPadding(origData)
	return origData, nil
}

// 针对配置文件属性进行解密处理
func DealWithDecrypt(src string, key string) (string, error) {
	//如果是{{cipher}}前缀，则代表是加密过的属性，先解密
	if strings.HasPrefix(src, "{{cipher}}") {
		data := src[10:]
		decodeData, err := BASE64StdDecode(data)
		if err != nil {
			return src, err
		}
		//解密
		origin, err := AesDecrypt(decodeData, []byte(key))
		if err != nil {
			return src, err
		}
		//返回明文
		return string(origin), nil
	} else {
		return src, nil
	}
}

// 针对配置文件属性进行加密处理
func DealWithEncrypt(src string, key string) (string, error) {
	encrypted, err := AesEncrypt([]byte(src), []byte(key))
	if err != nil {
		return src, err
	}

	data := BASE64StdEncode(encrypted)
	return "{{cipher}}" + data, nil
}
