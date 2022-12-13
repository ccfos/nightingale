package config

import "sync"

type PromOption struct {
	ClusterName   string
	Url           string
	BasicAuthUser string
	BasicAuthPass string

	Timeout     int64
	DialTimeout int64

	MaxIdleConnsPerHost int

	Headers []string
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

type PromOptionsStruct struct {
	Data map[string]PromOption
	sync.RWMutex
}

func (pos *PromOptionsStruct) Set(clusterName string, po PromOption) {
	pos.Lock()
	pos.Data[clusterName] = po
	pos.Unlock()
}

func (pos *PromOptionsStruct) Sets(clusterName string, po PromOption) {
	pos.Lock()
	pos.Data = map[string]PromOption{clusterName: po}
	pos.Unlock()
}

func (pos *PromOptionsStruct) Del(clusterName string) {
	pos.Lock()
	delete(pos.Data, clusterName)
	pos.Unlock()
}

func (pos *PromOptionsStruct) Get(clusterName string) (PromOption, bool) {
	pos.RLock()
	defer pos.RUnlock()
	ret, has := pos.Data[clusterName]
	return ret, has
}

// Data key is cluster name
var PromOptions = &PromOptionsStruct{Data: make(map[string]PromOption)}
