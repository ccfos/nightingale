package logger

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Severity int

const (
	FATAL Severity = iota
	ERROR
	WARNING
	INFO
	DEBUG
)

var severityName = []string{
	FATAL:   "FATAL",
	ERROR:   "ERROR",
	WARNING: "WARNING",
	INFO:    "INFO",
	DEBUG:   "DEBUG",
}

const (
	numSeverity = 5
)

type Backend interface {
	Log(s Severity, msg []byte)
	close()
}

type stdBackend struct{}

func (self *stdBackend) Log(s Severity, msg []byte) {
	os.Stdout.Write(msg)
}

func (self *stdBackend) close() {}

type Logger struct {
	s       Severity
	backend Backend
	mu      sync.Mutex

	freeList   *buffer
	freeListMu sync.Mutex

	logToStderr bool
}

//resued buffer for fast format the output string
type buffer struct {
	bytes.Buffer
	tmp  [64]byte
	next *buffer
}

func (self *Logger) getBuffer() *buffer {
	self.freeListMu.Lock()
	b := self.freeList
	if b != nil {
		self.freeList = b.next
	}
	self.freeListMu.Unlock()
	if b == nil {
		b = new(buffer)
	} else {
		b.next = nil
		b.Reset()
	}
	return b
}

// Some custom tiny helper functions to print the log header efficiently.
const digits = "0123456789"

// twoDigits formats a zero-prefixed two-digit integer at buf.tmp[i].
func (buf *buffer) twoDigits(i, d int) {
	buf.tmp[i+1] = digits[d%10]
	d /= 10
	buf.tmp[i] = digits[d%10]
}

// nDigits formats an n-digit integer at buf.tmp[i],
// padding with pad on the left.
// It assumes d >= 0.
func (buf *buffer) nDigits(n, i, d int, pad byte) {
	j := n - 1
	for ; j >= 0 && d > 0; j-- {
		buf.tmp[i+j] = digits[d%10]
		d /= 10
	}
	for ; j >= 0; j-- {
		buf.tmp[i+j] = pad
	}
}

// someDigits formats a zero-prefixed variable-width integer at buf.tmp[i].
func (buf *buffer) someDigits(i, d int) int {
	// Print into the top, then copy down. We know there's space for at least
	// a 10-digit number.
	j := len(buf.tmp)
	for {
		j--
		buf.tmp[j] = digits[d%10]
		d /= 10
		if d == 0 {
			break
		}
	}
	return copy(buf.tmp[i:], buf.tmp[j:])
}

func (self *Logger) putBuffer(b *buffer) {
	if b.Len() >= 256 {
		// Let big buffers die a natural death.
		return
	}
	self.freeListMu.Lock()
	b.next = self.freeList
	self.freeList = b
	self.freeListMu.Unlock()
}

func (self *Logger) formatHeader(s Severity, file string, line int) *buffer {
	now := time.Now()
	if line < 0 {
		line = 0 // not a real line number, but acceptable to someDigits
	}
	buf := self.getBuffer()

	// Avoid Fprintf, for speed. The format is so simple that we can do it quickly by hand.
	// It's worth about 3X. Fprintf is hard.
	year, month, day := now.Date()
	hour, minute, second := now.Clock()
	//2015-06-16 12:00:35 ERROR test.go:12 ...
	buf.nDigits(4, 0, year, '0')
	buf.tmp[4] = '-'
	buf.twoDigits(5, int(month))
	buf.tmp[7] = '-'
	buf.twoDigits(8, day)
	buf.tmp[10] = ' '
	buf.twoDigits(11, hour)
	buf.tmp[13] = ':'
	buf.twoDigits(14, minute)
	buf.tmp[16] = ':'
	buf.twoDigits(17, second)
	buf.tmp[19] = '.'
	buf.nDigits(6, 20, now.Nanosecond()/1000, '0')
	buf.tmp[26] = ' '
	buf.Write(buf.tmp[:27])
	buf.WriteString(severityName[s])
	buf.WriteByte(' ')
	buf.WriteString(file)
	buf.tmp[0] = ':'
	n := buf.someDigits(1, line)
	buf.tmp[n+1] = ' '
	buf.Write(buf.tmp[:n+2])
	return buf
}

func (self *Logger) header(s Severity, depth int) *buffer {
	_, file, line, ok := runtime.Caller(3 + depth)
	if !ok {
		file = "???"
		line = 1
	} else {
		dirs := strings.Split(file, "/")
		if len(dirs) >= 2 {
			file = dirs[len(dirs)-2] + "/" + dirs[len(dirs)-1]
		} else {
			file = dirs[len(dirs)-1]
		}
	}
	return self.formatHeader(s, file, line)
}

func (self *Logger) print(s Severity, args ...interface{}) {
	self.printDepth(s, 1, args...)
}

func (self *Logger) printf(s Severity, format string, args ...interface{}) {
	self.printfDepth(s, 1, format, args...)
}

func (self *Logger) printDepth(s Severity, depth int, args ...interface{}) {
	if self.s < s {
		return
	}
	buf := self.header(s, depth)
	fmt.Fprint(buf, args...)
	if buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	self.output(s, buf)
}

func (self *Logger) printfDepth(s Severity, depth int, format string, args ...interface{}) {
	if self.s < s {
		return
	}
	buf := self.header(s, depth)
	fmt.Fprintf(buf, format, args...)
	if buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	self.output(s, buf)
}

func (self *Logger) printfSimple(format string, args ...interface{}) {
	buf := self.getBuffer()
	fmt.Fprintf(buf, format, args...)
	if buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
	self.output(INFO, buf)
}

func (self *Logger) output(s Severity, buf *buffer) {
	if self.s < s {
		return
	}
	if self.logToStderr {
		os.Stderr.Write(buf.Bytes())
	} else {
		self.backend.Log(s, buf.Bytes())
	}
	if s == FATAL {
		trace := stacks(true)
		os.Stderr.Write(trace)
		os.Exit(255)
	}
	self.putBuffer(buf)
}

func stacks(all bool) []byte {
	// We don't know how big the traces are, so grow a few times if they don't fit. Start large, though.
	n := 10000
	if all {
		n = 100000
	}
	var trace []byte
	for i := 0; i < 5; i++ {
		trace = make([]byte, n)
		nbytes := runtime.Stack(trace, all)
		if nbytes < len(trace) {
			return trace[:nbytes]
		}
		n *= 2
	}
	return trace
}

/*--------------------------logger public functions--------------------------*/

func NewLogger(level interface{}, backend Backend) *Logger {
	l := new(Logger)
	l.SetSeverity(level)
	l.backend = backend
	return l
}

func (l *Logger) SetSeverity(level interface{}) {
	if s, ok := level.(Severity); ok {
		l.s = s
	} else {
		if s, ok := level.(string); ok {
			for i, name := range severityName {
				if name == s {
					l.s = Severity(i)
				}
			}
		}
	}
}

func (l *Logger) Close() {
	if l.backend != nil {
		l.backend.close()
	}
}

func (l *Logger) LogToStderr() {
	l.logToStderr = true
}

func (l *Logger) Debug(args ...interface{}) {
	l.print(DEBUG, args...)
}

func (l *Logger) Debugf(format string, args ...interface{}) {
	l.printf(DEBUG, format, args...)
}

func (l *Logger) Info(args ...interface{}) {
	l.print(INFO, args...)
}

func (l *Logger) Infof(format string, args ...interface{}) {
	l.printf(INFO, format, args...)
}

func (l *Logger) Warning(args ...interface{}) {
	l.print(WARNING, args...)
}

func (l *Logger) Warningf(format string, args ...interface{}) {
	l.printf(WARNING, format, args...)
}

func (l *Logger) Error(args ...interface{}) {
	l.print(ERROR, args...)
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	l.printf(ERROR, format, args...)
}

func (l *Logger) Fatal(args ...interface{}) {
	l.print(FATAL, args...)
}

func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.printf(FATAL, format, args...)
}

func (l *Logger) SetLogging(level interface{}, backend Backend) {
	l.SetSeverity(level)
	l.backend = backend
}

/////////////////////////////////////////////////////////////////
// depth version, only a low level api
func (l *Logger) LogDepth(s Severity, depth int, format string, args ...interface{}) {
	l.printfDepth(s, depth+1, format, args...)
}

func (l *Logger) PrintfSimple(format string, args ...interface{}) {
	l.printfSimple(format, args...)
}

/*---------------------------------------------------------------------------*/

var logging Logger
var fileback *FileBackend = nil
var sysback *syslogBackend = nil

func init() {
	SetLogging(DEBUG, &stdBackend{})
}

func SetLogging(level interface{}, backend Backend) {
	logging.SetLogging(level, backend)
}

func SetSeverity(level interface{}) {
	logging.SetSeverity(level)
}

func Close() {
	logging.Close()
}

func LogToStderr() {
	logging.LogToStderr()
}

/*-----------------------------public functions------------------------------*/

func Debug(args ...interface{}) {
	logging.print(DEBUG, args...)
}

func Debugf(format string, args ...interface{}) {
	logging.printf(DEBUG, format, args...)
}

func Info(args ...interface{}) {
	logging.print(INFO, args...)
}

func Infof(format string, args ...interface{}) {
	logging.printf(INFO, format, args...)
}

func Warning(args ...interface{}) {
	logging.print(WARNING, args...)
}

func Warningf(format string, args ...interface{}) {
	logging.printf(WARNING, format, args...)
}

func Error(args ...interface{}) {
	logging.print(ERROR, args...)
}

func Errorf(format string, args ...interface{}) {
	logging.printf(ERROR, format, args...)
}

func Fatal(args ...interface{}) {
	logging.print(FATAL, args...)
}

func Fatalf(format string, args ...interface{}) {
	logging.printf(FATAL, format, args...)
}

func LogDepth(s Severity, depth int, format string, args ...interface{}) {
	logging.printfDepth(s, depth+1, format, args...)
}

func Printf(format string, args ...interface{}) {
	logging.printfSimple(format, args...)
}

func GetLogger() *Logger {
	return &logging
}
