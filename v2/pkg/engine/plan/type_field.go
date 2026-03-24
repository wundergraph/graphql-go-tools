package plan

type TypeField struct {
	TypeName           string
	FieldNames         []string
	ExternalFieldNames []string
	FetchReasonFields  []string
}

type TypeFields []TypeField
