package introspection_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
)

var (
	null = []byte("null")
)

type Source struct {
	introspectionData *introspection.Data
}

func (s *Source) Load(ctx context.Context, headers http.Header, input []byte) (data []byte, err error) {
	var req introspectionInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.RequestType == TypeRequestType {
		return s.singleTypeBytes(req.TypeName)
	}

	return s.introspectionData.Schema.EnrichedSchemaJSON(), nil
}

func (s *Source) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (data []byte, err error) {
	return nil, errors.New("introspection data source does not support file uploads")
}

func (s *Source) singleTypeBytes(typeName *string) ([]byte, error) {
	if typeName == nil {
		return null, nil
	}

	b := s.introspectionData.Schema.EnrichedTypeJSON(*typeName)
	if b == nil {
		return null, nil
	}

	return b, nil
}
