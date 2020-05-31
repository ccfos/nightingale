package funcs

import (
	"math"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"

	"github.com/didi/nightingale/src/dataobj"
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
		GaugeValue("sys.fs.files.max", maxFiles,"允许使用的最大句柄量"),
		GaugeValue("sys.fs.files.free", maxFiles-allocateFiles,"文件句柄剩余量"),
		GaugeValue("sys.fs.files.used", allocateFiles,"文件句柄使用量"),
		GaugeValue("sys.fs.files.used.percent", v,"文件句柄使用占比"),
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
		GaugeValue("sys.ps.process.total", num,"系统进程数"),
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
		GaugeValue("sys.ps.entity.total", num,"系统调度单元总数"),
	}
}
