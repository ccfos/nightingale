package nux

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strconv"
	"strings"
	"time"

	"github.com/toolkits/pkg/file"
)

type DiskStats struct {
	Major             int
	Minor             int
	Device            string
	ReadRequests      uint64 // Total number of reads completed successfully.
	ReadMerged        uint64 // Adjacent read requests merged in a single req.
	ReadSectors       uint64 // Total number of sectors read successfully.
	MsecRead          uint64 // Total number of ms spent by all reads.
	WriteRequests     uint64 // total number of writes completed successfully.
	WriteMerged       uint64 // Adjacent write requests merged in a single req.
	WriteSectors      uint64 // total number of sectors written successfully.
	MsecWrite         uint64 // Total number of ms spent by all writes.
	IosInProgress     uint64 // Number of actual I/O requests currently in flight.
	MsecTotal         uint64 // Amount of time during which ios_in_progress >= 1.
	MsecWeightedTotal uint64 // Measure of recent I/O completion time and backlog.
	TS                time.Time
}

func (this *DiskStats) String() string {
	return fmt.Sprintf("<Device:%s, Major:%d, Minor:%d, ReadRequests:%d...>", this.Device, this.Major, this.Minor, this.ReadRequests)
}

func ListDiskStats() ([]*DiskStats, error) {
	proc_diskstats := "/proc/diskstats"
	if !file.IsExist(proc_diskstats) {
		return nil, fmt.Errorf("%s not exists", proc_diskstats)
	}

	contents, err := ioutil.ReadFile(proc_diskstats)
	if err != nil {
		return nil, err
	}

	ret := make([]*DiskStats, 0)

	reader := bufio.NewReader(bytes.NewBuffer(contents))
	for {
		line, err := file.ReadLine(reader)
		if err == io.EOF {
			err = nil
			break
		} else if err != nil {
			return nil, err
		}

		fields := strings.Fields(string(line))
		// shortcut the deduper and just skip disks that
		// haven't done a single read.  This elimiates a bunch
		// of loopback, ramdisk, and cdrom devices but still
		// lets us report on the rare case that we actually use
		// a ramdisk.
		if fields[3] == "0" {
			continue
		}

		size := len(fields)
		// kernel version too low
		if size != 14 {
			continue
		}

		item := &DiskStats{}
		if item.Major, err = strconv.Atoi(fields[0]); err != nil {
			return nil, err
		}

		if item.Minor, err = strconv.Atoi(fields[1]); err != nil {
			return nil, err
		}

		item.Device = fields[2]

		if item.ReadRequests, err = strconv.ParseUint(fields[3], 10, 64); err != nil {
			return nil, err
		}

		if item.ReadMerged, err = strconv.ParseUint(fields[4], 10, 64); err != nil {
			return nil, err
		}

		if item.ReadSectors, err = strconv.ParseUint(fields[5], 10, 64); err != nil {
			return nil, err
		}

		if item.MsecRead, err = strconv.ParseUint(fields[6], 10, 64); err != nil {
			return nil, err
		}

		if item.WriteRequests, err = strconv.ParseUint(fields[7], 10, 64); err != nil {
			return nil, err
		}

		if item.WriteMerged, err = strconv.ParseUint(fields[8], 10, 64); err != nil {
			return nil, err
		}

		if item.WriteSectors, err = strconv.ParseUint(fields[9], 10, 64); err != nil {
			return nil, err
		}

		if item.MsecWrite, err = strconv.ParseUint(fields[10], 10, 64); err != nil {
			return nil, err
		}

		if item.IosInProgress, err = strconv.ParseUint(fields[11], 10, 64); err != nil {
			return nil, err
		}

		if item.MsecTotal, err = strconv.ParseUint(fields[12], 10, 64); err != nil {
			return nil, err
		}

		if item.MsecWeightedTotal, err = strconv.ParseUint(fields[13], 10, 64); err != nil {
			return nil, err
		}

		item.TS = time.Now()
		ret = append(ret, item)
	}
	return ret, nil
}
