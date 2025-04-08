package introspection_datasource

import (
	"bytes"
	"strconv"
)

type requestType int

const (
	SchemaRequestType requestType = iota + 1
	TypeRequestType
	TypeFieldsRequestType
	TypeEnumValuesRequestType
	RootQueryTypeNameRequestType
)

const (
	schemaFieldName     = "__schema"
	typeFieldName       = "__type"
	fieldsFieldName     = "fields"
	enumValuesFieldName = "enumValues"
	rootQueryTypeName   = "__typename"
)

type introspectionInput struct {
	RequestType       requestType `json:"request_type"`
	OnTypeName        *string     `json:"on_type_name"`
	TypeName          *string     `json:"type_name"`
	RootQueryTypeName *string     `json:"__typename"`
	IncludeDeprecated bool        `json:"include_deprecated"`
}

var (
	lBrace                         = []byte("{")
	rBrace                         = []byte("}")
	comma                          = []byte(",")
	requestTypeField               = []byte(`"request_type":`)
	onTypeField                    = []byte(`"on_type_name":"{{ .object.name }}"`)
	typeNameField                  = []byte(`"type_name":"{{ .arguments.name }}"`)
	includeDeprecatedFieldArgument = []byte(`"include_deprecated":{{ .arguments.includeDeprecated }}`)
	includeDeprecatedFalse         = []byte(`"include_deprecated":false`)
	quote                          = []byte(`"`)
	rootQueryTypeNameField         = []byte(`,"__typename":"`)
)

func buildInput(fieldName string, hasIncludeDeprecatedArgument bool, enclosingTypeName string) string {
	buf := &bytes.Buffer{}
	buf.Write(lBrace)

	switch fieldName {
	case typeFieldName:
		writeRequestTypeField(buf, TypeRequestType)
		buf.Write(comma)
		buf.Write(typeNameField)
	case fieldsFieldName:
		writeRequestTypeField(buf, TypeFieldsRequestType)
		writeOnTypeFields(buf, hasIncludeDeprecatedArgument)
	case enumValuesFieldName:
		writeRequestTypeField(buf, TypeEnumValuesRequestType)
		writeOnTypeFields(buf, hasIncludeDeprecatedArgument)
	case rootQueryTypeName:
		writeRequestTypeField(buf, RootQueryTypeNameRequestType)
		buf.Write(rootQueryTypeNameField)
		buf.Write([]byte(enclosingTypeName))
		buf.Write(quote)
	default:
		writeRequestTypeField(buf, SchemaRequestType)
	}

	buf.Write(rBrace)

	return buf.String()
}

func writeRequestTypeField(buf *bytes.Buffer, inputType requestType) {
	buf.Write(requestTypeField)
	buf.Write([]byte(strconv.Itoa(int(inputType))))
}

func writeOnTypeFields(buf *bytes.Buffer, hasIncludeDeprecatedArgument bool) {
	buf.Write(comma)
	buf.Write(onTypeField)
	buf.Write(comma)
	if hasIncludeDeprecatedArgument {
		buf.Write(includeDeprecatedFieldArgument)
	} else {
		buf.Write(includeDeprecatedFalse)
	}
}
