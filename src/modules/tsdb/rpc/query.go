// Copyright 2017 Xiaomi, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rpc

import (
	"math"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/tsdb/cache"
	"github.com/didi/nightingale/src/modules/tsdb/index"
	"github.com/didi/nightingale/src/modules/tsdb/migrate"
	"github.com/didi/nightingale/src/modules/tsdb/rrdtool"
	"github.com/didi/nightingale/src/modules/tsdb/utils"
	"github.com/didi/nightingale/src/toolkits/stats"
	"github.com/didi/nightingale/src/toolkits/str"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

func (g *Tsdb) Query(param dataobj.TsdbQueryParam, resp *dataobj.TsdbQueryResponse) error {
	stats.Counter.Set("query.qp10s", 1)

	var (
		rrdDatas        []*dataobj.RRDData
		datasSize       int
		rrdFile         string
		cachePointsSize int
		err             error
	)

	// form empty response
	resp.Values = []*dataobj.RRDData{}
	resp.Endpoint = param.Endpoint
	resp.Counter = param.Counter
	resp.Nid = param.Nid
	if param.Nid != "" {
		param.Endpoint = dataobj.NidToEndpoint(param.Nid)
	}

	needStep := 0
	dsType := param.DsType

	step := param.Step
	seriesID := str.Checksum(param.Endpoint, param.Counter, "")

	if param.ConsolFunc == "" {
		param.ConsolFunc = "AVERAGE"
	}

	if dsType == "" || step == 0 {
		item := index.GetItemFronIndex(seriesID)
		if item == nil {
			dsType = "GAUGE"
			step = 10
		} else {
			dsType = item.DsType
			step = item.Step
		}
	}

	resp.DsType = dsType
	resp.Step = step

	startTs := param.Start - param.Start%int64(step)
	endTs := param.End - param.End%int64(step) + int64(step)
	if endTs-startTs-int64(step) < 1 {
		logger.Debug("time duration error", param)
		return nil
	}
	nowTs := time.Now().Unix()

	cachePoints := make([]*dataobj.RRDData, 0)
	cacheFirstTs := nowTs - nowTs%int64(step) - 3600 //存在cache的时间点
	if endTs > cacheFirstTs {                        //最后的时间点在cache范围内
		iters, err := cache.Caches.Get(seriesID, startTs, endTs)
		if err != nil {
			logger.Debug("get %v cache by %v err:%v", seriesID, param, err)
			stats.Counter.Set("query.miss", 1)
		}
		for _, iter := range iters {
			for iter.Next() {
				t, v := iter.Values()
				if int64(t) < startTs || int64(t) > endTs {
					//不在查询范围内
					continue
				}
				cachePoints = append(cachePoints, dataobj.NewRRDData(int64(t), v))
			}
		}
		//logger.Debugf("query %d cache count:%d detail:%v", seriesID, len(cachePoints), cachePoints)

		cachePointsSize = len(cachePoints)
		//查询起始时间在cache范围内，直接返回结果
		if cachePointsSize > 0 && param.Start >= cachePoints[0].Timestamp {
			resp.Values = cachePoints
			stats.Counter.Set("query.hit.cache", 1)
			goto _RETURN_OK
		}
	}

	rrdFile = utils.RrdFileName(rrdtool.Config.Storage, seriesID, dsType, step)
	if migrate.Config.Enabled && !file.IsExist(rrdFile) {
		rrdDatas, err = migrate.FetchData(startTs-int64(step), endTs, param.ConsolFunc, param.Endpoint, param.Counter, step)

		if !migrate.QueueCheck.Exists(seriesID) {
			node, err := migrate.TsdbNodeRing.GetNode(param.PK())
			if err != nil {
				logger.Error("E:", err)
			} else {
				filename := utils.QueryRrdFile(seriesID, dsType, step)
				Q := migrate.RRDFileQueues[node]
				body := dataobj.RRDFile{
					Key:      seriesID,
					Filename: filename,
				}
				Q.PushFront(body)
			}
		}
	} else {
		// read data from rrd file
		// 从RRD中获取数据不包含起始时间点
		// 例: startTs=1484651400,step=60,则第一个数据时间为1484651460)
		stats.Counter.Set("query.hit.file", 1)
		rrdDatas, err = rrdtool.Fetch(rrdFile, seriesID, param.ConsolFunc, startTs-int64(step), endTs, step)
		if err != nil {
			logger.Warningf("fetch rrd data err:%v seriesID:%v, param:%v", err, seriesID, param)
		}
		datasSize = len(rrdDatas)
		//logger.Debugf("query %d rrd items count:%d detail:%v ", seriesID, len(rrdDatas), rrdDatas)
	}

	if datasSize < 1 {
		resp.Values = cachePoints
		goto _RETURN_OK
	}

	if datasSize > 2 {
		step = int(rrdDatas[1].Timestamp - rrdDatas[0].Timestamp)
	}

	if endTs < cacheFirstTs {
		//请求结束时间不在cache时间范围内，直接返回磁盘数据

		resp.Values = rrdDatas
		goto _RETURN_OK
	}

	if cachePointsSize < 1 {
		//cache数据为空，直接返回磁盘数据
		resp.Values = rrdDatas
		goto _RETURN_OK
	}

	// merge
	{
		// fmt cached items
		var val dataobj.JsonFloat
		dataPoints := make([]*dataobj.RRDData, 0)

		ts := cachePoints[0].Timestamp
		cacheTs := ts

		//和磁盘中取出来的数据对齐时间戳
		if deta := ts % int64(step); deta != 0 {
			cacheTs = ts - deta + int64(step)
		}

		itemEndTs := cachePoints[cachePointsSize-1].Timestamp
		itemIdx := 0 //时间戳游标
		for cacheTs <= itemEndTs {
			vals := dataobj.JsonFloat(0.0)
			cnt := 0

			for ; itemIdx < cachePointsSize; itemIdx += 1 {
				// 依赖: cache的数据按照时间升序排列
				if cachePoints[itemIdx].Timestamp > cacheTs { //超过一个step范围，跳出去
					break
				}
				if isNumber(cachePoints[itemIdx].Value) {
					vals += dataobj.JsonFloat(cachePoints[itemIdx].Value)
					cnt += 1
				}
			}

			//cache内多个点合成一个点
			if cnt > 0 {
				val = vals / dataobj.JsonFloat(cnt)
			} else {
				val = dataobj.JsonFloat(math.NaN())
			}

			dataPoints = append(dataPoints, &dataobj.RRDData{Timestamp: cacheTs, Value: val})
			cacheTs += int64(step)
		}
		cacheSize := len(dataPoints)

		//将磁盘中的数据存到 merged
		merged := make([]*dataobj.RRDData, 0)
		if datasSize > 0 {
			for _, val := range rrdDatas {
				if val.Timestamp >= startTs && val.Timestamp <= endTs {
					// 依赖: rrdtool返回的数据,时间戳是连续的、不会有跳点的情况
					merged = append(merged, val)
				}
			}
		}

		if cacheSize > 0 {
			rrdDataSize := len(merged)
			lastTs := dataPoints[0].Timestamp

			// 找到merged中第一个时间戳比lastTs小的点
			rrdDataIdx := 0
			for rrdDataIdx = rrdDataSize - 1; rrdDataIdx >= 0; rrdDataIdx-- {
				if merged[rrdDataIdx].Timestamp < dataPoints[0].Timestamp {
					lastTs = merged[rrdDataIdx].Timestamp
					break
				}
			}

			// fix missing
			for ts := lastTs + int64(step); ts < dataPoints[0].Timestamp; ts += int64(step) {
				merged = append(merged, &dataobj.RRDData{Timestamp: ts, Value: dataobj.JsonFloat(math.NaN())})
			}

			// merge cached items to result
			rrdDataIdx += 1
			for cacheIdx := 0; cacheIdx < cacheSize; cacheIdx++ {
				// 从 rrdDataIdx 开始往后面追加数据
				if rrdDataIdx < rrdDataSize {
					if !math.IsNaN(float64(dataPoints[cacheIdx].Value)) {
						merged[rrdDataIdx] = dataPoints[cacheIdx] // 优先使用cache的数据
					}
				} else {
					merged = append(merged, dataPoints[cacheIdx])
				}

				rrdDataIdx++
			}
		}

		//logger.Debugf("query %d merged items count:%d detail:%v ", seriesID, len(merged), merged)

		mergedSize := len(merged)
		// fmt result
		retSize := int((endTs - startTs) / int64(step))
		retSize += 1
		ret := make([]*dataobj.RRDData, retSize, retSize)
		mergedIdx := 0
		ts = startTs - startTs%int64(step)
		for i := 0; i < retSize; i++ {
			if mergedIdx < mergedSize && ts == merged[mergedIdx].Timestamp {
				ret[i] = merged[mergedIdx]
				mergedIdx++
			} else {
				ret[i] = &dataobj.RRDData{Timestamp: ts, Value: dataobj.JsonFloat(math.NaN())}
			}
			ts += int64(step)
		}
		resp.Values = ret
	}

	//logger.Debugf("-->query data: %v <--data from cache %v <--data from disk %v <--merged data:%v", param, items, datas, resp.Values)

_RETURN_OK:
	rsize := len(resp.Values)
	realStep := 0

	if rsize > 2 {
		realStep = int(resp.Values[1].Timestamp - resp.Values[0].Timestamp)
	}
	if rsize > MaxRRAPointCnt || needStep != 0 {

		var sampleRate, sampleSize, sampleStep int
		if rsize > MaxRRAPointCnt {
			sampleRate = int(rsize/MaxRRAPointCnt) + 1
			sampleSize = int(rsize / sampleRate)
			sampleStep = sampleRate * realStep
			//logger.Debugf("rsize:%d sampleRate:%d sampleSize:%d sampleStep:%d", rsize, sampleRate, sampleSize, sampleStep)
		}

		// needStep 不为空则按照指定的step降采样，同环比会用到此功能
		if needStep != 0 && realStep != 0 {
			needStep = GetNeedStep(param.Start, param.Step, realStep) //统一环比1天，7天时间跨度，后期待优化

			sampleRate = int(needStep / realStep)
			if sampleRate == 0 {
				logger.Error("sampleRate is 0", param)
				sampleRate = 1
			}
			sampleSize = int(rsize / sampleRate)
			sampleStep = needStep
			//logger.Debugf("sampleRate:%d sampleSize:%d sampleStep:%d", sampleRate, sampleSize, sampleStep)
		}

		if sampleStep > 0 {
			// get offset
			offset := 0
			for i := 0; i < sampleRate && i < rsize; i++ {
				if resp.Values[i].Timestamp%int64(sampleStep) == 0 {
					offset = i
					break
				}
			}

			// set data
			sampled := make([]*dataobj.RRDData, 0)
			for i := 1; i < sampleSize; i++ {
				sv := &dataobj.RRDData{Timestamp: 0, Value: 0.0}
				cnt := 0
				jend := i*sampleRate + offset
				jstart := jend - sampleRate + 1

				if jend > rsize {
					break // 扔掉最后一个不完整的降采样周期
				}
				sv.Timestamp = resp.Values[jend].Timestamp
				for j := jstart; j <= jend && j < rsize; j++ {
					if j < 0 {
						continue
					}

					if !isNumber(resp.Values[j].Value) {
						continue
					}

					if !(startTs <= resp.Values[j].Timestamp &&
						endTs >= resp.Values[j].Timestamp) {
						// 时间范围不合法
						continue
					}

					sv.Value = sv.Value + dataobj.JsonFloat(resp.Values[j].Value)
					cnt += 1
				}

				if cnt == 0 {
					sv.Value = dataobj.JsonFloat(math.NaN())
				} else {
					sv.Value = sv.Value / dataobj.JsonFloat(cnt)
				}
				if sv.Timestamp >= param.Start && sv.Timestamp <= param.End {
					sampled = append(sampled, sv)
				}
			}

			resp.Step = sampleStep
			resp.Values = sampled
		} else if sampleStep <= 0 {
			logger.Errorf("zero step, %v", resp)
		}
	} else {
		tmpList := make([]*dataobj.RRDData, 0)
		//cache需要补null
		for _, dat := range resp.Values {
			if dat.Timestamp >= param.Start && dat.Timestamp <= param.End {
				tmpList = append(tmpList, &dataobj.RRDData{Timestamp: dat.Timestamp, Value: dat.Value})
			}
		}
		resp.Values = tmpList
	}

	// statistics
	return nil
}

func (g *Tsdb) GetRRD(param dataobj.RRDFileQuery, resp *dataobj.RRDFileResp) (err error) {
	go func() { //异步更新flag
		for _, f := range param.Files {
			err := cache.Caches.SetFlag(str.GetKey(f.Filename), rrdtool.ITEM_TO_SEND)
			if err != nil {
				logger.Errorf("key:%v file:%s set flag error:%v", f.Key, f.Filename, err)
			}
		}
	}()

	workerNum := 100
	worker := make(chan struct{}, workerNum) //控制goroutine并发数
	dataChan := make(chan *dataobj.File, 1000)

	for _, f := range param.Files {
		worker <- struct{}{}
		go getRRD(f, worker, dataChan)
	}

	//等待所有goroutine执行完成
	for i := 0; i < workerNum; i++ {
		worker <- struct{}{}
	}

	close(dataChan)
	for {
		d, ok := <-dataChan
		if !ok {
			break
		}
		resp.Files = append(resp.Files, *d)
	}
	return
}

func getRRD(f dataobj.RRDFile, worker chan struct{}, dataChan chan *dataobj.File) {
	defer func() {
		<-worker
	}()

	filePath := rrdtool.Config.Storage + "/" + f.Filename
	//将内存中的数据落盘
	key := str.GetKey(f.Filename)
	if c, exists := cache.Caches.GetCurrentChunk(key); exists {
		cache.ChunksSlots.Push(key, c)
	}

	chunks, exists := cache.ChunksSlots.GetChunks(key)
	if exists {
		m := make(map[string][]*cache.Chunk)
		m[key] = chunks
		rrdtool.FlushRRD(m)
	}

	body, err := rrdtool.ReadFile(filePath, filePath)
	if err != nil {
		logger.Error(err)
		return
	}
	tmp := dataobj.File{
		Key:      key,
		Filename: f.Filename,
		Body:     body,
	}
	dataChan <- &tmp
	return
}
