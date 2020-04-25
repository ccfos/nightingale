package funcs

import (
	"fmt"
	"strings"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/sys"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"
)

func DeviceMetrics() []*dataobj.MetricValue {
	var ret []*dataobj.MetricValue

	mountPoints, err := nux.ListMountPoint()
	fsFileFilter := make(map[string]struct{}) //过滤 /proc/mounts 出现重复的fsFile
	if err != nil {
		logger.Error("collect device metrics fail:", err)
		return ret
	}

	var myMountPoints = make(map[string]bool)
	if len(sys.Config.MountPoint) > 0 {
		for _, mp := range sys.Config.MountPoint {
			myMountPoints[mp] = true
		}
	}

	ignoreMountPointsPrefix := sys.Config.MountIgnorePrefix

	var diskTotal uint64 = 0
	var diskUsed uint64 = 0

	for idx := range mountPoints {
		fsSpec, fsFile, fsVfstype := mountPoints[idx][0], mountPoints[idx][1], mountPoints[idx][2]
		if len(myMountPoints) > 0 {
			if _, ok := myMountPoints[fsFile]; !ok {
				logger.Debug("mount point not matched with config", fsFile, "ignored.")
				continue
			}
		}

		if _, exists := fsFileFilter[fsFile]; exists {
			logger.Debugf("mount point %s was collected", fsFile)
			continue
		} else {
			fsFileFilter[fsFile] = struct{}{}
		}

		if hasIgnorePrefix(fsFile, ignoreMountPointsPrefix) {
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
		ret = append(ret, GaugeValue("disk.bytes.total", du.BlocksAll, tags))
		ret = append(ret, GaugeValue("disk.bytes.free", du.BlocksFree, tags))
		ret = append(ret, GaugeValue("disk.bytes.used", du.BlocksUsed, tags))
		ret = append(ret, GaugeValue("disk.bytes.used.percent", du.BlocksUsedPercent, tags))

		if du.InodesAll == 0 {
			continue
		}

		ret = append(ret, GaugeValue("disk.inodes.total", du.InodesAll, tags))
		ret = append(ret, GaugeValue("disk.inodes.free", du.InodesFree, tags))
		ret = append(ret, GaugeValue("disk.inodes.used", du.InodesUsed, tags))
		ret = append(ret, GaugeValue("disk.inodes.used.percent", du.InodesUsedPercent, tags))
	}

	if len(ret) > 0 && diskTotal > 0 {
		ret = append(ret, GaugeValue("disk.cap.bytes.total", float64(diskTotal)))
		ret = append(ret, GaugeValue("disk.cap.bytes.used", float64(diskUsed)))
		ret = append(ret, GaugeValue("disk.cap.bytes.free", float64(diskTotal-diskUsed)))
		ret = append(ret, GaugeValue("disk.cap.bytes.used.percent", float64(diskUsed)*100.0/float64(diskTotal)))
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
