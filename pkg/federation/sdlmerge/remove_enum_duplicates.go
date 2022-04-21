package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

type removeDuplicateEnumTypeDefinitionVisitor struct {
	document      *ast.Document
	enumSet       map[string]*ast.ParentType
	nodesToRemove []ast.Node
	lastRef       int
}

func newRemoveDuplicateEnumTypeDefinitionVisitor() *removeDuplicateEnumTypeDefinitionVisitor {
	return &removeDuplicateEnumTypeDefinitionVisitor{
		nil,
		make(map[string]*ast.ParentType),
		nil,
		ast.InvalidRef,
	}
}

func (r *removeDuplicateEnumTypeDefinitionVisitor) Register(walker *astvisitor.Walker) {
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterLeaveDocumentVisitor(r)
	walker.RegisterEnterEnumTypeDefinitionVisitor(r)
}

func (r *removeDuplicateEnumTypeDefinitionVisitor) EnterDocument(operation, _ *ast.Document) {
	r.document = operation
}

func (r *removeDuplicateEnumTypeDefinitionVisitor) EnterEnumTypeDefinition(ref int) {
	if ref <= r.lastRef {
		return
	}
	name := r.document.EnumTypeDefinitionNameString(ref)
	_, exists := r.enumSet[name]
	if exists {
		r.nodesToRemove = append(r.nodesToRemove, ast.Node{Kind: ast.NodeKindEnumTypeDefinition, Ref: ref})
	} else {
		r.enumSet[name] = &ast.ParentType{Ref: ref, ValueRefs: nil}
	}
	enum := r.enumSet[name]
	enum.ValueRefs = append(enum.ValueRefs, r.document.EnumTypeDefinitions[ref].EnumValuesDefinition.Refs...)
	r.lastRef = ref
}

func (r *removeDuplicateEnumTypeDefinitionVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.nodesToRemove == nil {
		return
	}
	for _, enum := range r.enumSet {
		valueSet := make(map[string]bool)
		var valuesToKeep []int
		for _, ref := range enum.ValueRefs {
			if !valueSet[r.document.EnumValueDefinitionNameString(ref)] {
				valueSet[r.document.EnumValueDefinitionNameString(ref)] = true
				valuesToKeep = append(valuesToKeep, ref)
			}
		}
		enum.ValueRefs = valuesToKeep
	}
	r.document.MergeAndRemoveDuplicateParentTypes(r.nodesToRemove, r.enumSet)
}
