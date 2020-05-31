package funcs

import (
	"fmt"
	"strings"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/sys"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"
	"github.com/toolkits/pkg/slice"
)

func DeviceMetrics() []*dataobj.MetricValue {
	ret := make([]*dataobj.MetricValue, 0)

	mountPoints, err := nux.ListMountPoint()
	fsFileFilter := make(map[string]struct{}) //过滤 /proc/mounts 出现重复的fsFile
	if err != nil {
		logger.Error("collect device metrics fail:", err)
		return ret
	}

	var diskTotal uint64 = 0
	var diskUsed uint64 = 0

	for idx := range mountPoints {
		fsSpec, fsFile, fsVfstype := mountPoints[idx][0], mountPoints[idx][1], mountPoints[idx][2]

		if _, exists := fsFileFilter[fsFile]; exists {
			logger.Debugf("mount point %s was collected", fsFile)
			continue
		} else {
			fsFileFilter[fsFile] = struct{}{}
		}

		// 注意: 虽然前缀被忽略了，但是被忽略的这部分分区里边有些仍然是需要采集的
		if hasIgnorePrefix(fsFile, sys.Config.MountIgnore.Prefix) &&
			!slice.ContainsString(sys.Config.MountIgnore.Exclude, fsFile) {
			continue
		}

		var du *nux.DeviceUsage
		du, err = nux.BuildDeviceUsage(fsSpec, fsFile, fsVfstype)
		if err != nil {
			logger.Errorf("fsSpec: %s, fsFile: %s, fsVfstype: %s, error: %v", fsSpec, fsFile, fsVfstype, err)
			continue
		}

		if du.BlocksAll == 0 {
			continue
		}

		diskTotal += du.BlocksAll
		diskUsed += du.BlocksUsed

		tags := fmt.Sprintf("mount=%s", du.FsFile)
		ret = append(ret, GaugeValue("disk.bytes.total", du.BlocksAll, "某分区大小",tags))
		ret = append(ret, GaugeValue("disk.bytes.free", du.BlocksFree,"某分区余量大小", tags))
		ret = append(ret, GaugeValue("disk.bytes.used", du.BlocksUsed,"某分区用量大小", tags))
		ret = append(ret, GaugeValue("disk.bytes.used.percent", du.BlocksUsedPercent,"某分区用量占比", tags))

		if du.InodesAll == 0 {
			continue
		}

		ret = append(ret, GaugeValue("disk.inodes.total", du.InodesAll,"某分区inode总数量", tags))
		ret = append(ret, GaugeValue("disk.inodes.free", du.InodesFree,"某分区空闲inode数量", tags))
		ret = append(ret, GaugeValue("disk.inodes.used", du.InodesUsed,"某分区已用inode数量", tags))
		ret = append(ret, GaugeValue("disk.inodes.used.percent", du.InodesUsedPercent,"某分区已用inode占比", tags))
	}

	if len(ret) > 0 && diskTotal > 0 {
		ret = append(ret, GaugeValue("disk.cap.bytes.total", float64(diskTotal),"所有分区大小之和"))
		ret = append(ret, GaugeValue("disk.cap.bytes.used", float64(diskUsed),"所有分区空闲大小之和"))
		ret = append(ret, GaugeValue("disk.cap.bytes.free", float64(diskTotal-diskUsed),"所有分区用量大小之和"))
		ret = append(ret, GaugeValue("disk.cap.bytes.used.percent", float64(diskUsed)*100.0/float64(diskTotal),"所有分区总用量占比"))
	}

	return ret
}

func hasIgnorePrefix(fsFile string, ignoreMountPointsPrefix []string) bool {
	hasPrefix := false
	if len(ignoreMountPointsPrefix) > 0 {
		for _, ignorePrefix := range ignoreMountPointsPrefix {
			if strings.HasPrefix(fsFile, ignorePrefix) {
				hasPrefix = true
				logger.Debugf("mount point %s has ignored prefix %s", fsFile, ignorePrefix)
				break
			}
		}
	}
	return hasPrefix
}
