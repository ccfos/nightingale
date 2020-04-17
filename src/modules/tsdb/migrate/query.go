package migrate

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/toolkits/pools"
	"github.com/didi/nightingale/src/toolkits/str"

	"github.com/toolkits/pkg/pool"
)

func FetchData(start, end int64, consolFun, endpoint, counter string, step int) ([]*dataobj.RRDData, error) {
	var err error
	if step <= 0 {
		step, err = getCounterStep(endpoint, counter)
		if err != nil {
			return nil, err
		}
	}

	qparm := GenQParam(start, end, consolFun, endpoint, counter, step)
	resp, err := QueryOne(qparm)
	if err != nil {
		return []*dataobj.RRDData{}, err
	}

	if len(resp.Values) < 1 {
		ts := start - start%int64(60)
		count := (end - start) / 60
		if count > 730 {
			count = 730
		}

		if count <= 0 {
			return []*dataobj.RRDData{}, nil
		}

		step := (end - start) / count // integer divide by zero
		for i := 0; i < int(count); i++ {
			resp.Values = append(resp.Values, &dataobj.RRDData{Timestamp: ts, Value: dataobj.JsonFloat(math.NaN())})
			ts += int64(step)
		}
	}

	return resp.Values, nil
}
func getCounterStep(endpoint, counter string) (step int, err error) {
	//从内存中获取
	return
}

func GenQParam(start, end int64, consolFunc, endpoint, counter string, step int) dataobj.TsdbQueryParam {
	return dataobj.TsdbQueryParam{
		Start:      start,
		End:        end,
		ConsolFunc: consolFunc,
		Endpoint:   endpoint,
		Counter:    counter,
		Step:       step,
	}
}

func QueryOne(para dataobj.TsdbQueryParam) (resp *dataobj.TsdbQueryResponse, err error) {
	start, end := para.Start, para.End
	resp = &dataobj.TsdbQueryResponse{}

	pk := str.PK(para.Endpoint, para.Counter)
	onePool, addr, err := selectPoolByPK(pk)
	if err != nil {
		return resp, err
	}

	conn, err := onePool.Fetch()
	if err != nil {
		return resp, err
	}

	rpcConn := conn.(pools.RpcClient)
	if rpcConn.Closed() {
		onePool.ForceClose(conn)
		return resp, errors.New("conn closed")
	}

	type ChResult struct {
		Err  error
		Resp *dataobj.TsdbQueryResponse
	}

	ch := make(chan *ChResult, 1)
	go func() {
		resp := &dataobj.TsdbQueryResponse{}
		err := rpcConn.Call("Tsdb.Query", para, resp)
		ch <- &ChResult{Err: err, Resp: resp}
	}()

	select {
	case <-time.After(time.Duration(Config.CallTimeout) * time.Millisecond):
		onePool.ForceClose(conn)
		return nil, fmt.Errorf("%s, call timeout. proc: %s", addr, onePool.Proc())
	case r := <-ch:
		if r.Err != nil {
			onePool.ForceClose(conn)
			return r.Resp, fmt.Errorf("%s, call failed, err %v. proc: %s", addr, r.Err, onePool.Proc())
		} else {
			onePool.Release(conn)
			if len(r.Resp.Values) < 1 {
				r.Resp.Values = []*dataobj.RRDData{}
				return r.Resp, nil
			}

			fixed := make([]*dataobj.RRDData, 0)
			for _, v := range r.Resp.Values {
				if v == nil || !(v.Timestamp >= start && v.Timestamp <= end) {
					continue
				}

				fixed = append(fixed, v)
			}
			r.Resp.Values = fixed
		}
		return r.Resp, nil
	}
}

func selectPoolByPK(pk string) (*pool.ConnPool, string, error) {
	node, err := TsdbNodeRing.GetNode(pk)
	if err != nil {
		return nil, "", err
	}

	addr, found := Config.OldCluster[node]
	if !found {
		return nil, "", errors.New("node not found")
	}

	onePool, found := TsdbConnPools.Get(addr)
	if !found {
		return nil, "", errors.New("addr not found")
	}

	return onePool, addr, nil
}
