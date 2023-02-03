package asyncapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/pkg/introspection"
	"github.com/TykTechnologies/graphql-go-tools/pkg/lexer/literal"
	"github.com/TykTechnologies/graphql-go-tools/pkg/operationreport"
	"github.com/buger/jsonparser"
	"github.com/iancoleman/strcase"
)

type converter struct {
	asyncapi   *AsyncAPI
	knownEnums map[string]struct{}
	knownTypes map[string]struct{}
}

// __TypeKind of introspection is an unexported type. In order to overcome the problem,
// this function creates and returns a TypeRef for a given kind. kind is a AsyncAPI type.
func getTypeRef(kind string) (introspection.TypeRef, error) {
	// See introspection_enum.go
	switch kind {
	case "string", "integer", "number", "boolean":
		return introspection.TypeRef{Kind: 0}, nil
	case "object":
		return introspection.TypeRef{Kind: 3}, nil
	case "array":
		return introspection.TypeRef{Kind: 1}, nil
	}
	return introspection.TypeRef{}, errors.New("unknown type")
}

func asyncAPITypeToGQLType(asyncAPIType string) (string, error) {
	// See https://www.asyncapi.com/docs/reference/specification/v2.4.0#dataTypeFormat
	switch asyncAPIType {
	case "string":
		return string(literal.STRING), nil
	case "integer":
		return string(literal.INT), nil
	case "number":
		return string(literal.FLOAT), nil
	case "boolean":
		return string(literal.BOOLEAN), nil
	default:
		return "", fmt.Errorf("unknown type: %s", asyncAPIType)
	}
}

func (c *converter) importEnumType(name string, enums []*Enum) *introspection.FullType {
	enumName := strcase.ToCamel(name)
	_, ok := c.knownEnums[enumName]
	if ok {
		return nil
	}

	enumType := &introspection.FullType{
		Kind: introspection.ENUM,
		Name: enumName,
	}
	for _, enum := range enums {
		if enum.ValueType == jsonparser.String {
			enumType.EnumValues = append(enumType.EnumValues, introspection.EnumValue{
				Name: strings.ToUpper(strcase.ToSnake(string(enum.Value))),
			})
		}
	}
	c.knownEnums[name] = struct{}{}
	return enumType
}

func (c *converter) importFullTypes() ([]introspection.FullType, error) {
	fullTypes := make([]introspection.FullType, 0)
	for _, channelItem := range c.asyncapi.Channels {
		msg := channelItem.Message

		fullTypeName := strcase.ToCamel(msg.Name)
		if _, ok := c.knownTypes[fullTypeName]; ok {
			continue
		}

		var sb = strings.Builder{}
		sb.WriteString(msg.Title)
		sb.WriteString("\n")
		sb.WriteString(msg.Summary)
		sb.WriteString("\n")
		sb.WriteString(msg.Description)
		ft := introspection.FullType{
			Kind:        introspection.OBJECT,
			Name:        fullTypeName,
			Description: strings.TrimSpace(sb.String()),
		}

		for name, prop := range msg.Payload.Properties {
			var f introspection.Field
			if prop.Enum == nil {
				gqlType, err := asyncAPITypeToGQLType(prop.Type)
				if err != nil {
					return nil, err
				}
				typeRef, err := getTypeRef(prop.Type)
				if err != nil {
					return nil, err
				}
				typeRef.Name = &gqlType
				f = introspection.Field{
					Name:        name,
					Description: prop.Description,
					Type:        typeRef,
				}
			} else {
				// ENUM type and its fields.
				enumType := c.importEnumType(name, prop.Enum)
				if enumType != nil {
					fullTypes = append(fullTypes, *enumType)
				}
				enumTypeName := strcase.ToCamel(name)
				typeRef, err := getTypeRef(prop.Type)
				if err != nil {
					return nil, err
				}
				typeRef.Name = &enumTypeName
				f = introspection.Field{
					Name:        name,
					Description: prop.Description,
					Type:        typeRef,
				}
			}
			ft.Fields = append(ft.Fields, f)
			sort.Slice(ft.Fields, func(i, j int) bool {
				return ft.Fields[i].Name < ft.Fields[j].Name
			})
		}

		c.knownTypes[fullTypeName] = struct{}{}
		fullTypes = append(fullTypes, ft)
		sort.Slice(fullTypes, func(i, j int) bool {
			return fullTypes[i].Name < fullTypes[j].Name
		})
	}
	return fullTypes, nil
}

func (c *converter) importSubscriptionType() (*introspection.FullType, error) {
	subscriptionType := &introspection.FullType{
		Kind: introspection.OBJECT,
		Name: "Subscription",
	}
	for _, channelItem := range c.asyncapi.Channels {
		typeName := strcase.ToCamel(channelItem.Message.Name)
		typeRef, err := getTypeRef("object")
		if err != nil {
			return nil, err
		}
		typeRef.Name = &typeName
		f := introspection.Field{
			Name: strcase.ToLowerCamel(channelItem.OperationID),
			Type: typeRef,
		}
		for paramName, paramType := range channelItem.Parameters {
			gqlType, err := asyncAPITypeToGQLType(paramType)
			if err != nil {
				return nil, err
			}

			paramTypeRef, err := getTypeRef(paramType)
			if err != nil {
				return nil, err
			}
			paramTypeRef.Name = &gqlType

			iv := introspection.InputValue{
				Name: paramName,
				Type: paramTypeRef,
			}
			f.Args = append(f.Args, iv)
			sort.Slice(f.Args, func(i, j int) bool {
				return f.Args[i].Name < f.Args[j].Name
			})
		}

		subscriptionType.Fields = append(subscriptionType.Fields, f)
		sort.Slice(subscriptionType.Fields, func(i, j int) bool {
			return subscriptionType.Fields[i].Name < subscriptionType.Fields[j].Name
		})
	}
	return subscriptionType, nil
}

func (c *converter) importQueryType() (*introspection.FullType, error) {
	// Query root type must be provided. We add an empty Query type with a dummy field.
	//
	// type Query {
	//    _: Boolean
	// }
	queryType := &introspection.FullType{
		Kind: introspection.OBJECT,
		Name: "Query",
	}
	typeName := string(literal.BOOLEAN)
	queryType.Fields = append(queryType.Fields, introspection.Field{
		Name: "_",
		Type: introspection.TypeRef{Kind: 0, Name: &typeName},
	})
	return queryType, nil
}

func ImportParsedAsyncAPIDocument(parsed *AsyncAPI, report *operationreport.Report) *ast.Document {
	// A parsed AsyncAPI document may include the same enum type name more than once.
	// In order to prevent from duplicated types in the resulting schema, we save the names.
	c := &converter{
		asyncapi:   parsed,
		knownEnums: make(map[string]struct{}),
		knownTypes: make(map[string]struct{}),
	}
	data := introspection.Data{}

	data.Schema.QueryType = &introspection.TypeName{
		Name: "Query",
	}
	queryType, err := c.importQueryType()
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	data.Schema.Types = append(data.Schema.Types, *queryType)

	data.Schema.SubscriptionType = &introspection.TypeName{
		Name: "Subscription",
	}
	subscriptionType, err := c.importSubscriptionType()
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	data.Schema.Types = append(data.Schema.Types, *subscriptionType)

	fullTypes, err := c.importFullTypes()
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	data.Schema.Types = append(data.Schema.Types, fullTypes...)

	outputPretty, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		report.AddInternalError(err)
		return nil
	}

	jc := introspection.JsonConverter{}
	buf := bytes.NewBuffer(outputPretty)
	doc, err := jc.GraphQLDocument(buf)
	if err != nil {
		report.AddInternalError(err)
		return nil
	}
	return doc
}

func ImportAsyncAPIDocumentByte(input []byte) (*ast.Document, operationreport.Report) {
	report := operationreport.Report{}
	asyncapi, err := ParseAsyncAPIDocument(input)
	if err != nil {
		report.AddInternalError(err)
		return nil, report
	}
	return ImportParsedAsyncAPIDocument(asyncapi, &report), report
}

func ImportAsyncAPIDocumentString(input string) (*ast.Document, operationreport.Report) {
	return ImportAsyncAPIDocumentByte([]byte(input))
}
