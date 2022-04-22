package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

type removeDuplicateUnionTypeDefinitionVisitor struct {
	document      *ast.Document
	unionSet      map[string]FieldlessParentType
	nodesToRemove []ast.Node
	lastRef       int
}

func newRemoveDuplicateUnionTypeDefinitionVisitor() *removeDuplicateUnionTypeDefinitionVisitor {
	return &removeDuplicateUnionTypeDefinitionVisitor{
		nil,
		make(map[string]FieldlessParentType),
		nil,
		ast.InvalidRef,
	}
}

func (r *removeDuplicateUnionTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterLeaveDocumentVisitor(r)
	walker.RegisterEnterUnionTypeDefinitionVisitor(r)
}

func (r *removeDuplicateUnionTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	r.document = operation
}

func (r *removeDuplicateUnionTypeDefinitionVisitor) EnterUnionTypeDefinition(ref int) {
	if ref <= r.lastRef {
		return
	}
	name := r.document.UnionTypeDefinitionNameString(ref)
	union, exists := r.unionSet[name]
	if exists {
		union.AppendValueRefs(r.document.UnionTypeDefinitions[ref].UnionMemberTypes.Refs)
		r.nodesToRemove = append(r.nodesToRemove, ast.Node{Kind: ast.NodeKindUnionTypeDefinition, Ref: ref})
	} else {
		r.unionSet[name] = UnionParentType{&r.document.UnionTypeDefinitions[ref], name}
	}
	r.lastRef = ref
}

func (r *removeDuplicateUnionTypeDefinitionVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.nodesToRemove == nil {
		return
	}
	for _, union := range r.unionSet {
		valueSet := make(map[string]bool)
		var valuesToKeep []int
		for _, ref := range union.ValueRefs() {
			if !valueSet[r.document.TypeNameString(ref)] {
				valueSet[r.document.TypeNameString(ref)] = true
				valuesToKeep = append(valuesToKeep, ref)
			}
		}
		union.SetValueRefs(valuesToKeep)
	}
	r.document.DeleteRootNodesInSingleLoop(r.nodesToRemove)
}
