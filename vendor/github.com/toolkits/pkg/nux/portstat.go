package nux

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/toolkits/pkg/file"
	"github.com/toolkits/pkg/slice"
	"github.com/toolkits/pkg/sys"
)

// ListeningPorts 为了兼容老代码
func ListeningPorts() ([]int64, error) {
	return TcpPorts()
}

func TcpPorts() ([]int64, error) {
	return listeningPorts("sh", "-c", "ss -t -l -n")
}

func UdpPorts() ([]int64, error) {
	return listeningPorts("sh", "-c", "ss -u -a -n")
}

func listeningPorts(name string, args ...string) ([]int64, error) {
	ports := []int64{}

	bs, err := sys.CmdOutBytes(name, args...)
	if err != nil {
		return ports, err
	}

	reader := bufio.NewReader(bytes.NewBuffer(bs))

	// ignore the first line
	line, err := file.ReadLine(reader)
	if err != nil {
		return ports, err
	}

	for {
		line, err = file.ReadLine(reader)
		if err == io.EOF {
			err = nil
			break
		} else if err != nil {
			return ports, err
		}

		fields := strings.Fields(string(line))
		fieldsLen := len(fields)

		if fieldsLen != 4 && fieldsLen != 5 {
			return ports, fmt.Errorf("output of %s format not supported", name)
		}

		portColumnIndex := 2
		if fieldsLen == 5 {
			portColumnIndex = 3
		}

		location := strings.LastIndex(fields[portColumnIndex], ":")
		port := fields[portColumnIndex][location+1:]

		if p, e := strconv.ParseInt(port, 10, 64); e != nil {
			return ports, fmt.Errorf("parse port to int64 fail: %s", e.Error())
		} else {
			ports = append(ports, p)
		}

	}

	return slice.UniqueInt64(ports), nil
}
