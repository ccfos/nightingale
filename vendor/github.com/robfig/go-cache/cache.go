package cache

import (
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"runtime"
	"sync"
	"time"
)

var (
	ErrKeyExists = fmt.Errorf("item already exists")
	ErrCacheMiss = fmt.Errorf("item not found")
)

type unexportedInterface interface {
	Set(string, interface{}, time.Duration)
	Add(string, interface{}, time.Duration) error
	Replace(string, interface{}, time.Duration) error
	Get(string) (interface{}, bool)
	Increment(string, int64) error
	IncrementFloat(string, float64) error
	Decrement(string, int64) error
	Delete(string)
	DeleteExpired()
	Flush()
	Save(io.Writer) error
	SaveFile(string) error
	Load(io.Reader) error
	LoadFile(io.Reader) error
}

type item struct {
	Object     interface{}
	Expiration *time.Time
}

// Returns true if the item has expired.
func (i *item) Expired() bool {
	if i.Expiration == nil {
		return false
	}
	return i.Expiration.Before(time.Now())
}

type Cache struct {
	*cache
	// If this is confusing, see the comment at the bottom of New()
}

type cache struct {
	sync.Mutex
	defaultExpiration time.Duration
	items             map[string]*item
	janitor           *janitor
}

// Add an item to the cache, replacing any existing item. If the duration is 0,
// the cache's default expiration time is used. If it is -1, the item never
// expires.
func (c *cache) Set(k string, x interface{}, d time.Duration) {
	c.Lock()
	c.set(k, x, d)
	// TODO: Calls to mu.Unlock are currently not deferred because defer
	// adds ~200 ns (as of go1.)
	c.Unlock()
}

func (c *cache) set(k string, x interface{}, d time.Duration) {
	var e *time.Time
	if d == 0 {
		d = c.defaultExpiration
	}
	if d > 0 {
		t := time.Now().Add(d)
		e = &t
	}
	c.items[k] = &item{
		Object:     x,
		Expiration: e,
	}
}

// Add an item to the cache only if an item doesn't already exist for the given
// key, or if the existing item has expired. Returns an error otherwise.
func (c *cache) Add(k string, x interface{}, d time.Duration) error {
	c.Lock()
	_, found := c.get(k)
	if found {
		c.Unlock()
		return ErrKeyExists
	}
	c.set(k, x, d)
	c.Unlock()
	return nil
}

// Set a new value for the cache key only if it already exists. Returns an
// error if it does not.
func (c *cache) Replace(k string, x interface{}, d time.Duration) error {
	c.Lock()
	_, found := c.get(k)
	if !found {
		c.Unlock()
		return fmt.Errorf("item %s doesn't exist", k)
	}
	c.set(k, x, d)
	c.Unlock()
	return nil
}

// Get an item from the cache. Returns the item or nil, and a bool indicating
// whether the key was found.
func (c *cache) Get(k string) (interface{}, bool) {
	c.Lock()
	x, found := c.get(k)
	c.Unlock()
	return x, found
}

func (c *cache) get(k string) (interface{}, bool) {
	item, found := c.items[k]
	if !found {
		return nil, false
	}
	if item.Expired() {
		c.delete(k)
		return nil, false
	}
	return item.Object, true
}

// Increment an item of type float32 or float64 by n. Returns an error if the
// item's value is not floating point, if it was not found, or if it is not
// possible to increment it by n. Pass a negative number to decrement the
// value.
func (c *cache) IncrementFloat(k string, n float64) error {
	c.Lock()
	v, found := c.items[k]
	if !found || v.Expired() {
		c.Unlock()
		return fmt.Errorf("item not found")
	}
	switch v.Object.(type) {
	case float32:
		v.Object = v.Object.(float32) + float32(n)
	case float64:
		v.Object = v.Object.(float64) + n
	default:
		c.Unlock()
		return fmt.Errorf("The value for %s does not have type float32 or float64", k)
	}
	c.Unlock()
	return nil
}

// Increment an item of type int, int8, int16, int32, int64, uintptr, uint,
// uint8, uint32, or uint64 by n. Returns an error if the
// item's value is not an integer, if it was not found, or if it is not
// possible to increment it by n.
// Wraps around on overlow.
func (c *cache) Increment(k string, n uint64) (uint64, error) {
	c.Lock()
	defer c.Unlock()
	v, found := c.items[k]
	if !found || v.Expired() {
		return 0, ErrCacheMiss
	}
	switch v.Object.(type) {
	case int:
		v.Object = v.Object.(int) + int(n)
		return uint64(v.Object.(int)), nil
	case int8:
		v.Object = v.Object.(int8) + int8(n)
		return uint64(v.Object.(int8)), nil
	case int16:
		v.Object = v.Object.(int16) + int16(n)
		return uint64(v.Object.(int16)), nil
	case int32:
		v.Object = v.Object.(int32) + int32(n)
		return uint64(v.Object.(int32)), nil
	case int64:
		v.Object = v.Object.(int64) + int64(n)
		return uint64(v.Object.(int64)), nil
	case uint:
		v.Object = v.Object.(uint) + uint(n)
		return uint64(v.Object.(uint)), nil
	case uintptr:
		v.Object = v.Object.(uintptr) + uintptr(n)
		return uint64(v.Object.(uintptr)), nil
	case uint8:
		v.Object = v.Object.(uint8) + uint8(n)
		return uint64(v.Object.(uint8)), nil
	case uint16:
		v.Object = v.Object.(uint16) + uint16(n)
		return uint64(v.Object.(uint16)), nil
	case uint32:
		v.Object = v.Object.(uint32) + uint32(n)
		return uint64(v.Object.(uint32)), nil
	case uint64:
		v.Object = v.Object.(uint64) + n
		return uint64(v.Object.(uint64)), nil
	}
	return 0, fmt.Errorf("The value for %s is not an integer", k)
}

// Decrement an item of type int, int8, int16, int32, int64, uintptr, uint,
// uint8, uint32, or uint64 by n. Returns an error if the
// item's value is not an integer, if it was not found, or if it is not
// possible to decrement it by n.
// Stops at 0 on underflow.
func (c *cache) Decrement(k string, n uint64) (uint64, error) {
	// TODO: Implement Increment and Decrement more cleanly.
	// (Cannot do Increment(k, n*-1) for uints.)
	c.Lock()
	defer c.Unlock()
	v, found := c.items[k]
	if !found || v.Expired() {
		return 0, ErrCacheMiss
	}
	switch v.Object.(type) {
	case int:
		vi := v.Object.(int)
		if vi > int(n) {
			v.Object = vi - int(n)
		} else {
			v.Object = int(0)
		}
		return uint64(v.Object.(int)), nil
	case int8:
		vi := v.Object.(int8)
		if vi > int8(n) {
			v.Object = vi - int8(n)
		} else {
			v.Object = int8(0)
		}
		return uint64(v.Object.(int8)), nil
	case int16:
		vi := v.Object.(int16)
		if vi > int16(n) {
			v.Object = vi - int16(n)
		} else {
			v.Object = int16(0)
		}
		return uint64(v.Object.(int16)), nil
	case int32:
		vi := v.Object.(int32)
		if vi > int32(n) {
			v.Object = vi - int32(n)
		} else {
			v.Object = int32(0)
		}
		return uint64(v.Object.(int32)), nil
	case int64:
		vi := v.Object.(int64)
		if vi > int64(n) {
			v.Object = vi - int64(n)
		} else {
			v.Object = int64(0)
		}
		return uint64(v.Object.(int64)), nil
	case uint:
		vi := v.Object.(uint)
		if vi > uint(n) {
			v.Object = vi - uint(n)
		} else {
			v.Object = uint(0)
		}
		return uint64(v.Object.(uint)), nil
	case uintptr:
		vi := v.Object.(uintptr)
		if vi > uintptr(n) {
			v.Object = vi - uintptr(n)
		} else {
			v.Object = uintptr(0)
		}
		return uint64(v.Object.(uintptr)), nil
	case uint8:
		vi := v.Object.(uint8)
		if vi > uint8(n) {
			v.Object = vi - uint8(n)
		} else {
			v.Object = uint8(0)
		}
		return uint64(v.Object.(uint8)), nil
	case uint16:
		vi := v.Object.(uint16)
		if vi > uint16(n) {
			v.Object = vi - uint16(n)
		} else {
			v.Object = uint16(0)
		}
		return uint64(v.Object.(uint16)), nil
	case uint32:
		vi := v.Object.(uint32)
		if vi > uint32(n) {
			v.Object = vi - uint32(n)
		} else {
			v.Object = uint32(0)
		}
		return uint64(v.Object.(uint32)), nil
	case uint64:
		vi := v.Object.(uint64)
		if vi > uint64(n) {
			v.Object = vi - uint64(n)
		} else {
			v.Object = uint64(0)
		}
		return uint64(v.Object.(uint64)), nil
	}
	return 0, fmt.Errorf("The value for %s is not an integer", k)
}

// Delete an item from the cache. Does nothing if the key is not in the cache.
func (c *cache) Delete(k string) (found bool) {
	c.Lock()
	_, found = c.get(k)
	c.delete(k)
	c.Unlock()
	return
}

func (c *cache) delete(k string) {
	delete(c.items, k)
}

// Delete all expired items from the cache.
func (c *cache) DeleteExpired() {
	c.Lock()
	for k, v := range c.items {
		if v.Expired() {
			c.delete(k)
		}
	}
	c.Unlock()
}

// Write the cache's items (using Gob) to an io.Writer.
func (c *cache) Save(w io.Writer) (err error) {
	enc := gob.NewEncoder(w)

	defer func() {
		if x := recover(); x != nil {
			err = fmt.Errorf("Error registering item types with Gob library")
		}
	}()
	for _, v := range c.items {
		gob.Register(v.Object)
	}
	err = enc.Encode(&c.items)
	return
}

// Save the cache's items to the given filename, creating the file if it
// doesn't exist, and overwriting it if it does.
func (c *cache) SaveFile(fname string) error {
	fp, err := os.Create(fname)
	if err != nil {
		return err
	}
	err = c.Save(fp)
	if err != nil {
		fp.Close()
		return err
	}
	return fp.Close()
}

// Add (Gob-serialized) cache items from an io.Reader, excluding any items with
// keys that already exist in the current cache.
func (c *cache) Load(r io.Reader) error {
	dec := gob.NewDecoder(r)
	items := map[string]*item{}
	err := dec.Decode(&items)
	if err == nil {
		for k, v := range items {
			_, found := c.items[k]
			if !found {
				c.items[k] = v
			}
		}
	}
	return err
}

// Load and add cache items from the given filename, excluding any items with
// keys that already exist in the current cache.
func (c *cache) LoadFile(fname string) error {
	fp, err := os.Open(fname)
	if err != nil {
		return err
	}
	err = c.Load(fp)
	if err != nil {
		fp.Close()
		return err
	}
	return fp.Close()
}

// Delete all items from the cache.
func (c *cache) Flush() {
	c.Lock()
	c.items = map[string]*item{}
	c.Unlock()
}

type janitor struct {
	Interval time.Duration
	stop     chan bool
}

func (j *janitor) Run(c *cache) {
	j.stop = make(chan bool)
	tick := time.Tick(j.Interval)
	for {
		select {
		case <-tick:
			c.DeleteExpired()
		case <-j.stop:
			return
		}
	}
}

func stopJanitor(c *Cache) {
	c.janitor.stop <- true
}

func runJanitor(c *cache, ci time.Duration) {
	j := &janitor{
		Interval: ci,
	}
	c.janitor = j
	go j.Run(c)
}

func newCache(de time.Duration) *cache {
	if de == 0 {
		de = -1
	}
	c := &cache{
		defaultExpiration: de,
		items:             map[string]*item{},
	}
	return c
}

// Return a new cache with a given default expiration duration and cleanup
// interval. If the expiration duration is less than 1, the items in the cache
// never expire (by default), and must be deleted manually. If the cleanup
// interval is less than one, expired items are not deleted from the cache
// before their next lookup or before calling DeleteExpired.
func New(defaultExpiration, cleanupInterval time.Duration) *Cache {
	c := newCache(defaultExpiration)
	// This trick ensures that the janitor goroutine (which--granted it
	// was enabled--is running DeleteExpired on c forever) does not keep
	// the returned C object from being garbage collected. When it is
	// garbage collected, the finalizer stops the janitor goroutine, after
	// which c can be collected.
	C := &Cache{c}
	if cleanupInterval > 0 {
		runJanitor(c, cleanupInterval)
		runtime.SetFinalizer(C, stopJanitor)
	}
	return C
}

type unexportedShardedCache struct {
	*shardedCache
}

type shardedCache struct {
	m       uint32
	cs      []*cache
	janitor *shardedJanitor
}

func (sc *shardedCache) bucket(k string) *cache {
	h := fnv.New32()
	h.Write([]byte(k))
	n := binary.BigEndian.Uint32(h.Sum(nil))
	return sc.cs[n%sc.m]
}

func (sc *shardedCache) Set(k string, x interface{}, d time.Duration) {
	sc.bucket(k).Set(k, x, d)
}

func (sc *shardedCache) Add(k string, x interface{}, d time.Duration) error {
	return sc.bucket(k).Add(k, x, d)
}

func (sc *shardedCache) Replace(k string, x interface{}, d time.Duration) error {
	return sc.bucket(k).Replace(k, x, d)
}

func (sc *shardedCache) Get(k string) (interface{}, bool) {
	return sc.bucket(k).Get(k)
}

func (sc *shardedCache) Increment(k string, n uint64) (uint64, error) {
	return sc.bucket(k).Increment(k, n)
}

func (sc *shardedCache) IncrementFloat(k string, n float64) error {
	return sc.bucket(k).IncrementFloat(k, n)
}

func (sc *shardedCache) Decrement(k string, n uint64) (uint64, error) {
	return sc.bucket(k).Decrement(k, n)
}

func (sc *shardedCache) Delete(k string) {
	sc.bucket(k).Delete(k)
}

func (sc *shardedCache) DeleteExpired() {
	for _, v := range sc.cs {
		v.DeleteExpired()
	}
}

func (sc *shardedCache) Flush() {
	for _, v := range sc.cs {
		v.Flush()
	}
}

type shardedJanitor struct {
	Interval time.Duration
	stop     chan bool
}

func (j *shardedJanitor) Run(sc *shardedCache) {
	j.stop = make(chan bool)
	tick := time.Tick(j.Interval)
	for {
		select {
		case <-tick:
			sc.DeleteExpired()
		case <-j.stop:
			return
		}
	}
}

func stopShardedJanitor(sc *unexportedShardedCache) {
	sc.janitor.stop <- true
}

func runShardedJanitor(sc *shardedCache, ci time.Duration) {
	j := &shardedJanitor{
		Interval: ci,
	}
	sc.janitor = j
	go j.Run(sc)
}

func newShardedCache(n int, de time.Duration) *shardedCache {
	sc := &shardedCache{
		m:  uint32(n - 1),
		cs: make([]*cache, n),
	}
	for i := 0; i < n; i++ {
		c := &cache{
			defaultExpiration: de,
			items:             map[string]*item{},
		}
		sc.cs[i] = c
	}
	return sc
}

func unexportedNewSharded(shards int, defaultExpiration, cleanupInterval time.Duration) *unexportedShardedCache {
	if defaultExpiration == 0 {
		defaultExpiration = -1
	}
	sc := newShardedCache(shards, defaultExpiration)
	SC := &unexportedShardedCache{sc}
	if cleanupInterval > 0 {
		runShardedJanitor(sc, cleanupInterval)
		runtime.SetFinalizer(SC, stopShardedJanitor)
	}
	return SC
}
