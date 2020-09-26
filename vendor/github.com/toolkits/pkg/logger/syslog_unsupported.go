// +build windows plan9 netbsd

package logger

import (
	"fmt"
)

type syslogBackend struct {
}

func NewSyslogBackend(priorityStr string, tag string) (*syslogBackend, error) {
	return nil, fmt.Errorf("Platform does not support syslog")
}

func (self *syslogBackend) Log(s Severity, msg []byte) {
}

func (self *syslogBackend) close() {
}

func (self *syslogBackend) tryPutInBuf(s Severity, msg []byte) {
}

func (self *syslogBackend) log() {
}
