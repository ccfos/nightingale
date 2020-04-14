package rpc

import (
	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/transfer/backend"
)

func (t *Transfer) Query(args []dataobj.QueryData, reply *dataobj.QueryDataResp) error {
	//start := time.Now()
	reply.Data = backend.FetchData(args)
	return nil
}
