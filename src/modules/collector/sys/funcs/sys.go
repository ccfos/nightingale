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
	var ret []*dataobj.MetricValue
	maxFiles, err := nux.KernelMaxFiles()
	if err != nil {
		logger.Error("failed collect kernel metrics:", err)
		return ret
	}

	allocateFiles, err := nux.KernelAllocateFiles()
	if err != nil {
		logger.Error("failed to call KernelAllocateFiles:", err)
		return ret
	}

	v := math.Ceil(float64(allocateFiles) * 100 / float64(maxFiles))
	ret = append(ret, GaugeValue("sys.fs.files.max", maxFiles))
	ret = append(ret, GaugeValue("sys.fs.files.free", maxFiles-allocateFiles))
	ret = append(ret, GaugeValue("sys.fs.files.used", allocateFiles))
	ret = append(ret, GaugeValue("sys.fs.files.used.percent", v))

	return ret
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
		GaugeValue("sys.ps.process.total", num),
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
		GaugeValue("sys.ps.entity.total", num),
	}
}
