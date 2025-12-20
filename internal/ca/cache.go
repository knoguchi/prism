package ca

import (
	"container/list"
	"crypto/tls"
	"sync"
)

// CertCache is an LRU cache for TLS certificates
type CertCache struct {
	mu       sync.RWMutex
	capacity int
	cache    map[string]*list.Element
	lru      *list.List
}

type cacheEntry struct {
	host string
	cert *tls.Certificate
}

// NewCertCache creates a new certificate cache with the given capacity
func NewCertCache(capacity int) *CertCache {
	return &CertCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element),
		lru:      list.New(),
	}
}

// Get retrieves a certificate from the cache
func (c *CertCache) Get(host string) *tls.Certificate {
	c.mu.RLock()
	elem, ok := c.cache[host]
	c.mu.RUnlock()

	if !ok {
		return nil
	}

	// Move to front (most recently used)
	c.mu.Lock()
	c.lru.MoveToFront(elem)
	c.mu.Unlock()

	return elem.Value.(*cacheEntry).cert
}

// Put adds a certificate to the cache
func (c *CertCache) Put(host string, cert *tls.Certificate) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists
	if elem, ok := c.cache[host]; ok {
		c.lru.MoveToFront(elem)
		elem.Value.(*cacheEntry).cert = cert
		return
	}

	// Evict oldest if at capacity
	if c.lru.Len() >= c.capacity {
		oldest := c.lru.Back()
		if oldest != nil {
			c.lru.Remove(oldest)
			delete(c.cache, oldest.Value.(*cacheEntry).host)
		}
	}

	// Add new entry
	entry := &cacheEntry{host: host, cert: cert}
	elem := c.lru.PushFront(entry)
	c.cache[host] = elem
}

// Remove removes a certificate from the cache
func (c *CertCache) Remove(host string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[host]; ok {
		c.lru.Remove(elem)
		delete(c.cache, host)
	}
}

// Clear removes all certificates from the cache
func (c *CertCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*list.Element)
	c.lru = list.New()
}

// Size returns the current number of cached certificates
func (c *CertCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lru.Len()
}

// Hosts returns a list of all cached hosts
func (c *CertCache) Hosts() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	hosts := make([]string, 0, c.lru.Len())
	for host := range c.cache {
		hosts = append(hosts, host)
	}
	return hosts
}
