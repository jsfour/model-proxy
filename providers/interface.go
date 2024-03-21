package provider

import "net/http"

type IModelProvider interface {
	GetEndpoint() string
	GetModels() []string
	CountTokens(req *http.Request, model string) (int, error)
}
