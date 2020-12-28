// Copyright (C) 2017-2018 Pilosa Corp. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package roaring

type sliceContainers struct {
	keys          []uint64
	containers    []*Container
	lastKey       uint64
	lastContainer *Container

	containersPool containersPool
}

// ContainerPoolingConfiguration represents the configuration for
// container pooling.
type ContainerPoolingConfiguration struct {
	// Maximum size of the allocated array that will be maintained in the pool.
	MaxArraySize int
	// Whether a bitmap should be allocated for each pooled container.
	AllocateBitmap bool
	// Maximum size of the allocated runs that will be maintained in the pool.
	MaxRunsSize int

	// Maximum number of containers to pool.
	MaxCapacity int
	// Maximum size of keys and containers slice to maintain after calls to Reset().
	MaxKeysAndContainersSliceLength int
}

// NewDefaultContainerPoolingConfiguration creates a ContainerPoolingConfiguration
// with default configuration.
func NewDefaultContainerPoolingConfiguration(maxCapacity int) ContainerPoolingConfiguration {
	return ContainerPoolingConfiguration{
		MaxArraySize:   ArrayMaxSize,
		MaxRunsSize:    runMaxSize,
		AllocateBitmap: true,

		MaxCapacity: maxCapacity,

		// Compared to the size of the containers themselves, these slices are
		// very small (8 bytes per entry), so we can afford to allow them to
		// grow larger.
		MaxKeysAndContainersSliceLength: maxCapacity * 10,
	}
}

type containersPool struct {
	containers []*Container // Nil if pooling disabled, otherwise present.
	config     ContainerPoolingConfiguration
}

func (cp *containersPool) put(c *Container) {
	if cp == nil || cp.containers == nil {
		// Ignore if pooling isn't configured.
		return
	}

	if c.mapped {
		// If the container was mapped, assume the whole thing has been
		// corrupted and reset it to an initial state (without reallocating
		// according to the pooling config as its likely it will just be
		// mapped again.)
		*c = newContainer()
		cp.containers = append(cp.containers, c)
		return
	}

	if len(cp.containers) > cp.config.MaxCapacity {
		// Don't allow pool to exceed maximum capacity.
		return
	}

	if cap(c.array) > cp.config.MaxArraySize {
		// Don't allow any containers with an allocated array slice to be
		// returned to the pool if the config doesn't allow it.
		c.array = nil
	}

	if cap(c.runs) > cp.config.MaxRunsSize {
		// Don't allow any containers with an allocated run slice to be
		// returned to the pool if the config doesn't allow it.
		c.runs = nil
	}

	if !cp.config.AllocateBitmap {
		c.bitmap = nil
	}

	// Reset before returning to the pool to ensure all calls to get() return
	// a clean container.
	c.Reset()
	cp.containers = append(cp.containers, c)
}

func (cp *containersPool) get() *Container {
	if cp == nil || cp.containers == nil {
		return NewContainer()
	}

	if len(cp.containers) > 0 {
		// If we have a pooled container available, use that.
		lastIdx := len(cp.containers) - 1
		c := cp.containers[lastIdx]
		cp.containers = cp.containers[:lastIdx]
		return c
	}

	// Pooling is enabled, but there are no available containers,
	// so we allocate.
	return NewContainerWithPooling(cp.config)
}

func newSliceContainers() *sliceContainers {
	return &sliceContainers{}
}

func newSliceContainersWithPooling(poolingConfig ContainerPoolingConfiguration) *sliceContainers {
	sc := &sliceContainers{
		keys:       make([]uint64, 0, poolingConfig.MaxCapacity),
		containers: make([]*Container, 0, poolingConfig.MaxCapacity),
	}

	sc.containersPool = containersPool{
		config:     poolingConfig,
		containers: make([]*Container, 0, poolingConfig.MaxCapacity),
	}
	for i := 0; i < poolingConfig.MaxCapacity; i++ {
		sc.containersPool.put(NewContainerWithPooling(poolingConfig))
	}

	return sc
}

func (sc *sliceContainers) Get(key uint64) *Container {
	i := search64(sc.keys, key)
	if i < 0 {
		return nil
	}
	return sc.containers[i]
}

func (sc *sliceContainers) Put(key uint64, c *Container) {
	i := search64(sc.keys, key)

	// If index is negative then there's not an exact match
	// and a container needs to be added.
	if i < 0 {
		sc.insertAt(key, c, -i-1)
	} else {
		sc.containers[i] = c
	}

}

func (sc *sliceContainers) PutContainerValues(key uint64, containerType byte, n int, mapped bool) {
	i := search64(sc.keys, key)
	if i < 0 {
		c := sc.containersPool.get()
		c.containerType = containerType
		c.n = int32(n)
		c.mapped = mapped
		sc.insertAt(key, c, -i-1)
	} else {
		c := sc.containers[i]
		c.containerType = containerType
		c.n = int32(n)
		c.mapped = mapped
	}

}

func (sc *sliceContainers) Remove(key uint64) {
	statsHit("sliceContainers/Remove")
	i := search64(sc.keys, key)
	if i < 0 {
		return
	}

	sc.keys = append(sc.keys[:i], sc.keys[i+1:]...)
	sc.containersPool.put(sc.containers[i])
	sc.containers = append(sc.containers[:i], sc.containers[i+1:]...)

}
func (sc *sliceContainers) insertAt(key uint64, c *Container, i int) {
	statsHit("sliceContainers/insertAt")
	sc.keys = append(sc.keys, 0)
	copy(sc.keys[i+1:], sc.keys[i:])
	sc.keys[i] = key

	sc.containers = append(sc.containers, nil)
	copy(sc.containers[i+1:], sc.containers[i:])
	sc.containers[i] = c
}

func (sc *sliceContainers) GetOrCreate(key uint64) *Container {
	// Check the last* cache for same container.
	if key == sc.lastKey && sc.lastContainer != nil {
		return sc.lastContainer
	}

	sc.lastKey = key
	i := search64(sc.keys, key)
	if i < 0 {
		c := sc.containersPool.get()
		sc.insertAt(key, c, -i-1)
		sc.lastContainer = c
		return c
	}

	sc.lastContainer = sc.containers[i]
	return sc.lastContainer
}

func (sc *sliceContainers) Clone() Containers {
	var other *sliceContainers
	if sc.containersPool.containers != nil {
		other = newSliceContainersWithPooling(sc.containersPool.config)
	} else {
		other = newSliceContainers()
	}

	if cap(other.keys) > len(sc.keys) {
		other.keys = other.keys[:len(sc.keys)]
	} else {
		other.keys = make([]uint64, len(sc.keys))
	}

	other.containers = make([]*Container, len(sc.containers))
	copy(other.keys, sc.keys)
	for i, c := range sc.containers {
		// TODO(rartoul): It would be more efficient to use one of
		// other's pooled containers instead of alllowing Clone to
		// allocate a new one when pooling is enabled.
		other.containers[i] = c.Clone()
	}
	return other
}

func (sc *sliceContainers) Last() (key uint64, c *Container) {
	if len(sc.keys) == 0 {
		return 0, nil
	}
	return sc.keys[len(sc.keys)-1], sc.containers[len(sc.keys)-1]
}

func (sc *sliceContainers) Size() int {
	return len(sc.keys)

}

func (sc *sliceContainers) Count() uint64 {
	n := uint64(0)
	for i := range sc.containers {
		n += uint64(sc.containers[i].n)
	}
	return n
}

func (sc *sliceContainers) Reset() {
	for i := range sc.containers {
		// Try and return containers to the pool (no-op if disabled.)
		sc.containersPool.put(sc.containers[i])
		// Clear pointers to allow G.C to reclaim objects if these were the
		// only outstanding pointers.
		sc.containers[i] = nil
	}

	if sc.poolingEnabled() {
		if cap(sc.keys) <= sc.containersPool.config.MaxKeysAndContainersSliceLength {
			sc.keys = sc.keys[:0]
		} else {
			sc.keys = make([]uint64, 0, sc.containersPool.config.MaxCapacity)
		}

		if cap(sc.containers) <= sc.containersPool.config.MaxKeysAndContainersSliceLength {
			sc.containers = sc.containers[:0]
		} else {
			sc.containers = make([]*Container, 0, sc.containersPool.config.MaxCapacity)
		}
	} else {
		sc.keys = sc.keys[:0]
		sc.containers = sc.containers[:0]
	}
	sc.lastContainer = nil
	sc.lastKey = 0
}

func (sc *sliceContainers) poolingEnabled() bool {
	return sc.containersPool.containers != nil
}

func (sc *sliceContainers) seek(key uint64) (int, bool) {
	i := search64(sc.keys, key)
	found := true
	if i < 0 {
		found = false
		i = -i - 1
	}
	return i, found
}

func (sc *sliceContainers) Iterator(key uint64) (citer ContainerIterator, found bool) {
	i, found := sc.seek(key)
	return &sliceIterator{e: sc, i: i}, found
}

func (sc *sliceContainers) Repair() {
	for _, c := range sc.containers {
		c.Repair()
	}
}

type sliceIterator struct {
	e     *sliceContainers
	i     int
	key   uint64
	value *Container
}

func (si *sliceIterator) Next() bool {
	if si.e == nil || si.i > len(si.e.keys)-1 {
		return false
	}
	si.key = si.e.keys[si.i]
	si.value = si.e.containers[si.i]
	si.i++

	return true
}

func (si *sliceIterator) Value() (uint64, *Container) {
	return si.key, si.value
}

type stackContainerIterator struct {
	// if heapIter is set, then we are falling back to the heap allocated iter
	// since the containers weren't using a slice of containers
	heapIter ContainerIterator

	e     *sliceContainers
	i     int
	key   uint64
	value *Container
}

func stackContainerIteratorFromContainers(
	key uint64,
	containers Containers,
) (iter stackContainerIterator, found bool) {
	// This falls back to just using a heap allocated iterator
	var heapIter ContainerIterator
	heapIter, found = containers.Iterator(key)
	iter = stackContainerIterator{heapIter: heapIter}
	return
}

func stackContainerIteratorFromSliceContainers(
	key uint64,
	sc *sliceContainers,
) (iter stackContainerIterator, found bool) {
	var i int
	i, found = sc.seek(key)
	iter = stackContainerIterator{e: sc, i: i}
	return
}

func (si *stackContainerIterator) Next() bool {
	if si.heapIter != nil {
		return si.heapIter.Next()
	}

	if si.e == nil || si.i > len(si.e.keys)-1 {
		return false
	}
	si.key = si.e.keys[si.i]
	si.value = si.e.containers[si.i]
	si.i++
	return true
}

func (si *stackContainerIterator) Value() (uint64, *Container) {
	if si.heapIter != nil {
		return si.heapIter.Value()
	}

	return si.key, si.value
}
