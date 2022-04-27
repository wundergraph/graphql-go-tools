package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type removeDuplicateFieldedValueTypesVisitor struct {
	*astvisitor.Walker
	document          *ast.Document
	valueTypeSet      map[string]FieldedValueType
	rootNodesToRemove []ast.Node
	lastInterfaceRef  int
}

func newRemoveDuplicateFieldedValueTypesVisitor() *removeDuplicateFieldedValueTypesVisitor {
	return &removeDuplicateFieldedValueTypesVisitor{
		nil,
		nil,
		make(map[string]FieldedValueType),
		nil,
		ast.InvalidRef,
	}
}

func (r *removeDuplicateFieldedValueTypesVisitor) Register(walker *astvisitor.Walker) {
	r.Walker = walker
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterEnterInterfaceTypeDefinitionVisitor(r)
	walker.RegisterLeaveDocumentVisitor(r)
}

func (r *removeDuplicateFieldedValueTypesVisitor) EnterDocument(operation, _ *ast.Document) {
	r.document = operation
}

func (r *removeDuplicateFieldedValueTypesVisitor) EnterInterfaceTypeDefinition(ref int) {
	if ref <= r.lastInterfaceRef {
		return
	}
	document := r.document
	name := document.InterfaceTypeDefinitionNameString(ref)
	i, exists := r.valueTypeSet[name]
	if exists {
		if !i.AreFieldsIdentical(document.InterfaceTypeDefinitions[ref].FieldsDefinition.Refs) {
			r.StopWithExternalErr(operationreport.ErrDuplicateValueTypesMustBeIdenticalToFederate(name))
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindInterfaceTypeDefinition, Ref: ref})
	} else {
		r.valueTypeSet[name] = NewInterfaceValueType(document, ref)
	}
	r.lastInterfaceRef = ref
}

func (r *removeDuplicateFieldedValueTypesVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.rootNodesToRemove != nil {
		r.document.DeleteRootNodes(r.rootNodesToRemove)
	}
}
