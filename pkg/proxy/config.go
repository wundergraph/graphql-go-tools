package proxy

import (
	"context"
	"net/url"
)

type RequestConfigProvider interface {
	GetRequestConfig(ctx context.Context) RequestConfig
}

type RequestConfig struct {
	Schema              *[]byte
	BackendURL          url.URL
	AddHeadersToContext [][]byte
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
