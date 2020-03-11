package nux

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/sys"
)

func SocketStatSummary() (m map[string]uint64, err error) {
	m = make(map[string]uint64)
	bs, err := ioutil.ReadFile("/proc/net/sockstat")
	if err != nil {
		return
	}
	reader := bufio.NewReader(bytes.NewBuffer(bs))

	for {
		var lineBytes []byte
		lineBytes, err = file.ReadLine(reader)
		if err == io.EOF {
			return
		}
		line := string(lineBytes)
		s := strings.Split(line, " ")
		if strings.HasPrefix(line, "sockets: used") {
			m["sockets.used"], _ = strconv.ParseUint(s[2], 10, 64)
		} else {
			m["sockets.tcp.inuse"], _ = strconv.ParseUint(s[2], 10, 64)
			m["sockets.tcp.timewait"], _ = strconv.ParseUint(s[6], 10, 64)
			break
		}
	}

	return
}

func ss() (m map[string]uint64, err error) {
	m = make(map[string]uint64)
	var bs []byte
	bs, err = sys.CmdOutBytes("sh", "-c", "ss -s")
	if err != nil {
		return
	}

	reader := bufio.NewReader(bytes.NewBuffer(bs))

	// ignore the first line
	line, e := file.ReadLine(reader)
	if e != nil {
		return m, e
	}

	for {
		line, err = file.ReadLine(reader)
		if err != nil {
			return
		}

		lineStr := string(line)
		if strings.HasPrefix(lineStr, "TCP") {
			left := strings.Index(lineStr, "(")
			right := strings.Index(lineStr, ")")
			if left < 0 || right < 0 {
				continue
			}

			content := lineStr[left+1 : right]
			arr := strings.Split(content, ", ")
			for _, val := range arr {
				fields := strings.Fields(val)
				if fields[0] == "timewait" {
					timewait_arr := strings.Split(fields[1], "/")
					m["timewait"], _ = strconv.ParseUint(timewait_arr[0], 10, 64)
					m["slabinfo.timewait"], _ = strconv.ParseUint(timewait_arr[1], 10, 64)
					continue
				}
				m[fields[0]], _ = strconv.ParseUint(fields[1], 10, 64)
			}
			return
		}
	}

	return
}
