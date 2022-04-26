package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type removeDuplicateFieldlessValueTypesVisitor struct {
	*astvisitor.Walker
	document          *ast.Document
	valueTypeSet      map[string]FieldlessValueType
	rootNodesToRemove []ast.Node
	lastEnumRef       int
	lastUnionRef      int
	lastScalarRef     int
}

func newRemoveDuplicateFieldlessValueTypesVisitor() *removeDuplicateFieldlessValueTypesVisitor {
	return &removeDuplicateFieldlessValueTypesVisitor{
		nil,
		nil,
		make(map[string]FieldlessValueType),
		nil,
		ast.InvalidRef,
		ast.InvalidRef,
		ast.InvalidRef,
	}
}

func (r *removeDuplicateFieldlessValueTypesVisitor) Register(walker *astvisitor.Walker) {
	r.Walker = walker
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterEnterEnumTypeDefinitionVisitor(r)
	walker.RegisterEnterScalarTypeDefinitionVisitor(r)
	walker.RegisterEnterUnionTypeDefinitionVisitor(r)
	walker.RegisterLeaveDocumentVisitor(r)
}

func (r *removeDuplicateFieldlessValueTypesVisitor) EnterDocument(operation, _ *ast.Document) {
	r.document = operation
}

func (r *removeDuplicateFieldlessValueTypesVisitor) EnterEnumTypeDefinition(ref int) {
	if ref <= r.lastEnumRef {
		return
	}
	document := r.document
	name := document.EnumTypeDefinitionNameString(ref)
	enum, exists := r.valueTypeSet[name]
	if exists {
		if !enum.AreValueRefsIdentical(r, document.EnumTypeDefinitions[ref].EnumValuesDefinition.Refs) {
			r.StopWithExternalErr(operationreport.ErrFederatingFieldlessValueType(name))
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindEnumTypeDefinition, Ref: ref})
	} else {
		r.valueTypeSet[name] = NewEnumValueType(r, ref)
	}
	r.lastEnumRef = ref
}

func (r *removeDuplicateFieldlessValueTypesVisitor) EnterScalarTypeDefinition(ref int) {
	if ref <= r.lastScalarRef {
		return
	}
	name := r.document.ScalarTypeDefinitionNameString(ref)
	_, exists := r.valueTypeSet[name]
	if exists {
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindScalarTypeDefinition, Ref: ref})
	} else {
		r.valueTypeSet[name] = ScalarValueType{name}
	}
	r.lastScalarRef = ref
}

func (r *removeDuplicateFieldlessValueTypesVisitor) EnterUnionTypeDefinition(ref int) {
	if ref <= r.lastUnionRef {
		return
	}
	document := r.document
	name := document.UnionTypeDefinitionNameString(ref)
	union, exists := r.valueTypeSet[name]
	if exists {
		if !union.AreValueRefsIdentical(r, document.UnionTypeDefinitions[ref].UnionMemberTypes.Refs) {
			r.StopWithExternalErr(operationreport.ErrFederatingFieldlessValueType(name))
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindUnionTypeDefinition, Ref: ref})
	} else {
		r.valueTypeSet[name] = NewUnionValueType(r, ref)
	}
	r.lastUnionRef = ref
}

func (r *removeDuplicateFieldlessValueTypesVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.rootNodesToRemove != nil {
		r.document.DeleteRootNodes(r.rootNodesToRemove)
	}
}
