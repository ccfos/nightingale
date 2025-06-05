package pool

import (
	"bytes"
	"sync"
	"time"

	gc "github.com/patrickmn/go-cache"
)

var (
	PoolClient = new(sync.Map)
)

var (
	// default cache instance, do not use this if you want to specify the defaultExpiration
	DefaultCache = gc.New(time.Hour*24, time.Hour)
)

var (
	bytesPool = sync.Pool{
		New: func() interface{} { return new(bytes.Buffer) },
	}
)

func PoolGetBytesBuffer() *bytes.Buffer {
	buf := bytesPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func PoolPutBytesBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	bytesPool.Put(buf)
}
