package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func newRemoveFieldDefinitions(directives ...string) *removeFieldDefinitionByDirective {
	directivesSet := make(map[string]struct{}, len(directives))
	for _, directive := range directives {
		directivesSet[directive] = struct{}{}
	}

	return &removeFieldDefinitionByDirective{
		directives: directivesSet,
	}
}

type removeFieldDefinitionByDirective struct {
	operation  *ast.Document
	directives map[string]struct{}
}

func (r *removeFieldDefinitionByDirective) Register(walker *astvisitor.Walker) {
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterLeaveObjectTypeDefinitionVisitor(r)
}

func (r *removeFieldDefinitionByDirective) EnterDocument(operation, _ *ast.Document) {
	r.operation = operation
}

func (r *removeFieldDefinitionByDirective) LeaveObjectTypeDefinition(ref int) {
	var refsForDeletion []int
	// select fields for deletion
	for _, fieldRef := range r.operation.ObjectTypeDefinitions[ref].FieldsDefinition.Refs {
		for _, directiveRef := range r.operation.FieldDefinitions[fieldRef].Directives.Refs {
			directiveName := r.operation.DirectiveNameString(directiveRef)
			if _, ok := r.directives[directiveName]; ok {
				refsForDeletion = append(refsForDeletion, fieldRef)
			}
		}
	}
	// delete fields
	for _, fieldRef := range refsForDeletion {
		if i, ok := indexOf(r.operation.ObjectTypeDefinitions[ref].FieldsDefinition.Refs, fieldRef); ok {
			r.operation.ObjectTypeDefinitions[ref].FieldsDefinition.Refs = append(r.operation.ObjectTypeDefinitions[ref].FieldsDefinition.Refs[:i], r.operation.ObjectTypeDefinitions[ref].FieldsDefinition.Refs[i+1:]...)
			r.operation.ObjectTypeDefinitions[ref].HasFieldDefinitions = len(r.operation.ObjectTypeDefinitions[ref].FieldsDefinition.Refs) > 0
		}
	}
}

func indexOf(refs []int, ref int) (int, bool) {
	for i, j := range refs {
		if ref == j {
			return i, true
		}
	}

	return -1, false
}
