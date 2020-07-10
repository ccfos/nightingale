package rpc

import (
	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/backend"
	"github.com/toolkits/pkg/logger"
)

func (t *Transfer) Query(args []dataobj.QueryData, reply *dataobj.QueryDataResp) error {
	dataSource, err := backend.GetDataSourceFor("")
	if err != nil {
		logger.Warningf("Could not find dataSource ")
		return err
	}
	reply.Data = dataSource.QueryData(args)
	return nil
}
