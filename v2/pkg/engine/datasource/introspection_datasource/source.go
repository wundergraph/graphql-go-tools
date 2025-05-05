package introspection_datasource

import (
	"bytes"
	"context"
	"encoding/json"
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

func (s *Source) Load(ctx context.Context, input []byte, out *bytes.Buffer) (err error) {
	var req introspectionInput
	if err := json.Unmarshal(input, &req); err != nil {
		return err
	}

	switch req.RequestType {
	case TypeRequestType:
		return s.singleType(out, req.TypeName)
	case TypeEnumValuesRequestType:
		return s.enumValuesForType(out, req.OnTypeName, req.IncludeDeprecated)
	case TypeFieldsRequestType:
		return s.fieldsForType(out, req.OnTypeName, req.IncludeDeprecated)
	}

	return json.NewEncoder(out).Encode(s.schemaWithoutTypeInfo())
}

func (s *Source) LoadWithFiles(ctx context.Context, input []byte, files []*httpclient.FileUpload, out *bytes.Buffer) (err error) {
	panic("not implemented")
}

func (s *Source) schemaWithoutTypeInfo() introspection.Schema {
	return s.introspectionData.Schema
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

func (s *Source) typeWithoutFieldAndEnumValues(typeInfo *introspection.FullType) *introspection.FullType {
	typeInfoCopy := *typeInfo
	typeInfoCopy.Fields = nil
	typeInfoCopy.EnumValues = nil

	return &typeInfoCopy
}

// __Type.fields
func (s *Source) fieldsForType(w io.Writer, typeName *string, includeDeprecated bool) error {
	typeInfo := s.typeInfo(typeName)
	if typeInfo == nil || len(typeInfo.Fields) == 0 {
		return s.writeNull(w)
	}

	if includeDeprecated {
		return json.NewEncoder(w).Encode(typeInfo.Fields)
	}

	fields := make([]introspection.Field, 0, len(typeInfo.Fields))
	for _, field := range typeInfo.Fields {
		if !field.IsDeprecated {
			fields = append(fields, field)
		}
	}

	return json.NewEncoder(w).Encode(fields)
}

// __Type.enumValues
func (s *Source) enumValuesForType(w io.Writer, typeName *string, includeDeprecated bool) error {
	typeInfo := s.typeInfo(typeName)
	if typeInfo == nil || len(typeInfo.EnumValues) == 0 {
		return s.writeNull(w)
	}

	if includeDeprecated {
		return json.NewEncoder(w).Encode(typeInfo.EnumValues)
	}

	enumValues := make([]introspection.EnumValue, 0, len(typeInfo.EnumValues))
	for _, enumValue := range typeInfo.EnumValues {
		if !enumValue.IsDeprecated {
			enumValues = append(enumValues, enumValue)
		}
	}

	return json.NewEncoder(w).Encode(enumValues)
}

//  __Directive.args
// __Field.args
// __Type.inputFields
