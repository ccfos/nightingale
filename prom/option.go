package prom

import (
	"fmt"
	"sync"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/dsauth"
)

type PromOption struct {
	ClusterName   string
	Url           string
	WriteAddr     string
	BasicAuthUser string
	BasicAuthPass string

	Timeout     int64
	DialTimeout int64

	MaxIdleConnsPerHost int

	Headers []string

	// TLS 配置（支持 mTLS）
	TLS models.TLS

	// Cloud auth (sigv4 / oauth2 / azure)
	Auth dsauth.Config
}

// String keeps credentials out of logs: PromOption used to be printed with %+v,
// which wrote the basic auth password and header values verbatim.
func (po PromOption) String() string {
	return fmt.Sprintf("{ClusterName:%s Url:%s WriteAddr:%s BasicAuthUser:%s Timeout:%d DialTimeout:%d MaxIdleConnsPerHost:%d HeaderCount:%d AuthType:%s}",
		po.ClusterName, po.Url, po.WriteAddr, po.BasicAuthUser, po.Timeout, po.DialTimeout, po.MaxIdleConnsPerHost, len(po.Headers)/2, po.Auth.Type)
}

func (po *PromOption) Equal(target PromOption) bool {
	if po.Url != target.Url {
		return false
	}

	if po.BasicAuthUser != target.BasicAuthUser {
		return false
	}

	if po.BasicAuthPass != target.BasicAuthPass {
		return false
	}

	if po.WriteAddr != target.WriteAddr {
		return false
	}

	if po.Timeout != target.Timeout {
		return false
	}

	if po.DialTimeout != target.DialTimeout {
		return false
	}

	if po.MaxIdleConnsPerHost != target.MaxIdleConnsPerHost {
		return false
	}

	if len(po.Headers) != len(target.Headers) {
		return false
	}

	for i := 0; i < len(po.Headers); i++ {
		if po.Headers[i] != target.Headers[i] {
			return false
		}
	}

	// 比较 TLS 配置
	if po.TLS.SkipTlsVerify != target.TLS.SkipTlsVerify {
		return false
	}
	if po.TLS.CACert != target.TLS.CACert {
		return false
	}
	if po.TLS.ClientCert != target.TLS.ClientCert {
		return false
	}
	if po.TLS.ClientKey != target.TLS.ClientKey {
		return false
	}
	if po.TLS.ServerName != target.TLS.ServerName {
		return false
	}
	if po.TLS.MinVersion != target.TLS.MinVersion {
		return false
	}
	if po.TLS.MaxVersion != target.TLS.MaxVersion {
		return false
	}

	if !po.Auth.Equal(target.Auth) {
		return false
	}

	return true
}

type PromOptionsStruct struct {
	Data map[int64]PromOption
	sync.RWMutex
}

func (pos *PromOptionsStruct) Set(datasourceId int64, po PromOption) {
	pos.Lock()
	pos.Data[datasourceId] = po
	pos.Unlock()
}

func (pos *PromOptionsStruct) Del(datasourceId int64) {
	pos.Lock()
	delete(pos.Data, datasourceId)
	pos.Unlock()
}

func (pos *PromOptionsStruct) Get(datasourceId int64) (PromOption, bool) {
	pos.RLock()
	defer pos.RUnlock()
	ret, has := pos.Data[datasourceId]
	return ret, has
}

// Data key is cluster name
var PromOptions = &PromOptionsStruct{Data: make(map[int64]PromOption)}
