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
	"fmt"
	"strings"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"
	"github.com/toolkits/pkg/slice"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/core"
	"github.com/didi/nightingale/src/modules/agent/sys"
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
		ret = append(ret, core.GaugeValue("disk.bytes.total", du.BlocksAll, tags))
		ret = append(ret, core.GaugeValue("disk.bytes.free", du.BlocksFree, tags))
		ret = append(ret, core.GaugeValue("disk.bytes.used", du.BlocksUsed, tags))
		ret = append(ret, core.GaugeValue("disk.bytes.used.percent", du.BlocksUsedPercent, tags))

		if du.InodesAll == 0 {
			continue
		}

		ret = append(ret, core.GaugeValue("disk.inodes.total", du.InodesAll, tags))
		ret = append(ret, core.GaugeValue("disk.inodes.free", du.InodesFree, tags))
		ret = append(ret, core.GaugeValue("disk.inodes.used", du.InodesUsed, tags))
		ret = append(ret, core.GaugeValue("disk.inodes.used.percent", du.InodesUsedPercent, tags))
	}

	if len(ret) > 0 && diskTotal > 0 {
		ret = append(ret, core.GaugeValue("disk.cap.bytes.total", float64(diskTotal)))
		ret = append(ret, core.GaugeValue("disk.cap.bytes.used", float64(diskUsed)))
		ret = append(ret, core.GaugeValue("disk.cap.bytes.free", float64(diskTotal-diskUsed)))
		ret = append(ret, core.GaugeValue("disk.cap.bytes.used.percent", float64(diskUsed)*100.0/float64(diskTotal)))
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
