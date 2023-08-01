package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

type removeDuplicateFieldlessSharedTypesVisitor struct {
	*astvisitor.Walker
	document          *ast.Document
	sharedTypeSet     map[string]fieldlessSharedType
	rootNodesToRemove []ast.Node
	lastEnumRef       int
	lastUnionRef      int
	lastScalarRef     int
}

func newRemoveDuplicateFieldlessSharedTypesVisitor() *removeDuplicateFieldlessSharedTypesVisitor {
	return &removeDuplicateFieldlessSharedTypesVisitor{
		nil,
		nil,
		make(map[string]fieldlessSharedType),
		nil,
		ast.InvalidRef,
		ast.InvalidRef,
		ast.InvalidRef,
	}
}

func (r *removeDuplicateFieldlessSharedTypesVisitor) Register(walker *astvisitor.Walker) {
	r.Walker = walker
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterEnterEnumTypeDefinitionVisitor(r)
	walker.RegisterEnterScalarTypeDefinitionVisitor(r)
	walker.RegisterEnterUnionTypeDefinitionVisitor(r)
	walker.RegisterLeaveDocumentVisitor(r)
}

func (r *removeDuplicateFieldlessSharedTypesVisitor) EnterDocument(operation, _ *ast.Document) {
	r.document = operation
}

func (r *removeDuplicateFieldlessSharedTypesVisitor) EnterEnumTypeDefinition(ref int) {
	if ref <= r.lastEnumRef {
		return
	}
	name := r.document.EnumTypeDefinitionNameString(ref)
	enum, exists := r.sharedTypeSet[name]
	if exists {
		if !enum.areValuesIdentical(r.document.EnumTypeDefinitions[ref].EnumValuesDefinition.Refs) {
			r.StopWithExternalErr(operationreport.ErrSharedTypesMustBeIdenticalToFederate(name))
			return
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindEnumTypeDefinition, Ref: ref})
	} else {
		r.sharedTypeSet[name] = newEnumSharedType(r.document, ref)
	}
	r.lastEnumRef = ref
}

func (r *removeDuplicateFieldlessSharedTypesVisitor) EnterScalarTypeDefinition(ref int) {
	if ref <= r.lastScalarRef {
		return
	}
	name := r.document.ScalarTypeDefinitionNameString(ref)
	_, exists := r.sharedTypeSet[name]
	if exists {
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindScalarTypeDefinition, Ref: ref})
	} else {
		r.sharedTypeSet[name] = scalarSharedType{}
	}
	r.lastScalarRef = ref
}

func (r *removeDuplicateFieldlessSharedTypesVisitor) EnterUnionTypeDefinition(ref int) {
	if ref <= r.lastUnionRef {
		return
	}
	name := r.document.UnionTypeDefinitionNameString(ref)
	union, exists := r.sharedTypeSet[name]
	if exists {
		if !union.areValuesIdentical(r.document.UnionTypeDefinitions[ref].UnionMemberTypes.Refs) {
			r.StopWithExternalErr(operationreport.ErrSharedTypesMustBeIdenticalToFederate(name))
			return
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindUnionTypeDefinition, Ref: ref})
	} else {
		r.sharedTypeSet[name] = newUnionSharedType(r.document, ref)
	}
	r.lastUnionRef = ref
}

func (r *removeDuplicateFieldlessSharedTypesVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.rootNodesToRemove != nil {
		r.document.DeleteRootNodes(r.rootNodesToRemove)
	}
}
