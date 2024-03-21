package cache

import "sync"

type StreamResponse struct {
	dataChan chan []byte
	closed   bool
	sync.Mutex
}

func NewStreamResponse(bufferSize int) *StreamResponse {
	return &StreamResponse{
		dataChan: make(chan []byte, bufferSize),
		closed:   false,
	}
}

func (sr *StreamResponse) WriteChunk(chunk []byte) {
	sr.Lock()
	defer sr.Unlock()

	if sr.closed {
		return
	}

	sr.dataChan <- chunk
}

func (sr *StreamResponse) Close() {
	sr.Lock()
	defer sr.Unlock()

	if !sr.closed {
		close(sr.dataChan)
		sr.closed = true
	}
}

func (sr *StreamResponse) ReadChunks() <-chan []byte {
	return sr.dataChan
}
