package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
)

type removeDuplicateEnumTypeDefinitionVisitor struct {
	document      *ast.Document
	enumSet       map[string]FieldlessParentType
	nodesToRemove []ast.Node
	lastRef       int
}

func newRemoveDuplicateEnumTypeDefinitionVisitor() *removeDuplicateEnumTypeDefinitionVisitor {
	return &removeDuplicateEnumTypeDefinitionVisitor{
		nil,
		make(map[string]FieldlessParentType),
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
	enum, exists := r.enumSet[name]
	if exists {
		enum.AppendValueRefs(r.document.EnumTypeDefinitions[ref].EnumValuesDefinition.Refs)
		r.nodesToRemove = append(r.nodesToRemove, ast.Node{Kind: ast.NodeKindEnumTypeDefinition, Ref: ref})
	} else {
		r.enumSet[name] = EnumParentType{&r.document.EnumTypeDefinitions[ref], name}
	}
	r.lastRef = ref
}

func (r *removeDuplicateEnumTypeDefinitionVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.nodesToRemove == nil {
		return
	}
	for _, enum := range r.enumSet {
		valueSet := make(map[string]bool)
		var valuesToKeep []int
		for _, ref := range enum.ValueRefs() {
			if !valueSet[r.document.EnumValueDefinitionNameString(ref)] {
				valueSet[r.document.EnumValueDefinitionNameString(ref)] = true
				valuesToKeep = append(valuesToKeep, ref)
			}
		}
		enum.SetValueRefs(valuesToKeep)
	}
	r.document.DeleteRootNodesInSingleLoop(r.nodesToRemove)
}
