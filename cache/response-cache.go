package cache

import "sync"

type ResponseCache struct {
	mu         sync.RWMutex
	nonStreams map[string][]byte
	streams    map[string]*StreamResponse
}

func NewResponseCache() *ResponseCache {
	return &ResponseCache{
		nonStreams: make(map[string][]byte),
		streams:    make(map[string]*StreamResponse),
	}
}

// Set stores a non-streaming response in the cache.
func (c *ResponseCache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nonStreams[key] = value
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
func (c *ResponseCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, exists := c.nonStreams[key]
	return val, exists
}

// GetStream retrieves a streaming response from the cache.
func (c *ResponseCache) GetStream(key string) (*StreamResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	stream, exists := c.streams[key]
	return stream, exists
}
