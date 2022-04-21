package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

type removeDuplicateUnionTypeDefinitionVisitor struct {
	document      *ast.Document
	unionSet      map[string]*ast.ParentType
	nodesToRemove []ast.Node
	lastRef       int
}

func newRemoveDuplicateUnionTypeDefinitionVisitor() *removeDuplicateUnionTypeDefinitionVisitor {
	return &removeDuplicateUnionTypeDefinitionVisitor{
		nil,
		make(map[string]*ast.ParentType),
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
	_, exists := r.unionSet[name]
	if exists {
		r.nodesToRemove = append(r.nodesToRemove, ast.Node{Kind: ast.NodeKindUnionTypeDefinition, Ref: ref})
	} else {
		r.unionSet[name] = &ast.ParentType{Ref: ref, ValueRefs: nil}
	}
	Union := r.unionSet[name]
	Union.ValueRefs = append(Union.ValueRefs, r.document.UnionTypeDefinitions[ref].UnionMemberTypes.Refs...)
	r.lastRef = ref
}

func (r *removeDuplicateUnionTypeDefinitionVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.nodesToRemove == nil {
		return
	}
	for _, Union := range r.unionSet {
		valueSet := make(map[string]bool)
		var valuesToKeep []int
		for _, ref := range Union.ValueRefs {
			if !valueSet[r.document.TypeNameString(ref)] {
				valueSet[r.document.TypeNameString(ref)] = true
				valuesToKeep = append(valuesToKeep, ref)
			}
		}
		Union.ValueRefs = valuesToKeep
	}
	r.document.MergeAndRemoveDuplicateParentTypes(r.nodesToRemove, r.unionSet)
}
