package logger

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	bufferSize = 256 * 1024
)

func getLastCheck(now time.Time) uint64 {
	return uint64(now.Year())*1000000 + uint64(now.Month())*10000 + uint64(now.Day())*100 + uint64(now.Hour())
}

type syncBuffer struct {
	*bufio.Writer
	file     *os.File
	count    uint64
	cur      int
	filePath string
	parent   *FileBackend
}

func (self *syncBuffer) Sync() error {
	return self.file.Sync()
}

func (self *syncBuffer) close() {
	self.Flush()
	self.Sync()
	self.file.Close()
}

func (self *syncBuffer) write(b []byte) {
	if !self.parent.rotateByHour && self.parent.maxSize > 0 && self.parent.rotateNum > 0 && self.count+uint64(len(b)) >= self.parent.maxSize {
		os.Rename(self.filePath, self.filePath+fmt.Sprintf(".%03d", self.cur))
		self.cur++
		if self.cur >= self.parent.rotateNum {
			self.cur = 0
		}
		self.count = 0
	}
	self.count += uint64(len(b))
	self.Writer.Write(b)
}

type FileBackend struct {
	mu            sync.Mutex
	dir           string //directory for log files
	files         [numSeverity]syncBuffer
	flushInterval time.Duration
	rotateNum     int
	maxSize       uint64
	fall          bool
	rotateByHour  bool
	lastCheck     uint64
	reg           *regexp.Regexp // for rotatebyhour log del...
	keepHours     uint           // keep how many hours old, only make sense when rotatebyhour is T
}

func (self *FileBackend) Flush() {
	self.mu.Lock()
	defer self.mu.Unlock()
	for i := 0; i < numSeverity; i++ {
		self.files[i].Flush()
		self.files[i].Sync()
	}

}

func (self *FileBackend) close() {
	self.Flush()
}

func (self *FileBackend) flushDaemon() {
	for {
		time.Sleep(self.flushInterval)
		self.Flush()
	}
}

func shouldDel(fileName string, left uint) bool {
	// tag should be like 2016071114
	tagInt, err := strconv.Atoi(strings.Split(fileName, ".")[2])
	if err != nil {
		return false
	}

	point := time.Now().Unix() - int64(left*3600)

	if getLastCheck(time.Unix(point, 0)) > uint64(tagInt) {
		return true
	}

	return false

}

func (self *FileBackend) rotateByHourDaemon() {
	for {
		time.Sleep(time.Second * 1)

		if self.rotateByHour {
			check := getLastCheck(time.Now())
			if self.lastCheck < check {
				for i := 0; i < numSeverity; i++ {
					os.Rename(self.files[i].filePath, self.files[i].filePath+fmt.Sprintf(".%d", self.lastCheck))
				}
				self.lastCheck = check
			}

			// also check log dir to del overtime files
			files, err := ioutil.ReadDir(self.dir)
			if err == nil {
				for _, file := range files {
					// exactly match, then we
					if file.Name() == self.reg.FindString(file.Name()) &&
						shouldDel(file.Name(), self.keepHours) {
						os.Remove(filepath.Join(self.dir, file.Name()))
					}
				}
			}
		}
	}
}

func (self *FileBackend) monitorFiles() {
	for range time.NewTicker(time.Second * 5).C {
		for i := 0; i < numSeverity; i++ {
			fileName := path.Join(self.dir, severityName[i]+".log")
			if _, err := os.Stat(fileName); err != nil && os.IsNotExist(err) {
				if f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
					self.mu.Lock()
					self.files[i].close()
					self.files[i].Writer = bufio.NewWriterSize(f, bufferSize)
					self.files[i].file = f
					self.mu.Unlock()
				}
			}
		}
	}
}

func (self *FileBackend) Log(s Severity, msg []byte) {
	self.mu.Lock()
	switch s {
	case FATAL:
		self.files[FATAL].write(msg)
	case ERROR:
		self.files[ERROR].write(msg)
	case WARNING:
		self.files[WARNING].write(msg)
	case INFO:
		self.files[INFO].write(msg)
	case DEBUG:
		self.files[DEBUG].write(msg)
	}
	if self.fall && s < INFO {
		self.files[INFO].write(msg)
	}
	self.mu.Unlock()
	if s == FATAL {
		self.Flush()
	}
}

func (self *FileBackend) Rotate(rotateNum1 int, maxSize1 uint64) {
	self.rotateNum = rotateNum1
	self.maxSize = maxSize1
}

func (self *FileBackend) SetRotateByHour(rotateByHour bool) {
	self.rotateByHour = rotateByHour
	if self.rotateByHour {
		self.lastCheck = getLastCheck(time.Now())
	} else {
		self.lastCheck = 0
	}
}

func (self *FileBackend) SetKeepHours(hours uint) {
	self.keepHours = hours
}

func (self *FileBackend) Fall() {
	self.fall = true
}

func (self *FileBackend) SetFlushDuration(t time.Duration) {
	if t >= time.Second {
		self.flushInterval = t
	} else {
		self.flushInterval = time.Second
	}
}
func NewFileBackend(dir string) (*FileBackend, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	var fb FileBackend
	fb.dir = dir
	for i := 0; i < numSeverity; i++ {
		fileName := path.Join(dir, severityName[i]+".log")
		f, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}

		count := uint64(0)
		stat, err := f.Stat()
		if err == nil {
			count = uint64(stat.Size())
		}
		fb.files[i] = syncBuffer{
			Writer:   bufio.NewWriterSize(f, bufferSize),
			file:     f,
			filePath: fileName,
			parent:   &fb,
			count:    count,
		}

	}
	// default
	fb.flushInterval = time.Second * 3
	fb.rotateNum = 20
	fb.maxSize = 1024 * 1024 * 1024
	fb.rotateByHour = false
	fb.lastCheck = 0
	// init reg to match files
	// ONLY cover this centry...
	fb.reg = regexp.MustCompile("(INFO|ERROR|WARNING|DEBUG|FATAL)\\.log\\.20[0-9]{8}")
	fb.keepHours = 24 * 7

	go fb.flushDaemon()
	go fb.monitorFiles()
	go fb.rotateByHourDaemon()
	return &fb, nil
}

func Rotate(rotateNum1 int, maxSize1 uint64) {
	if fileback != nil {
		fileback.Rotate(rotateNum1, maxSize1)
	}
}

func Fall() {
	if fileback != nil {
		fileback.Fall()
	}
}

func SetFlushDuration(t time.Duration) {
	if fileback != nil {
		fileback.SetFlushDuration(t)
	}

}

func SetRotateByHour(rotateByHour bool) {
	if fileback != nil {
		fileback.SetRotateByHour(rotateByHour)
	}
}

func SetKeepHours(hours uint) {
	if fileback != nil {
		fileback.SetKeepHours(hours)
	}
}
