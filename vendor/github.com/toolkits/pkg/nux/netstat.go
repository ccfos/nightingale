package nux

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
)

// @param ext e.g. TcpExt or IpExt
func Netstat(ext string) (ret map[string]uint64, err error) {
	ret = make(map[string]uint64)
	var contents []byte
	contents, err = ioutil.ReadFile("/proc/net/netstat")
	if err != nil {
		return
	}

	reader := bufio.NewReader(bytes.NewBuffer(contents))
	for {
		var bs []byte
		bs, err = file.ReadLine(reader)
		if err == io.EOF {
			err = nil
			break
		} else if err != nil {
			return
		}

		line := string(bs)
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}

		title := strings.TrimSpace(line[:idx])
		if title == ext {
			ths := strings.Fields(strings.TrimSpace(line[idx+1:]))
			// the next line must be values
			bs, err = file.ReadLine(reader)
			if err != nil {
				return
			}

			valLine := string(bs)
			tds := strings.Fields(strings.TrimSpace(valLine[idx+1:]))
			for i := 0; i < len(ths); i++ {
				ret[ths[i]], err = strconv.ParseUint(tds[i], 10, 64)
				if err != nil {
					return
				}
			}

			return
		}

	}

	return
}
