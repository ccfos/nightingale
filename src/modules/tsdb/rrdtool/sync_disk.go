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

package rrdtool

import (
	"io/ioutil"
	"sync"
	"sync/atomic"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/tsdb/cache"
	"github.com/didi/nightingale/src/modules/tsdb/index"
	"github.com/didi/nightingale/src/modules/tsdb/utils"
	"github.com/didi/nightingale/src/toolkits/stats"

	"github.com/toolkits/pkg/concurrent/semaphore"
	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
)

var bufferPool = sync.Pool{New: func() interface{} { return new(dataobj.TsdbItem) }}

var (
	disk_counter uint64
	net_counter  uint64
)

const (
	ITEM_TO_SEND    = 1
	ITEM_TO_PULLRRD = 2
)

const (
	_ = iota
	IO_TASK_M_READ
	IO_TASK_M_WRITE
	IO_TASK_M_FLUSH
	IO_TASK_M_FETCH
)

type File struct {
	Filename string
	Body     []byte
}

type fetch_t struct {
	filename string
	cf       string
	start    int64
	end      int64
	step     int
	data     []*dataobj.RRDData
}

type flushfile_t struct {
	seriesID string
	items    []*dataobj.TsdbItem
}

type readfile_t struct {
	filename string
	data     []byte
}

type io_task_t struct {
	method int
	args   interface{}
	done   chan error
}

var (
	Out_done_chan    chan int
	io_task_chans    []chan *io_task_t
	flushrrd_timeout int32

	Config RRDSection
)

type RRDSection struct {
	Enabled     bool        `yaml:"enabled"`
	Migrate     bool        `yaml:"enabled"`
	Storage     string      `yaml:"storage"`
	Batch       int         `yaml:"batch"`
	Concurrency int         `yaml:"concurrency"`
	Wait        int         `yaml:"wait"`
	RRA         map[int]int `yaml:"rra"`
	IOWorkerNum int         `yaml:"ioWorkerNum"`
}

func Init(cfg RRDSection) {
	Config = cfg
	InitChannel()
	Start()

	go FlushFinishd2Disk()
}

func InitChannel() { //初始化io池
	Out_done_chan = make(chan int, 1)
	ioWorkerNum := Config.IOWorkerNum
	io_task_chans = make([]chan *io_task_t, ioWorkerNum)
	for i := 0; i < ioWorkerNum; i++ {
		io_task_chans[i] = make(chan *io_task_t, 16)
	}
}

func Start() {
	var err error
	// check data dir
	if err = file.EnsureDirRW(Config.Storage); err != nil {
		logger.Fatal("rrdtool.Start error, bad data dir "+Config.Storage+",", err)
	}

	// sync disk
	go ioWorker()
	logger.Info("rrdtool.Start ok")
}

func ioWorker() {
	ioWorkerNum := Config.IOWorkerNum
	for i := 0; i < ioWorkerNum; i++ {
		go func(i int) {
			var err error
			for {
				select {
				case task := <-io_task_chans[i]:
					if task.method == IO_TASK_M_READ {
						if args, ok := task.args.(*readfile_t); ok {
							args.data, err = ioutil.ReadFile(args.filename)
							task.done <- err
						}
					} else if task.method == IO_TASK_M_WRITE {
						//filename must not exist
						if args, ok := task.args.(*File); ok {
							baseDir := file.Dir(args.Filename)
							if err = file.InsureDir(baseDir); err != nil {
								task.done <- err
							}
							task.done <- writeFile(args.Filename, args.Body, 0644)
						}
					} else if task.method == IO_TASK_M_FLUSH {
						if args, ok := task.args.(*flushfile_t); ok {
							task.done <- Flushrrd(args.seriesID, args.items)
						}
					} else if task.method == IO_TASK_M_FETCH {
						if args, ok := task.args.(*fetch_t); ok {
							args.data, err = fetch(args.filename, args.cf, args.start, args.end, args.step)
							task.done <- err
						}
					}
				}
			}
		}(i)
	}
}

func FlushFinishd2Disk() {
	var idx int = 0
	//time.Sleep(time.Second * time.Duration(cache.Config.SpanInSeconds))
	ticker := time.NewTicker(time.Millisecond * time.Duration(cache.Config.FlushDiskStepMs)).C
	slotNum := cache.Config.SpanInSeconds * 1000 / cache.Config.FlushDiskStepMs
	for {
		select {
		case <-ticker:
			idx = idx % slotNum
			chunks := cache.ChunksSlots.Get(idx)
			flushChunks := make(map[string][]*cache.Chunk, 0)
			for key, cs := range chunks {
				if Config.Migrate {
					item := index.GetItemFronIndex(key)
					rrdFile := utils.RrdFileName(Config.Storage, key, item.DsType, item.Step)
					//在扩容期间，当新实例内存中的曲线对应的rrd文件还没有从旧实例获取并落盘时，先在内存中继续保持
					if !file.IsExist(rrdFile) && cache.Caches.GetFlag(key) == ITEM_TO_PULLRRD {
						cache.ChunksSlots.PushChunks(key, cs)
						continue
					}
				}
				flushChunks[key] = cs
			}
			FlushRRD(flushChunks)
			idx += 1
		case <-cache.FlushDoneChan:
			logger.Info("FlushFinishd2Disk recv sigout and exit...")
			return
		}
	}
}

func Persist() {
	logger.Info("start Persist")

	for _, shard := range cache.Caches {
		if len(shard.Items) == 0 {
			continue
		}
		for id, chunks := range shard.Items {
			cache.ChunksSlots.Push(id, chunks.GetChunk(chunks.CurrentChunkPos))
		}
	}

	for i := 0; i < cache.ChunksSlots.Size; i++ {
		FlushRRD(cache.ChunksSlots.Get(i))
	}

	return
}

func FlushRRD(flushChunks map[string][]*cache.Chunk) {
	sema := semaphore.NewSemaphore(Config.Concurrency)
	var wg sync.WaitGroup
	for key, chunks := range flushChunks {
		//控制并发
		sema.Acquire()
		wg.Add(1)
		go func(seriesID string, chunks []*cache.Chunk) {
			defer sema.Release()
			defer wg.Done()
			for _, c := range chunks {
				iter := c.Iter()
				items := []*dataobj.TsdbItem{}
				for iter.Next() {
					t, v := iter.Values()
					d := bufferPool.Get().(*dataobj.TsdbItem)
					d = &dataobj.TsdbItem{
						Timestamp: int64(t),
						Value:     v,
					}
					items = append(items, d)
					bufferPool.Put(d)
				}

				err := FlushFile(seriesID, items)
				if err != nil {
					stats.Counter.Set("flush.rrd.err", 1)
					logger.Errorf("flush %v data to rrd err:%v", seriesID, err)
					continue
				}
			}
		}(key, chunks)
	}
	wg.Wait()
}

//todo items数据结构优化
func Commit(seriesID string, items []*dataobj.TsdbItem) {
	FlushFile(seriesID, items)
}

func FlushFile(seriesID string, items []*dataobj.TsdbItem) error {
	done := make(chan error, 1)
	index, err := getIndex(seriesID)
	if err != nil {
		return err
	}
	io_task_chans[index] <- &io_task_t{
		method: IO_TASK_M_FLUSH,
		args: &flushfile_t{
			seriesID: seriesID,
			items:    items,
		},
		done: done,
	}
	stats.Counter.Set("series.write", 1)
	atomic.AddUint64(&disk_counter, 1)
	return <-done
}

func Fetch(filename string, seriesID string, cf string, start, end int64, step int) ([]*dataobj.RRDData, error) {
	done := make(chan error, 1)
	task := &io_task_t{
		method: IO_TASK_M_FETCH,
		args: &fetch_t{
			filename: filename,
			cf:       cf,
			start:    start,
			end:      end,
			step:     step,
		},
		done: done,
	}
	index, err := getIndex(seriesID)
	if err != nil {
		return nil, err
	}

	io_task_chans[index] <- task
	err = <-done
	return task.args.(*fetch_t).data, err
}

func getIndex(seriesID string) (index int, err error) {
	batchNum := Config.IOWorkerNum

	if batchNum <= 1 {
		return 0, nil
	}

	return int(utils.HashKey(seriesID) % uint32(batchNum)), nil
}

func ReadFile(filename string, seriesID string) ([]byte, error) {
	done := make(chan error, 1)
	task := &io_task_t{
		method: IO_TASK_M_READ,
		args:   &readfile_t{filename: filename},
		done:   done,
	}

	index, err := getIndex(seriesID)
	if err != nil {
		return nil, err
	}

	io_task_chans[index] <- task
	err = <-done
	return task.args.(*readfile_t).data, err
}
