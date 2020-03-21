package stra

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"

	"github.com/didi/nightingale/src/model"
	"github.com/didi/nightingale/src/toolkits/address"
	"github.com/didi/nightingale/src/toolkits/identity"
)

func GetCollects() {
	if !StraConfig.Enable {
		return
	}

	detect()
	go loopDetect()
}

func loopDetect() {
	t1 := time.NewTicker(time.Duration(StraConfig.Interval) * time.Second)
	for {
		<-t1.C
		detect()
	}
}

func detect() {
	c, err := GetCollectsRetry()
	if err != nil {
		logger.Errorf("get collect err:%v", err)
		return
	}

	Collect.Update(&c)
}

type CollectResp struct {
	Dat model.Collect `json:"dat"`
	Err string        `json:"err"`
}

func GetCollectsRetry() (model.Collect, error) {
	count := len(address.GetHTTPAddresses("monapi"))
	var resp CollectResp
	var err error
	for i := 0; i < count; i++ {
		resp, err = getCollects()
		if err == nil {
			if resp.Err != "" {
				err = fmt.Errorf(resp.Err)
				continue
			}
			return resp.Dat, err
		}
	}

	return resp.Dat, err
}

func getCollects() (CollectResp, error) {
	addrs := address.GetHTTPAddresses("monapi")
	i := rand.Intn(len(addrs))
	addr := addrs[i]

	var res CollectResp
	var err error

	url := fmt.Sprintf("http://%s%s%s", addr, StraConfig.Api, identity.Identity)
	err = httplib.Get(url).SetTimeout(time.Duration(StraConfig.Timeout) * time.Millisecond).ToJSON(&res)
	if err != nil {
		err = fmt.Errorf("get collects from remote:%s failed, error:%v", url, err)
	}

	return res, err
}
