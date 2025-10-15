package introspection_datasource

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/introspection"
)

var (
	null = []byte("null")
)

type Source struct {
	introspectionData *introspection.Data
}

func (s *Source) Load(ctx context.Context, input []byte) (data []byte, err error) {
	var req introspectionInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, err
	}

	if req.RequestType == TypeRequestType {
		return s.singleTypeBytes(req.TypeName)
	}

	return json.Marshal(s.introspectionData.Schema)
}

func (s *Source) LoadWithFiles(ctx context.Context, input []byte, files []*httpclient.FileUpload) (data []byte, err error) {
	return nil, errors.New("introspection data source does not support file uploads")
}

func (s *Source) typeInfo(typeName *string) *introspection.FullType {
	if typeName == nil {
		return nil
	}

	return s.introspectionData.Schema.TypeByName(*typeName)
}

func (s *Source) writeNull(w io.Writer) error {
	_, err := w.Write(null)
	return err
}

func (s *Source) singleType(w io.Writer, typeName *string) error {
	typeInfo := s.typeInfo(typeName)
	if typeInfo == nil {
		return s.writeNull(w)
	}

	return json.NewEncoder(w).Encode(typeInfo)
}

func (s *Source) singleTypeBytes(typeName *string) ([]byte, error) {
	typeInfo := s.typeInfo(typeName)
	if typeInfo == nil {
		return null, nil
	}

	return json.Marshal(typeInfo)
}
