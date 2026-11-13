package router

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/ccfos/nightingale/v6/dskit/doris"
	"github.com/ccfos/nightingale/v6/pkg/aop"
	"github.com/ccfos/nightingale/v6/pkg/ginx"
	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/logger"
)

// LogQueryResponse encodes one row at a time into a bounded HTTP buffer.
// Query errors finish the envelope with a nonempty err; transport errors stop it.
type LogQueryResponse struct {
	c         *gin.Context
	buffer    *bufio.Writer
	started   bool
	finished  bool
	writeErr  error
	rows      int64
	batchTail []byte
	outerRows int64
}

// NewLogQueryResponse creates a row writer for the existing log-query JSON envelope.
func NewLogQueryResponse(c *gin.Context) *LogQueryResponse {
	target := c.Writer
	// The standard access logger and this project's Gin fork capture entire
	// bodies on Write. Stream only these Doris responses without that capture;
	// the underlying Gin writer still records status and byte counts.
	switch logger := target.(type) {
	case *aop.CustomResponseWriter:
		target = logger.ResponseWriter
	case aop.CustomResponseWriter:
		target = logger.ResponseWriter
	}
	w := &LogQueryResponse{c: c, buffer: bufio.NewWriterSize(logQueryStreamingBody{target}, 32<<10)}
	return w
}

type logQueryStreamingBody struct{ writer gin.ResponseWriter }

func (w logQueryStreamingBody) Write(body []byte) (int, error) {
	// Gin.WriteString updates status/size without appending to its body buffer.
	written := 0
	for len(body) > 0 {
		size := min(len(body), 32<<10)
		n, err := w.writer.WriteString(string(body[:size]))
		written += n
		if err != nil {
			return written, err
		}
		if n != size {
			return written, io.ErrShortWrite
		}
		body = body[size:]
	}
	return written, nil
}

// Write accepts an already encoded complete row.
func (w *LogQueryResponse) Write(body []byte) (int, error) {
	if err := w.startItem(); err != nil {
		return 0, err
	}
	n, err := w.buffer.Write(body)
	if err == nil {
		w.rows++
	} else {
		w.writeErr = err
	}
	return n, err
}

func (w *LogQueryResponse) startItem() error {
	if w.finished {
		return io.ErrClosedPipe
	}
	if !w.started {
		w.c.Header("Content-Type", "application/json; charset=utf-8")
		w.started = true
		if _, err := w.buffer.WriteString("{\"dat\":{\"list\":["); err != nil {
			w.writeErr = err
			return err
		}
	}
	if w.rows > 0 {
		if err := w.buffer.WriteByte(','); err != nil {
			w.writeErr = err
			return err
		}
	}
	return nil
}

func (w *LogQueryResponse) beginBatchQuery(q Query) error {
	tail, err := json.Marshal(map[string]any{"ref": q.Ref, "ds_id": q.Did, "ds_cate": q.DsCate})
	if err != nil {
		return err
	}
	if err := w.startItem(); err != nil {
		return err
	}
	if _, err := w.buffer.WriteString("{\"data\":["); err != nil {
		w.writeErr = err
		return err
	}
	w.batchTail = tail[1:]
	w.outerRows, w.rows = w.rows, 0
	return nil
}

func (w *LogQueryResponse) endBatchQuery() error {
	if w.batchTail == nil {
		return nil
	}
	if _, err := w.buffer.WriteString("],"); err != nil {
		w.writeErr = err
		return err
	}
	if _, err := w.buffer.Write(w.batchTail); err != nil {
		w.writeErr = err
		return err
	}
	w.rows = w.outerRows + 1
	w.batchTail = nil
	return nil
}

// Row encodes and writes one complete row before returning.
func (w *LogQueryResponse) Row(row map[string]any) error {
	if w.finished {
		return io.ErrClosedPipe
	}
	if err := w.c.Request.Context().Err(); err != nil {
		return err
	}
	prepared, err := doris.PrepareJSONRow(row)
	if err != nil {
		return err
	}
	if err := w.startItem(); err != nil {
		w.writeErr = err
		return err
	}
	// Finish the current row on application cancellation so Fail can close a
	// valid envelope. A transport write error still stops output immediately.
	if err := prepared.Write(context.WithoutCancel(w.c.Request.Context()), w.buffer); err != nil {
		w.writeErr = err
		return err
	}
	if err := w.buffer.WriteByte('\n'); err != nil {
		w.writeErr = err
		return err
	}
	w.rows++
	return w.c.Request.Context().Err()
}

// Finish closes a successful response with its total and optional pagination flag.
func (w *LogQueryResponse) Finish(total int64, pagination *bool) error {
	return w.complete(total, pagination, "")
}

func (w *LogQueryResponse) complete(total int64, pagination *bool, message string) error {
	if w.finished {
		return io.ErrClosedPipe
	}
	if err := w.endBatchQuery(); err != nil {
		w.finished = true
		return err
	}
	w.finished = true
	if !w.started {
		w.c.Header("Content-Type", "application/json; charset=utf-8")
		w.started = true
		if _, err := w.buffer.WriteString("{\"dat\":{\"list\":["); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(w.buffer, "],\"total\":%d", total); err != nil {
		return err
	}
	if pagination != nil {
		if _, err := fmt.Fprintf(w.buffer, ",\"pagination_supported\":%t", *pagination); err != nil {
			return err
		}
	}
	trace, err := json.Marshal(w.c.GetString("trace_id"))
	if err != nil {
		return err
	}
	errorJSON, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.buffer, "},\"err\":%s,\"request_id\":%s}", errorJSON, trace); err != nil {
		return err
	}
	return w.buffer.Flush()
}

// Fail closes a started JSON response with err, unless the transport has failed.
func (w *LogQueryResponse) Fail(err error) {
	defer w.c.Abort()
	if w.finished || w.writeErr != nil {
		logger.Warningf("Doris streaming response stopped: request_id=%s err=%v", w.c.GetString("trace_id"), err)
		w.finished = true
		return
	}
	message := err.Error()
	if message == "" {
		message = "Doris query failed"
	}
	if !w.c.Writer.Written() {
		w.finished = true
		ginx.Bomb(200, "%s", message)
		return
	}
	logger.Warningf("Doris streaming response failed: request_id=%s err=%v", w.c.GetString("trace_id"), err)
	// The client rejects nonempty err and ignores any rows already transmitted.
	if writeErr := w.complete(0, nil, message); writeErr != nil {
		logger.Warningf("Doris streaming error response failed: request_id=%s err=%v", w.c.GetString("trace_id"), writeErr)
	}
}

var _ io.Writer = (*LogQueryResponse)(nil)
