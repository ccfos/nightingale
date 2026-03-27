package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ClientCache caches LLM clients keyed by config fingerprint,
// so that requests with the same LLM configuration share the
// underlying http.Transport connection pool.
//
// Entries that are not accessed within the TTL are automatically
// evicted by a background goroutine to prevent memory leaks when
// users frequently rotate API keys or change LLM configurations.
type ClientCache struct {
	clients sync.Map // fingerprint -> *cacheItem
}

// cacheItem wraps an LLM client with its last-access timestamp.
type cacheItem struct {
	client   LLM
	accessed atomic.Int64 // unix seconds, updated on every GetOrCreate hit
}

const defaultTTL = 30 * time.Minute

// NewClientCache creates a new ClientCache and starts a background
// goroutine that evicts entries not accessed within the TTL.
func NewClientCache() *ClientCache {
	c := &ClientCache{}
	go c.evictLoop(defaultTTL)
	return c
}

// GetOrCreate returns a cached LLM client for the given config,
// or creates and caches a new one if no match exists.
func (c *ClientCache) GetOrCreate(cfg *Config) (LLM, error) {
	key := c.fingerprint(cfg)
	now := time.Now().Unix()

	if v, ok := c.clients.Load(key); ok {
		item := v.(*cacheItem)
		item.accessed.Store(now)
		return item.client, nil
	}

	client, err := New(cfg)
	if err != nil {
		return nil, err
	}

	item := &cacheItem{client: client}
	item.accessed.Store(now)
	actual, _ := c.clients.LoadOrStore(key, item)
	return actual.(*cacheItem).client, nil
}

// evictLoop periodically scans the cache and removes entries that
// have not been accessed within the given TTL.
func (c *ClientCache) evictLoop(ttl time.Duration) {
	ticker := time.NewTicker(ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-ttl).Unix()
		c.clients.Range(func(key, value any) bool {
			item := value.(*cacheItem)
			if item.accessed.Load() < cutoff {
				c.clients.Delete(key)
			}
			return true
		})
	}
}

// fingerprint computes a hash over all Config fields that affect
// the LLM client or its underlying http.Client / http.Transport.
//
// All Config fields are included in the fingerprint.
func (c *ClientCache) fingerprint(cfg *Config) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|%s|%v|%s|%d",
		cfg.Provider,
		cfg.BaseURL,
		cfg.APIKey,
		cfg.Model,
		cfg.SkipSSLVerify,
		cfg.Proxy,
		cfg.Timeout,
	)

	if cfg.Temperature != nil {
		fmt.Fprintf(h, "|t=%g", *cfg.Temperature)
	}
	if cfg.MaxTokens != nil {
		fmt.Fprintf(h, "|m=%d", *cfg.MaxTokens)
	}

	// Headers are baked into the LLM struct and sent with every request,
	// so they must be part of the fingerprint. Sort keys for determinism.
	if len(cfg.Headers) > 0 {
		keys := make([]string, 0, len(cfg.Headers))
		for k := range cfg.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(h, "|%s=%s", k, cfg.Headers[k])
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}
