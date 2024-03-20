package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"

	tiktoken "github.com/pkoukk/tiktoken-go"
)

type OpenAIProvider struct {
	endpoint string
	models   []string
}

func parseMessagesContent(req *http.Request) ([]string, error) {
	var payload struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}

	// Decode the request body into the payload struct
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	err = json.Unmarshal(bodyBytes, &payload)
	if err != nil {
		return nil, err
	}

	// Extract the content from each message
	var contents []string
	for _, message := range payload.Messages {
		contents = append(contents, message.Content)
	}

	return contents, nil
}

func (o *OpenAIProvider) GetEndpoint() string {
	return o.endpoint
}

func (o *OpenAIProvider) GetModels() []string {
	return o.models
}

func (o *OpenAIProvider) CountTokens(req *http.Request, model string) (int, error) {
	content, err := parseMessagesContent(req)
	if err != nil {
		log.Println("Error parsing messages content:", err)
		return 0, err
	}

	// parseMessagesContent extracts the "content" field from each message in the "messages" array.
	tkm, err := tiktoken.EncodingForModel(model)
	if err != nil {
		err = fmt.Errorf("getEncoding: %v", err)
		return 0, err
	}

	token := tkm.Encode(strings.Join(content, " "), nil, nil)

	return len(token), nil
}

type IModelProvider interface {
	GetEndpoint() string
	GetModels() []string
	CountTokens(req *http.Request, model string) (int, error)
}

type ServiceResolver struct {
	mu        sync.RWMutex
	providers map[string]IModelProvider
}

func NewServiceResolver() *ServiceResolver {
	svc := &ServiceResolver{
		providers: make(map[string]IModelProvider),
	}

	openai := &OpenAIProvider{
		endpoint: "https://api.openai.com",
		models: []string{
			"gpt-3.5-turbo-1106",
			"text-embedding-3-large",
			"tts-1-hd-1106",
			"tts-1-hd",
			"gpt-4-0314",
			"gpt-3.5-turbo",
			"gpt-4-32k-0314",
			"gpt-3.5-turbo-0125",
			"gpt-4-0613",
			"gpt-3.5-turbo-0301",
			"gpt-3.5-turbo-0613",
			"gpt-3.5-turbo-instruct-0914",
			"gpt-3.5-turbo-16k-0613",
			"gpt-4",
			"tts-1",
			"davinci-002",
			"gpt-4-vision-preview",
			"gpt-3.5-turbo-instruct",
			"babbage-002",
			"tts-1-1106",
			"gpt-3.5-turbo-16k",
			"gpt-4-0125-preview",
			"gpt-4-turbo-preview",
			"gpt-4-1106-preview",
			"text-embedding-ada-002",
			"text-embedding-3-small",
		},
	}
	svc.Register(openai)
	return svc
}

func (r *ServiceResolver) GetReverseProxy(req *http.Request) (*httputil.ReverseProxy, error) {
	var requestBodyBuffer bytes.Buffer
	tee := io.TeeReader(req.Body, &requestBodyBuffer)
	decoder := json.NewDecoder(tee)
	var requestBody map[string]interface{}
	err := decoder.Decode(&requestBody)
	if err != nil {
		return nil, err
	}
	req.Body.Close()

	model, ok := requestBody["model"].(string)
	req.Body = io.NopCloser(&requestBodyBuffer)

	if !ok {
		return nil, errors.New("Model not specified in the request")
	}

	provider, found := r.Resolve(model)
	if !found {
		return nil, errors.New("Service not found for the specified model")
	}

	targetURL := provider.GetEndpoint()

	log.Println("Targeting model", model, "at", targetURL)
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
		tokensCount, err := provider.CountTokens(req, model)
		if err != nil {
			log.Println("Error counting tokens:", err)
		}
		log.Println("Proxying request to", req.URL.String(), "with", tokensCount, "tokens")
	}

	return proxy, nil
}

func (r *ServiceResolver) Resolve(modelName string) (IModelProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, exists := r.providers[modelName]
	return provider, exists
}

func (r *ServiceResolver) Register(provider IModelProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, modelName := range provider.GetModels() {
		log.Println("Registering", modelName)
		r.providers[modelName] = provider
	}
}

type ResponseCache struct {
	mu    sync.RWMutex
	store map[string][]byte
}

// Set stores a response in the cache.
func (c *ResponseCache) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = value
}

// Get retrieves a response from the cache.
func (c *ResponseCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, exists := c.store[key]
	return val, exists
}

// NewResponseCache creates a new ResponseCache.
func NewResponseCache() *ResponseCache {
	return &ResponseCache{
		store: make(map[string][]byte),
	}
}

func main() {
	cache := NewResponseCache()

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

		if val, found := cache.Get(cacheKey); found {
			log.Println("Cache hit")
			w.Write(val) // Return the cached response
			return
		}
		log.Println("Cache miss")

		// Capture the response
		rec := httptest.NewRecorder()
		proxy.ServeHTTP(rec, r)

		responseBody, err := io.ReadAll(rec.Body)
		if err != nil {
			w.WriteHeader(rec.Code)
			log.Println("Returning response")
			w.Write(responseBody)
			w.Header().Set("model-proxy-cache", "hit")
			return
		}

		// Cache the response if the status code indicates success
		if rec.Code >= 200 && rec.Code < 300 {
			log.Println("Caching response")
			cache.Set(cacheKey, responseBody)
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
