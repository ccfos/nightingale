package sw

import (
	"bufio"
	"bytes"
	"github.com/toolkits/file"
	"github.com/toolkits/sys"
	"io"
	"strconv"
	"strings"
)

func PingStatSummary(ip string, count, timeout int) (m map[string]string, err error) {
	m = make(map[string]string)
	var bs []byte
	bs, err = sys.CmdOutBytes("ping", "-c", strconv.Itoa(count), "-W", strconv.Itoa(timeout), ip)
	if err != nil {
		return m, err
	}

	reader := bufio.NewReader(bytes.NewBuffer(bs))

	// ignore the first line
	line, e := file.ReadLine(reader)
	if e != nil {
		return m, e
	}

	for {
		line, err = file.ReadLine(reader)
		if err == io.EOF {
			err = nil
			break
		} else if err != nil {
			return m, err
		}

		lineStr := string(line)
		if strings.Contains(lineStr, "packet loss") {
			arr := strings.Split(lineStr, ", ")
			for _, val := range arr {
				fields := strings.Fields(val)
				if fields[1] == "packet" {
					m["pkloss"] = fields[0]
				}
			}
		}

		if strings.Contains(lineStr, "min/avg/max") {
			fields := strings.Fields(lineStr)
			result := strings.Split(fields[3], "/")
			m["min"] = result[0]
			m["avg"] = result[1]
			m["max"] = result[2]
		}
	}

	return m, e
}
