package utils

import (
	"io"
	"os"
	"strconv"
)

// RRDTOOL UTILS
// 监控数据对应的rrd文件名称

const RRDDIRS uint64 = 1000

func QueryRrdFile(seriesID string, dsType string, step int) string {
	return seriesID[0:2] + "/" + seriesID + "_" + dsType + "_" + strconv.Itoa(step) + ".rrd"
}

func RrdFileName(baseDir string, seriesID string, dsType string, step int) string {
	return baseDir + "/" + seriesID[0:2] + "/" + seriesID + "_" + dsType + "_" + strconv.Itoa(step) + ".rrd"
}

// WriteFile writes data to a file named by filename.
// file must not exist
func WriteFile(filename string, data []byte, perm os.FileMode) error {
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

func HashKey(key string) uint32 {
	hash := uint32(2166136261)
	const prime32 = uint32(16777619)
	for i := 0; i < len(key); i++ {
		hash *= prime32
		hash ^= uint32(key[i])
	}
	return hash
}
