package cache

import (
	"net/http"
	"sync"
)

type cacheEntry struct {
	Body    []byte
	Headers http.Header
}

type ResponseCache struct {
	mu         sync.RWMutex
	nonStreams map[string]cacheEntry
	streams    map[string]*StreamResponse
}

func NewResponseCache() *ResponseCache {
	return &ResponseCache{
		nonStreams: make(map[string]cacheEntry),
		streams:    make(map[string]*StreamResponse),
	}
}

// Set stores a non-streaming response in the cache.
func (c *ResponseCache) Set(key string, value []byte, headers http.Header) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nonStreams[key] = cacheEntry{Body: value, Headers: headers}
}

// SetStream initializes a streaming response in the cache.
func (c *ResponseCache) SetStream(key string, bufferSize int) *StreamResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	stream := NewStreamResponse(bufferSize)
	c.streams[key] = stream
	return stream
}

// Get retrieves a non-streaming response from the cache.
func (c *ResponseCache) Get(key string) (cacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, exists := c.nonStreams[key]
	if exists {
		return entry, true
	}
	return cacheEntry{}, false
}

// GetStream retrieves a streaming response from the cache.
func (c *ResponseCache) GetStream(key string) (*StreamResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	stream, exists := c.streams[key]
	return stream, exists
}
