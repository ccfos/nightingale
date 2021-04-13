package logger

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
)

type Logger struct {
	DebugFlag	bool
	VerboseFlag	bool
	*log.Logger
}

var (
	def *Logger
)

func CreateLogger(verbose, debug bool) *Logger {
	def = InitLogger(verbose, debug, log.Ldate|log.Ltime)

	return def
}

func CreateLoggerWithFile(verbose, debug bool, file string) *Logger {
	def = InitLoggerWithFile(verbose, debug, file, log.Ldate|log.Ltime)

	return def
}

func GetDefaultLogger() *Logger {
	return def
}

func InitLogger(debug, verbose bool, flag int) *Logger {
	l := &Logger{debug, verbose, log.New(os.Stdout, "", flag)}
	def = l
	return l
}

func InitLoggerWithFile(debug, verbose bool, file string, flag int) *Logger {
	f, e := os.OpenFile(file, os.O_WRONLY, 0666)

	if e != nil {
		panic("Unable to open log file.")
	}

	l := &Logger{debug, verbose, log.New(f, "", flag)}
	def = l
	return l
}

func (x *Logger) Debug(format string, v ...interface{}) {
	if x.DebugFlag {
		x.Printf("[DEBUG] " + x.getVerboseInfo() + fmt.Sprintf(format, v...))
	}
}

func (x *Logger) Info(format string, v ...interface{}) {
	x.getVerboseInfo()
	x.Printf("[INFO] " + x.getVerboseInfo() + fmt.Sprintf(format, v...))
}

func (x *Logger) Error(format string, v ...interface{}) {
	x.Printf("[ERROR] " + x.getVerboseInfo() + fmt.Sprintf(format, v...))
}

func (x *Logger) Fatal(format string, v ...interface{}) {
	x.Printf("[FATAL] " + x.getVerboseInfo() + fmt.Sprintf(format, v...))
}

func (x *Logger) getVerboseInfo() string {
	var verboseInfo string
	// If verbose info is enabled
	if x.VerboseFlag {
		// Retrieve 3 stacks behind to get the actual caller.
		pc := make([]uintptr, 1)
		ret := runtime.Callers(3, pc)
		if ret > 0 {
			f := runtime.FuncForPC(pc[0])
			file, line := f.FileLine(pc[0])
			verboseInfo = fmt.Sprintf("%s:%d (%s) ", file[strings.LastIndex(file, "/")+1:], line, f.Name())
		}
	}
	return verboseInfo
}
