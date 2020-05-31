package funcs

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/didi/nightingale/src/dataobj"
	"github.com/didi/nightingale/src/modules/collector/sys"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"
	"github.com/toolkits/pkg/slice"
)

func FsRWMetrics() []*dataobj.MetricValue {
	ret := make([]*dataobj.MetricValue, 0)

	mountPoints, err := nux.ListMountPoint()
	if err != nil {
		logger.Errorf("failed to call ListMountPoint:%v\n", err)
		return ret
	}

	fsFileFilter := make(map[string]struct{}) //过滤 /proc/mounts 出现重复的fsFile

	for idx := range mountPoints {
		var du *nux.DeviceUsage
		du, err = nux.BuildDeviceUsage(mountPoints[idx][0], mountPoints[idx][1], mountPoints[idx][2])
		if err != nil {
			logger.Warning(idx, " failed to call BuildDeviceUsage:", err)
			continue
		}

		if hasIgnorePrefix(du.FsFile, sys.Config.MountIgnore.Prefix) &&
			!slice.ContainsString(sys.Config.MountIgnore.Exclude, du.FsFile) {
			continue
		}

		if _, exists := fsFileFilter[du.FsFile]; exists {
			logger.Debugf("mount point %s was collected", du.FsFile)
			continue
		} else {
			fsFileFilter[du.FsFile] = struct{}{}
		}

		tags := fmt.Sprintf("mount=%s", du.FsFile)

		f, err := os.Open(du.FsFile)
		if err != nil {
			logger.Error("target mount point open failed:", err)
			ret = append(ret, GaugeValue("disk.rw.error", 1, "硬盘分区读写是否有错",tags))
			continue
		}

		fs, err := f.Stat()
		if err != nil {
			logger.Error("get target mount point status failed:", err)
			ret = append(ret, GaugeValue("disk.rw.error", 2,"硬盘分区读写是否有错", tags))
			continue
		}

		if !fs.IsDir() {
			continue
		}

		file := filepath.Join(du.FsFile, ".fs-detect")
		now := time.Now().Format("2006-01-02 15:04:05")
		content := "FS-RW" + now
		err = CheckFS(file, content)
		if err != nil {
			ret = append(ret, GaugeValue("disk.rw.error", 3,"硬盘分区读写是否有错", tags))
		} else {
			ret = append(ret, GaugeValue("disk.rw.error", 0,"硬盘分区读写是否有错", tags))
		}
	}

	return ret
}

func CheckFS(file string, content string) error {
	//write test
	fd, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	defer fd.Close()
	if err != nil {
		logger.Error("Open file failed: ", err)
		return err
	}
	buf := []byte(content)
	count, err := fd.Write(buf)
	if err != nil || count != len(buf) {
		logger.Error("Write file failed: ", err)
		return err
	}
	//read test
	read, err := ioutil.ReadFile(file)
	if err != nil {
		logger.Error("Read file failed: ", err)
		return err
	}
	if string(read) != content {
		logger.Error("Read content failed: ", string(read))
		return errors.New("read content failed")
	}
	//clean the file
	err = os.Remove(file)
	if err != nil {
		logger.Error("Remove file filed: ", err)
		return err
	}
	return nil
}
