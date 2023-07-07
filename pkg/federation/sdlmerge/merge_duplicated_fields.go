package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

type mergeDuplicatedFieldsVisitor struct {
	*astvisitor.Walker
	document *ast.Document
}

func newMergeDuplicatedFieldsVisitor() *mergeDuplicatedFieldsVisitor {
	return &mergeDuplicatedFieldsVisitor{
		nil,
		nil,
	}
}

func (m *mergeDuplicatedFieldsVisitor) Register(walker *astvisitor.Walker) {
	m.Walker = walker
	walker.RegisterEnterDocumentVisitor(m)
	walker.RegisterLeaveObjectTypeDefinitionVisitor(m)
}

func (m *mergeDuplicatedFieldsVisitor) EnterDocument(document, _ *ast.Document) {
	m.document = document
}

func (m *mergeDuplicatedFieldsVisitor) LeaveObjectTypeDefinition(ref int) {
	var refsForDeletion []int
	fieldByTypeRefSet := make(map[string]int)
	for _, fieldRef := range m.document.ObjectTypeDefinitions[ref].FieldsDefinition.Refs {
		fieldName := m.document.FieldDefinitionNameString(fieldRef)
		newTypeRef := m.document.FieldDefinitions[fieldRef].Type
		if oldTypeRef, ok := fieldByTypeRefSet[fieldName]; ok {
			if m.document.TypesAreEqualDeep(oldTypeRef, newTypeRef) {
				refsForDeletion = append(refsForDeletion, fieldRef)
				continue
			}
			oldFieldTypeNameBytes, err := m.document.PrintTypeBytes(oldTypeRef, nil)
			if err != nil {
				m.Walker.StopWithInternalErr(err)
				return
			}
			newFieldTypeNameBytes, err := m.document.PrintTypeBytes(newTypeRef, nil)
			if err != nil {
				m.Walker.StopWithInternalErr(err)
				return
			}
			m.Walker.StopWithExternalErr(operationreport.ErrDuplicateFieldsMustBeIdentical(
				fieldName, m.document.ObjectTypeDefinitionNameString(ref), string(oldFieldTypeNameBytes), string(newFieldTypeNameBytes),
			))
			return
		}

		fieldByTypeRefSet[fieldName] = newTypeRef
	}

	m.document.RemoveFieldDefinitionsFromObjectTypeDefinition(refsForDeletion, ref)
}
