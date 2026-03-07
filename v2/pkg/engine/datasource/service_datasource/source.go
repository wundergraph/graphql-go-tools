package service_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// Source is the data source for the __service field.
type Source struct {
	service *Service
}

// NewSource creates a new Source with the given service configuration.
func NewSource(service *Service) *Source {
	return &Source{service: service}
}

// Load implements the DataSource interface.
func (s *Source) Load(ctx context.Context, headers http.Header, input []byte) (data []byte, err error) {
	return json.Marshal(s.service)
}

// LoadWithFiles implements the DataSource interface.
func (s *Source) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (data []byte, err error) {
	return nil, errors.New("service data source does not support file uploads")
}
