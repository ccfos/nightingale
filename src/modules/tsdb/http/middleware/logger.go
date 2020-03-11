package middleware

import (
	"log"
	"net/http"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/toolkits/pkg/logger"
)

// Logger is a middleware handler that logs the request as it goes in and the response as it goes out.
type Logger struct {
	// Logger inherits from log.Logger used to log messages with the Logger middleware
	*log.Logger
}

// NewLogger returns a new Logger instance
func NewLogger() *Logger {
	return &Logger{}
}

func (l *Logger) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	start := time.Now()
	next(rw, r)

	res := rw.(negroni.ResponseWriter)
	logger.Debugf("%v [method:%s][uri:%s][status:%d][use:%v][from:%s]", time.Now().Format("2006/01/02 15:04:05"), r.Method, r.URL.Path, res.Status(), time.Since(start), r.RemoteAddr)
}
