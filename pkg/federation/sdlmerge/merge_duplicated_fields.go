package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
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
	fieldSet := make(map[string]struct{})
	for _, fieldRef := range m.document.ObjectTypeDefinitions[ref].FieldsDefinition.Refs {
		fieldName := m.document.FieldDefinitionNameString(fieldRef)
		if _, ok := fieldSet[fieldName]; ok {
			refsForDeletion = append(refsForDeletion, fieldRef)
		} else {
			fieldSet[fieldName] = struct{}{}
		}
	}

	m.document.RemoveFieldDefinitionsFromObjectTypeDefinition(refsForDeletion, ref)
}
