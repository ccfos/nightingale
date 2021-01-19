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
	"errors"
	"io"
	"math"
	"os"
	"time"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/tsdb/index"
	"github.com/didi/nightingale/src/modules/tsdb/utils"

	"github.com/open-falcon/rrdlite"
	"github.com/toolkits/pkg/file"
)

func create(filename string, item *dataobj.TsdbItem) error {
	now := time.Now()
	start := now.Add(time.Duration(-24) * time.Hour)
	step := uint(item.Step)

	c := rrdlite.NewCreator(filename, start, step)
	c.DS("metric", item.DsType, item.Heartbeat, item.Min, item.Max)

	// 设置各种归档策略
	// 10s一个点存 12小时

	for archive, cnt := range Config.RRA {
		if archive == 1 {
			c.RRA("AVERAGE", 0, archive, cnt)
		} else {
			c.RRA("AVERAGE", 0, archive, cnt)
			c.RRA("MAX", 0, archive, cnt)
			c.RRA("MIN", 0, archive, cnt)
		}
	}

	return c.Create(true)
}

func update(filename string, items []*dataobj.TsdbItem) error {
	u := rrdlite.NewUpdater(filename)

	for _, item := range items {
		v := math.Abs(item.Value)
		if v > 1e+300 || (v < 1e-300 && v > 0) {
			continue
		}
		u.Cache(item.Timestamp, item.Value)
	}

	return u.Update()
}

// flush to disk from memory
// 最新的数据在列表的最后面
func Flushrrd(seriesID string, items []*dataobj.TsdbItem) error {
	item := index.GetItemFronIndex(seriesID)
	if items == nil || len(items) == 0 || item == nil {
		return errors.New("empty items")
	}

	filename := utils.RrdFileName(Config.Storage, seriesID, item.DsType, item.Step)
	if !file.IsExist(filename) {
		baseDir := file.Dir(filename)

		err := file.InsureDir(baseDir)
		if err != nil {
			return err
		}

		err = create(filename, item)
		if err != nil {
			return err
		}
	}

	return update(filename, items)
}

func fetch(filename string, cf string, start, end int64, step int) ([]*dataobj.RRDData, error) {
	start_t := time.Unix(start, 0)
	end_t := time.Unix(end, 0)
	step_t := time.Duration(step) * time.Second

	fetchRes, err := rrdlite.Fetch(filename, cf, start_t, end_t, step_t)
	if err != nil {
		return []*dataobj.RRDData{}, err
	}

	defer fetchRes.FreeValues()

	values := fetchRes.Values()
	size := len(values)
	ret := make([]*dataobj.RRDData, size)

	start_ts := fetchRes.Start.Unix()
	step_s := fetchRes.Step.Seconds()

	for i, val := range values {
		ts := start_ts + int64(i+1)*int64(step_s)
		d := &dataobj.RRDData{
			Timestamp: ts,
			Value:     dataobj.JsonFloat(val),
		}
		ret[i] = d
	}

	return ret, nil
}

// WriteFile writes data to a file named by filename.
// file must not exist
func writeFile(filename string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	n, err := f.Write(data)
	if err == nil && n < len(data) {
		err = io.ErrShortWrite
	}
	if err1 := f.Close(); err == nil {
		err = err1
	}
	return err
}
