package introspection_datasource

type introspectionRequestType int

const (
	SchemaIntrospectionRequestType introspectionRequestType = iota + 1
	TypeIntrospectionRequestType
	TypeFieldsIntrospectionRequestType
	EnumValuesIntrospectionRequestType
)

type introspectionInput struct {
	RequestType       introspectionRequestType `json:"request_type"`
	OnTypeName        *string                  `json:"on_type_name"`
	TypeName          *string                  `json:"type_name"`
	IncludeDeprecated bool                     `json:"include_deprecated"`
}
