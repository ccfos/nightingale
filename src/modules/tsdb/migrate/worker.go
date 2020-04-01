package migrate

import (
	"strings"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/tsdb/cache"
	"github.com/didi/nightingale/src/modules/tsdb/rrdtool"
	"github.com/didi/nightingale/src/modules/tsdb/utils"
	"github.com/didi/nightingale/src/toolkits/stats"
	"github.com/didi/nightingale/src/toolkits/str"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

// send
const (
	DefaultSendTaskSleepInterval = time.Millisecond * 50 //默认睡眠间隔为50ms
)

func StartMigrate() {
	for node, addr := range Config.OldCluster {
		go pullRRD(node, addr, Config.Concurrency)
	}

	for node, addr := range Config.NewCluster {
		go send2NewTsdbTask(node, addr, Config.Concurrency)
	}

	for node, addr := range Config.OldCluster {
		go send2OldTsdbTask(node, addr, Config.Concurrency)
	}
}

func pullRRD(node string, addr string, concurrent int) {
	batch := Config.Batch // 一次发送,最多batch条数据
	Q := RRDFileQueues[node]

	sema := semaphore.NewSemaphore(concurrent)
	for {
		fnames := Q.PopBackBy(batch)
		count := len(fnames)

		if count == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}
		stats.Counter.Set("pull.rrd", count)

		filenames := make([]dataobj.RRDFile, count)
		for i := 0; i < count; i++ {
			filenames[i] = fnames[i].(dataobj.RRDFile)
			cache.Caches.SetFlag(str.GetKey(filenames[i].Filename), rrdtool.ITEM_TO_PULLRRD)
		}

		//控制并发
		sema.Acquire()
		go func(addr string, filenames []dataobj.RRDFile, count int) {
			defer sema.Release()

			req := dataobj.RRDFileQuery{Files: filenames}
			resp := &dataobj.RRDFileResp{}
			var err error
			sendOk := false
			for i := 0; i < 3; i++ { //最多重试3次
				err = TsdbConnPools.Call(addr, "Tsdb.GetRRD", req, resp)
				if err == nil {
					sendOk = true
					break
				}
				time.Sleep(time.Millisecond * 10)
			}
			for _, f := range resp.Files {
				filePath := rrdtool.Config.Storage + "/" + f.Filename

				paths := strings.Split(f.Filename, "/")
				if len(paths) != 2 {
					logger.Errorf("write rrd file err %v filename:%s", err, f.Filename)
					stats.Counter.Set("pull.rrd.err", count)
					continue
				}
				file.EnsureDir(rrdtool.Config.Storage + "/" + paths[0])
				err = utils.WriteFile(filePath, f.Body, 0644)
				if err != nil {
					stats.Counter.Set("pull.rrd.err", count)
					logger.Errorf("write rrd file err %v filename:%s", err, f.Filename)
				}

				cache.Caches.SetFlag(str.GetKey(f.Filename), 0) //重置曲线标志位
			}

			// statistics
			if !sendOk {
				logger.Errorf("get %v from old tsdb %s:%s fail: %v", filenames, node, addr, err)
			} else {
				logger.Infof("get %v from old tsdb %s:%s ok", filenames, node, addr)
			}
		}(addr, filenames, count)
	}
}

func send2OldTsdbTask(node string, addr string, concurrent int) {
	batch := Config.Batch // 一次发送,最多batch条数据
	Q := TsdbQueues[node]

	sema := semaphore.NewSemaphore(concurrent)

	for {
		items := Q.PopBackBy(batch)
		count := len(items)

		if count == 0 {
			time.Sleep(DefaultSendTaskSleepInterval)
			continue
		}

		tsdbItems := make([]*dataobj.TsdbItem, count)
		for i := 0; i < count; i++ {
			tsdbItems[i] = items[i].(*dataobj.TsdbItem)
			tsdbItems[i].From = dataobj.GRAPH
			stats.Counter.Set("migrate.old.out", 1)

			logger.Debug("send to old tsdb->: ", tsdbItems[i])
		}

		//控制并发
		sema.Acquire()
		go func(addr string, tsdbItems []*dataobj.TsdbItem, count int) {
			defer sema.Release()

			resp := &dataobj.SimpleRpcResponse{}
			var err error
			sendOk := false
			for i := 0; i < 3; i++ { //最多重试3次
				err = TsdbConnPools.Call(addr, "Tsdb.Send", tsdbItems, resp)
				if err == nil {
					sendOk = true
					break
				}
				time.Sleep(time.Millisecond * 10)
			}

			// statistics
			//atomic.AddInt64(&PointOut2Tsdb, int64(count))
			if !sendOk {
				logger.Errorf("send %v to tsdb %s:%s fail: %v", tsdbItems, node, addr, err)
			} else {
				logger.Infof("send to tsdb %s:%s ok", node, addr)
			}
		}(addr, tsdbItems, count)
	}
}

func send2NewTsdbTask(node string, addr string, concurrent int) {
	batch := Config.Batch // 一次发送,最多batch条数据
	Q := NewTsdbQueues[node]

	sema := semaphore.NewSemaphore(concurrent)

	for {
		items := Q.PopBackBy(batch)
		count := len(items)

		if count == 0 {
			time.Sleep(DefaultSendTaskSleepInterval)
			continue
		}

		tsdbItems := make([]*dataobj.TsdbItem, count)
		for i := 0; i < count; i++ {
			tsdbItems[i] = items[i].(*dataobj.TsdbItem)
			tsdbItems[i].From = dataobj.GRAPH
			stats.Counter.Set("migrate.new.out", 1)
			logger.Debug("send to new tsdb->: ", tsdbItems[i])
		}

		//控制并发
		sema.Acquire()
		go func(addr string, tsdbItems []*dataobj.TsdbItem, count int) {
			defer sema.Release()

			resp := &dataobj.SimpleRpcResponse{}
			var err error
			sendOk := false
			for i := 0; i < 3; i++ { //最多重试3次
				err = NewTsdbConnPools.Call(addr, "Tsdb.Send", tsdbItems, resp)
				if err == nil {
					sendOk = true
					break
				}
				time.Sleep(time.Millisecond * 10)
			}

			// statistics
			//atomic.AddInt64(&PointOut2Tsdb, int64(count))
			if !sendOk {
				logger.Errorf("send %v to tsdb %s:%s fail: %v", tsdbItems, node, addr, err)
			} else {
				logger.Infof("send to tsdb %s:%s ok", node, addr)
			}
		}(addr, tsdbItems, count)
	}
}
