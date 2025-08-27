package plan

type TypeField struct {
	TypeName            string
	FieldNames          []string
	ExternalFieldNames  []string
	ProtectedFieldNames []string
}

type TypeFields []TypeField
