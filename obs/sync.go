package obs

import (
	"fmt"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/time"
)

type SyncRecord struct {
	Timestamp int64
	Mills     int64
	Count     int
	Message   string
}

func (sr *SyncRecord) String() string {
	var sb strings.Builder
	sb.WriteString("timestamp: ")
	sb.WriteString(time.Format(sr.Timestamp))
	sb.WriteString(", mills: ")
	sb.WriteString(fmt.Sprint(sr.Mills, "ms"))
	sb.WriteString(", count: ")
	sb.WriteString(fmt.Sprint(sr.Count))
	sb.WriteString(", message: ")
	sb.WriteString(sr.Message)

	return sb.String()
}

type SyncRecords struct {
	Current *SyncRecord
	Last    *SyncRecord
}

type SyncObs struct {
	sync.RWMutex
	records map[string]*SyncRecords
}

func NewSyncObs() *SyncObs {
	return &SyncObs{
		records: make(map[string]*SyncRecords),
	}
}

var syncObs = NewSyncObs()

func (so *SyncObs) Put(key string, timestamp, mills int64, count int, message string) {
	sr := &SyncRecord{
		Timestamp: timestamp,
		Mills:     mills,
		Count:     count,
		Message:   message,
	}

	so.Lock()
	defer so.Unlock()

	if _, ok := so.records[key]; !ok {
		so.records[key] = &SyncRecords{Current: sr}
		return
	}

	so.records[key].Last = so.records[key].Current
	so.records[key].Current = sr
}

// busi_groups:
// last: timestamp, mills, count
// curr: timestamp, mills, count
func (so *SyncObs) Sprint() string {
	so.RLock()
	defer so.RUnlock()

	var sb strings.Builder
	sb.WriteString("\n")

	for k, v := range so.records {
		sb.WriteString(k)
		sb.WriteString(":\n")
		if v.Last != nil {
			sb.WriteString("last: ")
			sb.WriteString(v.Last.String())
			sb.WriteString("\n")
		}
		sb.WriteString("curr: ")
		sb.WriteString(v.Current.String())
		sb.WriteString("\n\n")
	}

	return sb.String()
}

func (so *SyncObs) ConfigRouter(r *gin.Engine) {
	r.GET("/obs/sync", func(c *gin.Context) {
		clientIP := c.ClientIP()
		if clientIP != "127.0.0.1" && clientIP != "::1" {
			c.String(403, "forbidden")
			return
		}
		c.String(200, so.Sprint())
	})
}

// package level functions
func ConfigSyncRouter(r *gin.Engine) {
	syncObs.ConfigRouter(r)
}

func PutSyncRecord(key string, timestamp, mills int64, count int, message string) {
	syncObs.Put(key, timestamp, mills, count, message)
}
