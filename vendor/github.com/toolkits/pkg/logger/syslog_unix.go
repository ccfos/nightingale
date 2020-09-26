// +build linux darwin freebsd openbsd solaris

package logger

import (
	"fmt"
	"log/syslog"
	"os"
)

type syslogBackend struct {
	writer [numSeverity]*syslog.Writer
	buf    [numSeverity]chan []byte
}

var SyslogPriorityMap = map[string]syslog.Priority{
	"local0": syslog.LOG_LOCAL0,
	"local1": syslog.LOG_LOCAL1,
	"local2": syslog.LOG_LOCAL2,
	"local3": syslog.LOG_LOCAL3,
	"local4": syslog.LOG_LOCAL4,
	"local5": syslog.LOG_LOCAL5,
	"local6": syslog.LOG_LOCAL6,
	"local7": syslog.LOG_LOCAL7,
}

var pmap = []syslog.Priority{syslog.LOG_EMERG, syslog.LOG_ERR, syslog.LOG_WARNING, syslog.LOG_INFO, syslog.LOG_DEBUG}

func NewSyslogBackend(priorityStr string, tag string) (*syslogBackend, error) {
	priority, ok := SyslogPriorityMap[priorityStr]
	if !ok {
		return nil, fmt.Errorf("unknown syslog priority: %s", priorityStr)
	}
	var err error
	var b syslogBackend
	for i := 0; i < numSeverity; i++ {
		b.writer[i], err = syslog.New(priority|pmap[i], tag)
		if err != nil {
			return nil, err
		}
		b.buf[i] = make(chan []byte, 1<<16)
	}
	b.log()
	return &b, nil
}

func DialSyslogBackend(network, raddr string, priority syslog.Priority, tag string) (*syslogBackend, error) {
	var err error
	var b syslogBackend
	for i := 0; i < numSeverity; i++ {
		b.writer[i], err = syslog.Dial(network, raddr, priority|pmap[i], tag+severityName[i])
		if err != nil {
			return nil, err
		}
		b.buf[i] = make(chan []byte, 1<<16)
	}
	b.log()
	return &b, nil
}

func (self *syslogBackend) Log(s Severity, msg []byte) {
	msg1 := make([]byte, len(msg))
	copy(msg1, msg)
	switch s {
	case FATAL:
		self.tryPutInBuf(FATAL, msg1)
	case ERROR:
		self.tryPutInBuf(ERROR, msg1)
	case WARNING:
		self.tryPutInBuf(WARNING, msg1)
	case INFO:
		self.tryPutInBuf(INFO, msg1)
	case DEBUG:
		self.tryPutInBuf(DEBUG, msg1)
	}
}

func (self *syslogBackend) close() {
	for i := 0; i < numSeverity; i++ {
		self.writer[i].Close()
	}
}

func (self *syslogBackend) tryPutInBuf(s Severity, msg []byte) {
	select {
	case self.buf[s] <- msg:
	default:
		os.Stderr.Write(msg)
	}
}

func (self *syslogBackend) log() {
	for i := 0; i < numSeverity; i++ {
		go func(index int) {
			for {
				msg := <-self.buf[index]
				self.writer[index].Write(msg[27:])
			}
		}(i)
	}
}
