package tdengine

import (
	"sync"

	"github.com/ccfos/nightingale/v6/pkg/tlsx"
)

type TdengineOption struct {
	DatasourceName string
	Url            string
	BasicAuthUser  string
	BasicAuthPass  string
	Token          string

	Timeout     int64
	DialTimeout int64

	MaxIdleConnsPerHost int

	Headers []string

	tlsx.ClientConfig
}

func (po *TdengineOption) Equal(target TdengineOption) bool {
	if po.Url != target.Url {
		return false
	}

	if po.BasicAuthUser != target.BasicAuthUser {
		return false
	}

	if po.BasicAuthPass != target.BasicAuthPass {
		return false
	}

	if po.Token != target.Token {
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

	return true
}

type TdengineOptionsStruct struct {
	Data map[int64]TdengineOption
	sync.RWMutex
}

func (pos *TdengineOptionsStruct) Set(datasourceId int64, po TdengineOption) {
	pos.Lock()
	pos.Data[datasourceId] = po
	pos.Unlock()
}

func (pos *TdengineOptionsStruct) Del(datasourceId int64) {
	pos.Lock()
	delete(pos.Data, datasourceId)
	pos.Unlock()
}

func (pos *TdengineOptionsStruct) Get(datasourceId int64) (TdengineOption, bool) {
	pos.RLock()
	defer pos.RUnlock()
	ret, has := pos.Data[datasourceId]
	return ret, has
}

// Data key is cluster name
var TdengineOptions = &TdengineOptionsStruct{Data: make(map[int64]TdengineOption)}
