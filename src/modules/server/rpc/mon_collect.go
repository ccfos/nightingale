package rpc

import (
	"encoding/json"

	"github.com/didi/nightingale/v4/src/models"
	"github.com/didi/nightingale/v4/src/modules/server/cache"

	"github.com/toolkits/pkg/logger"
)

func (*Server) GetCollectBy(endpoint string, resp *string) error {
	collect := cache.CollectCache.GetBy(endpoint)
	collectByte, _ := json.Marshal(collect)
	*resp = string(collectByte)

	logger.Debugf("agent %s get collect %+v %s", endpoint, collect, *resp)

	return nil
}

func (*Server) GetProberCollectBy(endpoint string, resp *models.CollectRuleRpcResp) error {
	resp.Data = cache.CollectRuleCache.GetBy(endpoint)
	return nil
}

func (*Server) GetApiCollectBy(key string, resp *models.ApiCollectRpcResp) error {
	resp.Data = cache.ApiCollectCache.GetBy(key)
	return nil
}
