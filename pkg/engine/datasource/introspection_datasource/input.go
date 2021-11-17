package introspection_datasource

type requestType int

const (
	SchemaRequestType requestType = iota + 1
	TypeRequestType
	TypeFieldsRequestType
	TypeEnumValuesRequestType
)

type introspectionInput struct {
	RequestType       requestType `json:"request_type"`
	OnTypeName        *string     `json:"on_type_name"`
	TypeName          *string     `json:"type_name"`
	IncludeDeprecated bool        `json:"include_deprecated"`
}
