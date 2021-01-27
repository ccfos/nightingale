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

package funcs

import (
	"math"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/core"
)

func FsKernelMetrics() []*dataobj.MetricValue {
	maxFiles, err := nux.KernelMaxFiles()
	if err != nil {
		logger.Errorf("failed to call collect KernelMaxFiles:%v\n", err)
		return nil
	}

	allocateFiles, err := nux.KernelAllocateFiles()
	if err != nil {
		logger.Errorf("failed to call KernelAllocateFiles:%v\n", err)
		return nil
	}

	v := math.Ceil(float64(allocateFiles) * 100 / float64(maxFiles))
	return []*dataobj.MetricValue{
		core.GaugeValue("sys.fs.files.max", maxFiles),
		core.GaugeValue("sys.fs.files.free", maxFiles-allocateFiles),
		core.GaugeValue("sys.fs.files.used", allocateFiles),
		core.GaugeValue("sys.fs.files.used.percent", v),
	}
}

func ProcsNumMetrics() []*dataobj.MetricValue {
	var dirs []string
	num := 0
	dirs, err := file.DirsUnder("/proc")
	if err != nil {
		logger.Error("read /proc err:", err)
		return nil
	}

	size := len(dirs)
	if size == 0 {
		logger.Error("dirs is null")
		return nil
	}

	for i := 0; i < size; i++ {
		_, e := strconv.Atoi(dirs[i])
		if e != nil {
			continue
		}
		num += 1
	}

	return []*dataobj.MetricValue{
		core.GaugeValue("sys.ps.process.total", num),
	}
}

func EntityNumMetrics() []*dataobj.MetricValue {
	data, err := file.ToTrimString("/proc/loadavg")
	if err != nil {
		return nil
	}

	L := strings.Fields(data)
	if len(L) < 5 {
		logger.Errorf("get entity num err: %v", data)
		return nil
	}

	arr := strings.Split(L[3], "/")
	if len(arr) != 2 {
		logger.Errorf("get entity num err: %v", data)
		return nil
	}

	num, err := strconv.ParseFloat(arr[1], 64)
	if err != nil {
		logger.Errorf("get entity num err: %v", err)
		return nil
	}

	return []*dataobj.MetricValue{
		core.GaugeValue("sys.ps.entity.total", num),
	}
}
