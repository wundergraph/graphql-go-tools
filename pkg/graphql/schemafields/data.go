package schemafields

type Data struct {
	Types []Type `json:"types"`
}

type Type struct {
	Name   string  `json:"name"`
	Fields []Field `json:"fields"`
}

type Field struct {
	Name    string  `json:"name"`
	Type    string  `json:"type"`
	TypeRef *string `json:"type_ref"`
}
