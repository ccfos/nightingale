package plugins

import (
	"github.com/didi/nightingale/src/toolkits/i18n"
	"github.com/influxdata/telegraf/plugins/common/tls"
)

func init() {
	i18n.DictRegister(langDict)
}

var (
	langDict = map[string]map[string]string{
		"zh": map[string]string{
			"disables SSL certificate verification":                           "禁用SSL证书验证",
			"verify certificates of TLS enabled servers using this CA bundle": "使用此CA文件验证服务器的证书",
			"identify TLS client using this SSL certificate file":             "使用此SSL证书文件标识TLS客户端",
			"identify TLS client using this SSL key file":                     "使用此SSL密钥文件标识TLS客户端",
		},
	}
)

type ClientConfig struct {
	InsecureSkipVerify bool   `label:"Insecure Skip" json:"insecure_skip_verify" default:"false" description:"disables SSL certificate verification"`
	TLSCA              string `label:"CA" json:"tls_ca" format:"file" description:"verify certificates of TLS enabled servers using this CA bundle"`
	TLSCert            string `label:"Cert" json:"tls_cert" format:"file" description:"identify TLS client using this SSL certificate file"`
	TLSKey             string `label:"Key" json:"tls_key" format:"file" description:"identify TLS client using this SSL key file"`
}

func (config ClientConfig) TlsClientConfig() tls.ClientConfig {
	return tls.ClientConfig{
		InsecureSkipVerify: config.InsecureSkipVerify,
		TLSCA:              config.TLSCA,
		TLSCert:            config.TLSCert,
		TLSKey:             config.TLSKey,
	}
}
