package sdlmerge

import "github.com/wundergraph/graphql-go-tools/pkg/ast"

type fieldlessSharedType interface {
	areValuesIdentical(valueRefsToCompare []int) bool
	valueRefs() []int
	valueName(ref int) string
}

func createValueSet(f fieldlessSharedType) map[string]bool {
	valueSet := make(map[string]bool)
	for _, valueRef := range f.valueRefs() {
		valueSet[f.valueName(valueRef)] = true
	}
	return valueSet
}

type fieldedSharedType struct {
	document  *ast.Document
	fieldKind ast.NodeKind
	fieldRefs []int
	fieldSet  map[string]int
}

func newFieldedSharedType(document *ast.Document, fieldKind ast.NodeKind, fieldRefs []int) fieldedSharedType {
	f := fieldedSharedType{
		document,
		fieldKind,
		fieldRefs,
		nil,
	}
	f.createFieldSet()
	return f
}

func (f fieldedSharedType) areFieldsIdentical(fieldRefsToCompare []int) bool {
	if len(f.fieldRefs) != len(fieldRefsToCompare) {
		return false
	}
	for _, fieldRef := range fieldRefsToCompare {
		actualFieldName := f.fieldName(fieldRef)
		expectedTypeRef, exists := f.fieldSet[actualFieldName]
		if !exists {
			return false
		}
		actualTypeRef := f.fieldTypeRef(fieldRef)
		if !f.document.TypesAreCompatibleDeep(expectedTypeRef, actualTypeRef) {
			return false
		}
	}
	return true
}

func (f *fieldedSharedType) createFieldSet() {
	fieldSet := make(map[string]int)
	for _, fieldRef := range f.fieldRefs {
		fieldSet[f.fieldName(fieldRef)] = f.fieldTypeRef(fieldRef)
	}
	f.fieldSet = fieldSet
}

func (f fieldedSharedType) fieldName(ref int) string {
	switch f.fieldKind {
	case ast.NodeKindInputValueDefinition:
		return f.document.InputValueDefinitionNameString(ref)
	default:
		return f.document.FieldDefinitionNameString(ref)
	}
}

func (f fieldedSharedType) fieldTypeRef(ref int) int {
	switch f.fieldKind {
	case ast.NodeKindInputValueDefinition:
		return f.document.InputValueDefinitions[ref].Type
	default:
		return f.document.FieldDefinitions[ref].Type
	}
}

type enumSharedType struct {
	*ast.EnumTypeDefinition
	document *ast.Document
	valueSet map[string]bool
}

func newEnumSharedType(document *ast.Document, ref int) enumSharedType {
	e := enumSharedType{
		&document.EnumTypeDefinitions[ref],
		document,
		nil,
	}
	e.valueSet = createValueSet(e)
	return e
}

func (e enumSharedType) areValuesIdentical(valueRefsToCompare []int) bool {
	if len(e.valueRefs()) != len(valueRefsToCompare) {
		return false
	}
	for _, valueRefToCompare := range valueRefsToCompare {
		name := e.valueName(valueRefToCompare)
		if !e.valueSet[name] {
			return false
		}
	}
	return true
}

func (e enumSharedType) valueRefs() []int {
	return e.EnumValuesDefinition.Refs
}

func (e enumSharedType) valueName(ref int) string {
	return e.document.EnumValueDefinitionNameString(ref)
}

type unionSharedType struct {
	*ast.UnionTypeDefinition
	document *ast.Document
	valueSet map[string]bool
}

func newUnionSharedType(document *ast.Document, ref int) unionSharedType {
	u := unionSharedType{
		&document.UnionTypeDefinitions[ref],
		document,
		nil,
	}
	u.valueSet = createValueSet(u)
	return u
}

func (u unionSharedType) areValuesIdentical(valueRefsToCompare []int) bool {
	if len(u.valueRefs()) != len(valueRefsToCompare) {
		return false
	}
	for _, refToCompare := range valueRefsToCompare {
		name := u.valueName(refToCompare)
		if !u.valueSet[name] {
			return false
		}
	}
	return true
}

func (u unionSharedType) valueRefs() []int {
	return u.UnionMemberTypes.Refs
}

func (u unionSharedType) valueName(ref int) string {
	return u.document.TypeNameString(ref)
}

type scalarSharedType struct {
}

func (_ scalarSharedType) areValuesIdentical(_ []int) bool {
	return true
}

func (_ scalarSharedType) valueRefs() []int {
	return nil
}

func (_ scalarSharedType) valueName(_ int) string {
	return ""
}
