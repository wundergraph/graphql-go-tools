package proxy

import (
	"context"
	"net/url"
)

// RequestConfigProvider is the interface to retrieve the configuration to handle a request
// based on the provided information in the context the implementation might decide how to set the request config
// This could be used to dynamically decide which backend should be used to satisfy a request
// On the other hand the context could be ignored and you simply return a static configuration for all requests
// You can basically use any http middleware in front of the request config provider and setup the request context
// After that it's up to the RequestConfigProvider implementation how to set the configuration
// This should give the user enough flexibility
type RequestConfigProvider interface {
	GetRequestConfig(ctx context.Context) RequestConfig
}

// RequestConfig configures how the proxy should handle a request
type RequestConfig struct {
	// Schema is a pointer to the publicly exposed schema by the proxy
	Schema *[]byte
	// BackendURL is the URL of the backend origin graphql server
	BackendURL url.URL
	// AddHeadersToContext are the headers that should be extracted from a request to the proxy and added to the context
	// from the context the headers are available to graphql middleWares, e.g. to set variables
	AddHeadersToContext [][]byte
	// BackendHeaders are headers that should be statically set to backend requests
	// This could be used to add authentication to securely communicate with the origin server
	BackendHeaders map[string][]string
}

type StaticRequestConfigProvider struct {
	config RequestConfig
}

func (s *StaticRequestConfigProvider) GetRequestConfig(ctx context.Context) RequestConfig {
	return s.config
}

func NewStaticRequestConfigProvider(config RequestConfig) *StaticRequestConfigProvider {
	return &StaticRequestConfigProvider{
		config: config,
	}
}
