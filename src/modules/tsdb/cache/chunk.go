package cache

import (
	"fmt"
	"sync"

	"github.com/didi/nightingale/src/modules/tsdb/utils"

	tsz "github.com/dgryski/go-tsz"
	"github.com/toolkits/pkg/logger"
)

var FlushDoneChan chan int

func init() {
	FlushDoneChan = make(chan int, 1)
}

type Chunk struct {
	tsz.Series
	FirstTs   uint32
	LastTs    uint32
	NumPoints uint32
	Closed    bool
}

func NewChunk(t0 uint32) *Chunk {
	return &Chunk{
		Series:    *tsz.New(t0),
		FirstTs:   0,
		LastTs:    0,
		NumPoints: 0,
		Closed:    false,
	}
}

func (c *Chunk) Push(t uint32, v float64) error {
	if t <= c.LastTs {
		return fmt.Errorf("Point must be newer than already added points. t:%d v:%v,lastTs: %d\n", t, v, c.LastTs)
	}
	c.Series.Push(t, v)
	c.NumPoints += 1
	c.LastTs = t

	return nil
}

func (c *Chunk) FinishSync() {
	c.Closed = true //存在panic的可能
	c.Series.Finish()
}

var ChunksSlots *ChunksSlot

type ChunksSlot struct {
	sync.RWMutex
	Data []map[string][]*Chunk
	Size int
}

func (c *ChunksSlot) Len(idx int) int {
	c.Lock()
	defer c.Unlock()

	return len(c.Data[idx])
}

func (c *ChunksSlot) Get(idx int) map[string][]*Chunk {
	c.Lock()
	defer c.Unlock()

	items := c.Data[idx]
	ret := make(map[string][]*Chunk)
	for k, v := range items {
		ret[k] = v
	}
	c.Data[idx] = make(map[string][]*Chunk)

	return ret
}

func (c *ChunksSlot) GetChunks(key string) ([]*Chunk, bool) {
	c.Lock()
	defer c.Unlock()

	idx, err := GetChunkIndex(key, c.Size)
	if err != nil {
		logger.Error(err)
		return nil, false
	}

	val, ok := c.Data[idx][key]
	if ok {
		delete(c.Data[idx], key)
	}
	return val, ok
}

func (c *ChunksSlot) PushChunks(key string, vals []*Chunk) {
	c.Lock()
	defer c.Unlock()
	idx, err := GetChunkIndex(key, c.Size)
	if err != nil {
		logger.Error(err)
		return
	}
	if _, exists := c.Data[idx][key]; !exists {
		c.Data[idx][key] = make([]*Chunk, 0)
	} else {
		for _, v := range c.Data[idx][key] {
			vals = append(vals, v)
		}
	}

	c.Data[idx][key] = vals
}

func (c *ChunksSlot) Push(key string, val *Chunk) {
	c.Lock()
	defer c.Unlock()
	idx, err := GetChunkIndex(key, c.Size)
	if err != nil {
		logger.Error(err)
		return
	}
	if _, exists := c.Data[idx][key]; !exists {
		c.Data[idx][key] = make([]*Chunk, 0)
	}

	c.Data[idx][key] = append(c.Data[idx][key], val)
}

func GetChunkIndex(key string, size int) (uint32, error) {
	return utils.HashKey(key) % uint32(size), nil
}
