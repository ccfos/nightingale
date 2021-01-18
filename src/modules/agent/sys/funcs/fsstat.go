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
	"errors"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/toolkits/pkg/logger"
	"github.com/toolkits/pkg/nux"
	"github.com/toolkits/pkg/slice"

	"github.com/didi/nightingale/src/common/dataobj"
	"github.com/didi/nightingale/src/modules/agent/core"
	"github.com/didi/nightingale/src/modules/agent/sys"
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
			ret = append(ret, core.GaugeValue("disk.rw.error", 1, tags))
			continue
		}

		fs, err := f.Stat()
		if err != nil {
			logger.Error("get target mount point status failed:", err)
			ret = append(ret, core.GaugeValue("disk.rw.error", 2, tags))
			continue
		}

		if !fs.IsDir() {
			continue
		}

		file := filepath.Join(du.FsFile, ".fs-detect."+genRandStr())
		now := time.Now().Format("2006-01-02 15:04:05")
		content := "FS-RW" + now
		err = CheckFS(file, content)
		if err != nil {
			ret = append(ret, core.GaugeValue("disk.rw.error", 3, tags))
		} else {
			ret = append(ret, core.GaugeValue("disk.rw.error", 0, tags))
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

func genRandStr() string {
	const len = 5
	var letters []byte = []byte("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

	randBytes := make([]byte, len)
	if _, err := rand.Read(randBytes); err != nil {
		return fmt.Sprintf("%d", rand.Int63())
	}

	for i := 0; i < len; i++ {
		pos := randBytes[i] % 62
		randBytes[i] = letters[pos]
	}

	return string(randBytes)
}
