package middleware

import (
	"net/http"
	"runtime"

	"github.com/didi/nightingale/src/modules/tsdb/http/render"

	"github.com/toolkits/pkg/logger"
)

// Recovery is a Negroni middleware that recovers from any panics and writes a 500 if there was one.
type Recovery struct {
	StackAll  bool
	StackSize int
}

type Error struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Time string `json:"time"`
	File string `json:"file"`
	Line int    `json:"line"`
}

// NewRecovery returns a new instance of Recovery
func NewRecovery() *Recovery {
	return &Recovery{
		StackAll:  false,
		StackSize: 1024 * 8,
	}
}

func (rec *Recovery) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	defer func() {
		if err := recover(); err != nil {
			if e, ok := err.(Error); ok {
				logger.Errorf("[%s:%d] %s [Error:]%s", e.File, e.Line, e.Time, e.Msg)

				render.Message(w, e.Msg)
				return
			}

			// Negroni part
			w.WriteHeader(http.StatusInternalServerError)
			stack := make([]byte, rec.StackSize)
			stack = stack[:runtime.Stack(stack, rec.StackAll)]

			logger.Errorf("PANIC: %s\n%s", err, stack)
		}
	}()

	next(w, r)
}
