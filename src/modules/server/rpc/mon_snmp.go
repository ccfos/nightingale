package rpc

import (
	"encoding/json"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"
	"github.com/toolkits/pkg/logger"
)

func (*Server) SnmpCollectsGet(key string, resp *string) error {
	data := cache.SnmpCollectCache.GetBy(key)
	b, err := json.Marshal(data)
	if err != nil {
		logger.Warningf("get collect err:%v", err)
	}

	*resp = string(b)
	return nil
}

func (*Server) HWsGet(key string, resp *models.NetworkHardwareRpcResp) error {
	resp.Data = cache.SnmpHWCache.GetBy(key)
	return nil
}

func (*Server) HWsPut(hws []*models.NetworkHardware, resp *string) error {
	for i := 0; i < len(hws); i++ {
		hw, err := models.NetworkHardwareGet("id=?", hws[i].Id)
		if err != nil {
			logger.Warningf("get hw:%+v err:%v", hws[i], err)
			continue
		}

		if hw == nil {
			continue
		}

		hw.Name = hws[i].Name
		hw.SN = hws[i].SN
		hw.Uptime = hws[i].Uptime
		hw.Info = hws[i].Info

		err = hw.Update("name", "sn", "info", "uptime")
		if err != nil {
			logger.Warningf("get hw:%+v err:%v", hws[i], err)
			continue
		}
	}
	return nil
}
