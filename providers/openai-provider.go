package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

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

func NewOpenAIProvider() *OpenAIProvider {
	return &OpenAIProvider{
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
}
