package memsto

import (
	"sync"
	"time"
)

// LRUNode represents a node in the doubly linked list
type LRUNode struct {
	Key       uint64
	Timestamp int64
	Prev      *LRUNode
	Next      *LRUNode
}

// AlertStatusLRUCache implements a thread-safe LRU cache for check repeat alert status
type AlertStatusLRUCache struct {
	capacity int
	size     int
	cache    map[uint64]*LRUNode
	head     *LRUNode
	tail     *LRUNode
	mutex    sync.RWMutex
}

// NewAlertStatusLRUCache creates a new LRU cache with the specified capacity
func NewAlertStatusLRUCache(capacity int) *AlertStatusLRUCache {
	cache := &AlertStatusLRUCache{
		capacity: capacity,
		cache:    make(map[uint64]*LRUNode),
		head:     &LRUNode{},
		tail:     &LRUNode{},
		mutex:    sync.RWMutex{},
	}

	// Initialize dummy head and tail nodes
	cache.head.Next = cache.tail
	cache.tail.Prev = cache.head

	return cache
}

func (lru *AlertStatusLRUCache) checkNodeExpired(node *LRUNode) bool {
	return node.Timestamp < time.Now().Unix()-86400
}

// addNode adds a node right after head
func (lru *AlertStatusLRUCache) addNode(node *LRUNode) {
	node.Prev = lru.head
	node.Next = lru.head.Next

	lru.head.Next.Prev = node
	lru.head.Next = node
}

// removeNode removes an existing node from the linked list
func (lru *AlertStatusLRUCache) removeNode(node *LRUNode) {
	prevNode := node.Prev
	newNode := node.Next

	prevNode.Next = newNode
	newNode.Prev = prevNode
}

// moveToHead moves a node to the head and update timestamp
func (lru *AlertStatusLRUCache) moveToHead(node *LRUNode) {
	lru.removeNode(node)
	node.Timestamp = time.Now().Unix()
	lru.addNode(node)
}

// popTail remove tail node if expired
func (lru *AlertStatusLRUCache) popTail() *LRUNode {
	if lru.checkNodeExpired(lru.tail.Prev) {
		lastNode := lru.tail.Prev
		lru.removeNode(lastNode)
		return lastNode
	}
	return nil
}

// Put adds or updates an entry in the cache and show success or not
func (lru *AlertStatusLRUCache) Put(key uint64) bool {
	lru.mutex.Lock()
	defer lru.mutex.Unlock()

	if node, exists := lru.cache[key]; exists {
		// Update existing node
		lru.moveToHead(node)
		return true
	}
	if lru.size == lru.capacity {
		// Remove the least recently used node
		if tail := lru.popTail(); tail != nil {
			delete(lru.cache, tail.Key)
			lru.size--
		} else {
			return false
		}
	}

	// Add new node
	newNode := &LRUNode{
		Key:       key,
		Timestamp: time.Now().Unix(),
	}

	lru.cache[key] = newNode
	lru.addNode(newNode)
	lru.size++

	return true
}
