package plan

import (
	"slices"
)

type TypeField struct {
	TypeName           string
	FieldNames         []string
	ExternalFieldNames []string
}

type TypeFields []TypeField

func (f TypeFields) HasNode(typeName, fieldName string) bool {
	return slices.ContainsFunc(f, func(t TypeField) bool {
		return typeName == t.TypeName && slices.Contains(t.FieldNames, fieldName)
	})
}

func (f TypeFields) HasExternalNode(typeName, fieldName string) bool {
	return slices.ContainsFunc(f, func(t TypeField) bool {
		return typeName == t.TypeName && slices.Contains(t.ExternalFieldNames, fieldName)
	})
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
