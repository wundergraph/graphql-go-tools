package proxy

type RequestConfigProvider interface {
	GetRequestConfig(requestURI []byte) RequestConfig
}

type RequestConfig struct {
	Schema              *[]byte
	BackendHost         string
	BackendAddr         []byte
	AddHeadersToContext [][]byte
}

type StaticRequestConfigProvider struct {
	config RequestConfig
}

func (s *StaticRequestConfigProvider) GetRequestConfig(requestURI []byte) RequestConfig {
	return s.config
}

func NewStaticSchemaProvider(config RequestConfig) *StaticRequestConfigProvider {
	return &StaticRequestConfigProvider{
		config: config,
	}
}
