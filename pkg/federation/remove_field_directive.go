package federation

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

func removeDirective(directives ...string) func(walker * astvisitor.Walker) {
	return func(walker *astvisitor.Walker) {
		directivesSet := make(map[string]struct{}, len(directives))
		for _, directive := range directives {
			directivesSet[directive] = struct{}{}
		}

		visitor := removeFieldDirective{
			Walker:    walker,
			directives: directivesSet,
		}

		walker.RegisterEnterDocumentVisitor(&visitor)
		walker.RegisterEnterObjectTypeDefinitionVisitor(&visitor)
	}
}

type removeFieldDirective struct {
	*astvisitor.Walker
	operation *ast.Document
	directives map[string]struct{}
}

func (r *removeFieldDirective) EnterDocument(operation, _ *ast.Document) {
	r.operation = operation
}

func (r *removeFieldDirective) EnterObjectTypeDefinition(ref int) {
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