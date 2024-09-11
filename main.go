package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"sync"

	"github.com/andybalholm/brotli"
	cache "github.com/jsfour/model-proxy/cache"
	provider "github.com/jsfour/model-proxy/providers"
)

type sseTransport struct {
	Transport http.RoundTripper
}

func (t *sseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Make the actual round trip call using the original transport.
	resp, err := t.Transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// Check if this is an event stream.
	if req.Header.Get("Accept") == "text/event-stream" {
		// Make a flushing response writer if needed.
		flusher, ok := resp.Body.(http.Flusher)
		if !ok {
			return nil, io.EOF
		}

		// Now wrap the response body in our streaming reader.
		resp.Body = &sseReader{
			reader:  resp.Body,
			flusher: flusher,
		}
	}

	// Pass the response up the chain.
	return resp, nil
}

type sseReader struct {
	reader  io.ReadCloser
	flusher http.Flusher
	buffer  bytes.Buffer
}

func (r *sseReader) Read(p []byte) (n int, err error) {
	// Read data from the original stream.
	n, err = r.reader.Read(p)

	// If this was a successful read, flush the data.
	if n > 0 {
		if flusher, ok := r.flusher.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	return n, err
}

func (r *sseReader) Close() error {
	return r.reader.Close()
}

type ServiceResolver struct {
	mu        sync.RWMutex
	providers map[string]provider.IModelProvider
}

func NewServiceResolver() *ServiceResolver {
	svc := &ServiceResolver{
		providers: make(map[string]provider.IModelProvider),
	}

	openai := provider.NewOpenAIProvider()

	svc.Register(openai)
	return svc
}

func (r *ServiceResolver) GetReverseProxy(req *http.Request) (*httputil.ReverseProxy, error) {
	var requestBody struct {
		Model  string `json:"model,omitempty"`
		Stream bool   `json:"stream,omitempty"`
	}
	var requestBodyBuffer bytes.Buffer
	tee := io.TeeReader(req.Body, &requestBodyBuffer)
	err := json.NewDecoder(tee).Decode(&requestBody)
	if err != nil {
		return nil, err
	}
	req.Body.Close()

	req.Body = io.NopCloser(&requestBodyBuffer)

	if requestBody.Model == "" {
		return nil, errors.New("Model not specified in the request")
	}

	provider, found := r.Resolve(requestBody.Model)
	if !found {
		return nil, errors.New("Service not found for the specified model")
	}

	targetURL := provider.GetEndpoint()

	log.Println("Targeting model", requestBody.Model, "at", targetURL)
	target, err := url.Parse(targetURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = target.Host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		tokensCount, err := provider.CountTokens(req, requestBody.Model)
		if err != nil {
			log.Println("Error counting tokens:", err)
		}
		log.Println("Proxying request to", req.URL.String(), "with", tokensCount, "tokens")
	}

	// if requestBody.Stream {
	originalTransport := proxy.Transport
	if originalTransport == nil {
		originalTransport = http.DefaultTransport
	}
	proxy.Transport = &sseTransport{
		Transport: originalTransport,
	}
	// }

	return proxy, nil
}

func (r *ServiceResolver) Resolve(modelName string) (provider.IModelProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, exists := r.providers[modelName]
	return provider, exists
}

func (r *ServiceResolver) Register(provider provider.IModelProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, modelName := range provider.GetModels() {
		log.Println("Registering", modelName)
		r.providers[modelName] = provider
	}
}

func main() {
	// RunLlama()
	cache := cache.NewResponseCache()

	resolver := NewServiceResolver()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		proxy, err := resolver.GetReverseProxy(r)

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Println("Request received")
		cacheKey, err := generateCacheKey(r)
		if err != nil {
			http.Error(w, "Failed to generate cache key", http.StatusInternalServerError)
			return
		}

		if cacheEntry, found := cache.Get(cacheKey); found {
			log.Printf("Cache hit: %d bytes", len(cacheEntry.Body))

			w.Header().Set("model-proxy-cache", "hit")

			// Decompress Brotli-encoded content
			reader := brotli.NewReader(bytes.NewReader(cacheEntry.Body))
			decompressed, err := io.ReadAll(reader)
			if err != nil {
				log.Printf("Error decompressing Brotli content: %v", err)
				// Handle error appropriately
			} else {
				log.Printf("Decompressed cache content: %s", string(decompressed))
			}

			// Write the original (compressed) content to the response
			// Set cached headers from cacheEntry.headers
			for key, values := range cacheEntry.Headers {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}

			w.Write(cacheEntry.Body)
			return
		}

		if streamCache, found := cache.GetStream(cacheKey); found {
			log.Println("Cache hit stream")
			w.Header().Set("model-proxy-cache", "hit")

			// Since we have a streaming response, we will read from the stream's channel
			for chunk := range streamCache.ReadChunks() {
				_, writeErr := w.Write(chunk)
				if writeErr != nil {
					// If an error occurs while writing to the response writer,
					// we can log the error and break out of the loop.
					log.Printf("Error writing chunk to response: %v\n", writeErr)
					break
				}
				// Optional: Flush the response writer if it supports flushing, to send chunks to the client as they're written
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
			}

			// Once done with the loop, it means the stream has been closed and all chunks are written.
			return
		}

		// play back response
		log.Println("Cache miss")

		// Capture the response
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, r)

		responseBody, err := io.ReadAll(rec.Body)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Encoding", "br")
			w.Header().Set("model-proxy-cache", "miss")
			w.WriteHeader(rec.Code)
			log.Println("Error reading response returning response")
			log.Println(err)
			w.Write(responseBody)
			return
		}

		// Cache the response if the status code indicates success
		if rec.Code >= 200 && rec.Code < 300 {
			log.Printf("Caching response of size %d bytes", len(responseBody))
			// Create headers with application type and content encoding if they exist
			headers := http.Header{}
			if contentType := rec.Header().Get("Content-Type"); contentType != "" {
				headers.Set("Content-Type", contentType)
			}
			if contentEncoding := rec.Header().Get("Content-Encoding"); contentEncoding != "" {
				headers.Set("Content-Encoding", contentEncoding)
			}
			cache.Set(cacheKey, responseBody, headers)
		}

		// Copy the captured response to the actual response
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}

		w.WriteHeader(rec.Code)
		log.Println("Returning response")
		w.Write(responseBody)
	})

	log.Println("Loading on localhost:8080")
	log.Println("Server is running...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// generateCacheKey creates a unique string based on the request's path, body, and headers.
func generateCacheKey(r *http.Request) (string, error) {
	// Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	// Restore the body so it can be read again
	r.Body = io.NopCloser(bytes.NewReader(body))

	// Create a hash
	hash := sha256.New()
	if bearerToken := r.Header.Get("Authorization"); bearerToken != "" {
		hash.Write([]byte(bearerToken))
	}
	hash.Write([]byte(r.URL.Path))
	hash.Write(body)
	return hex.EncodeToString(hash.Sum(nil)), nil
}
