package plan

type TypeField struct {
	TypeName   string
	FieldNames []string
}

type TypeFields []TypeField

func (f TypeFields) HasNode(typeName, fieldName string) bool {
	for i := range f {
		if typeName != f[i].TypeName {
			continue
		}
		for j := range f[i].FieldNames {
			if fieldName == f[i].FieldNames[j] {
				return true
			}
		}
	}
	return false
}

func (f TypeFields) HasNodeWithTypename(typeName string) bool {
	for i := range f {
		if typeName != f[i].TypeName {
			continue
		}
		return true
	}
	return false
}
