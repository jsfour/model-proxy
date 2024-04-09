# Golang Model Proxy Cache Service
### Develop against large language models without the big bill

This application serves as a reverse proxy with caching capabilities, specifically tailored for language model model API requests. Built with Golang, it facilitates interactions with models hosted on platforms like OpenAI by caching responses and minimizing redundant external API calls.

The goal is to allow for you to develop against llm api's without running up a bill.

## Features

- **Reverse Proxy Functionality:** Directs API requests to the appropriate machine learning model service provider. **Currently only supports OpenAI.**
- **Caching Mechanism:** Stores successful responses to reduce API calls and improve performance. Cache hits serve responses directly from the cache.
- **Token Counting:** Leveraging `tiktoken-go`, the service estimates the number of tokens in each request's payload to keep track of usage.
- **Dynamic Service Resolution:** Looks up the configured model service provider based on the requested model in the API call.
- **Extensibility:** Supports registering multiple model providers through the `IModelProvider` interface, each with its own set of API models and endpoints.

## How It Works

The entry point `main()` initiates the application by setting up a response cache and a service resolver that includes an `OpenAIProvider` responsible for handling OpenAI API requests. The HTTP server listens on port `8080` and processes incoming requests through a handler which:

1. Generates a cache key based on the request's path, body, and header.
2. Checks the cache for a stored response corresponding to the cache key.
3. If a cache hit occurs, it serves the response directly from the cache.
4. On a cache miss, it determines the correct service provider and reverse proxies the request to the target machine learning model API.

## API

The application exposes a single HTTP endpoint `/` that accepts requests with model specifications in the body. Examples of supported model names include "gpt-4", "gpt-3.5-turbo", and "text-embedding-3-large".

## Setup

To run the service, ensure you have the following:

- Golang installed and configured.
- `tiktoken-go` library installed (`go get github.com/pkoukk/tiktoken-go`).

To start the server, execute:

```sh
go run main.go
```

The server will listen on `http://localhost:8080`.

## Installation

Download the repository and install the dependencies:

```sh
go get
```

Then install via:

```sh
go install
```

## Usage

Make an API request to the service with the desired machine learning model name and payload:

```sh
curl -X POST http://localhost:8080/v1/chat/completions -H "Content-Type: application/json" \
    -d '{"model": "gpt-3.5-turbo", "messages": [{"role": "user", "content": "What is the capital of France?"}]}'
```

Replace the `model` value and the `messages` array content with your specific requirements.

### Nodejs

You can drop in the proxt via the

```typescript
const llm = new OpenAI({
  baseURL: "http://localhost:8080/v1",
});

const res = await llm.chat.completions.create({
  messages: [
    { role: "system", content: "You are a helpful assistant." },
    { role: "user", content: "Hello!" },
  ],
  model: "gpt-3.5-turbo",
  temperature: 1,
});
```

## Caching Key Generation

ls $GOPATH/bin
The `generateCacheKey()` function creates a unique cache key by hashing the request's path, body, and the 'Authorization' bearer token if present.

## Token Counting

The `CountTokens()` method, part of the `OpenAIProvider` implementation of `IModelProvider`, counts tokens in the request content for a given model, aiding in managing token usage.

## Note

This application serves as an example and may require additional security and error handling features to be production-ready.
