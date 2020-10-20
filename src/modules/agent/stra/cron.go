package stra

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/net/httplib"

	"github.com/didi/nightingale/src/common/address"
	"github.com/didi/nightingale/src/models"
	"github.com/didi/nightingale/src/modules/agent/config"
)

func GetCollects() {
	if !config.Config.Stra.Enable {
		return
	}

	detect()
	go loopDetect()
}

func loopDetect() {
	t1 := time.NewTicker(time.Duration(config.Config.Stra.Interval) * time.Second)
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
	Dat models.Collect `json:"dat"`
	Err string         `json:"err"`
}

func GetCollectsRetry() (models.Collect, error) {
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

	url := fmt.Sprintf("http://%s%s%s", addr, config.Config.Stra.Api, config.Endpoint)
	err = httplib.Get(url).SetTimeout(time.Duration(config.Config.Stra.Timeout) * time.Millisecond).ToJSON(&res)
	if err != nil {
		err = fmt.Errorf("get collects from remote:%s failed, error:%v", url, err)
	}

	return res, err
}
