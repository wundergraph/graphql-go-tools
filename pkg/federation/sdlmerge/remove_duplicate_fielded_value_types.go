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
	lastInputRef      int
	lastInterfaceRef  int
	lastObjectRef     int
}

func newRemoveDuplicateFieldedValueTypesVisitor() *removeDuplicateFieldedValueTypesVisitor {
	return &removeDuplicateFieldedValueTypesVisitor{
		nil,
		nil,
		make(map[string]FieldedValueType),
		nil,
		ast.InvalidRef,
		ast.InvalidRef,
		ast.InvalidRef,
	}
}

func (r *removeDuplicateFieldedValueTypesVisitor) Register(walker *astvisitor.Walker) {
	r.Walker = walker
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterEnterInputObjectTypeDefinitionVisitor(r)
	walker.RegisterEnterInterfaceTypeDefinitionVisitor(r)
	walker.RegisterEnterObjectTypeDefinitionVisitor(r)
	walker.RegisterLeaveDocumentVisitor(r)
}

func (r *removeDuplicateFieldedValueTypesVisitor) EnterDocument(operation, _ *ast.Document) {
	r.document = operation
}

func (r *removeDuplicateFieldedValueTypesVisitor) EnterInputObjectTypeDefinition(ref int) {
	if ref <= r.lastInputRef {
		return
	}
	document := r.document
	name := document.InputObjectTypeDefinitionNameString(ref)
	refs := document.InputObjectTypeDefinitions[ref].InputFieldsDefinition.Refs
	input, exists := r.valueTypeSet[name]
	if exists {
		if !input.AreFieldsIdentical(refs) {
			r.StopWithExternalErr(operationreport.ErrDuplicateValueTypesMustBeIdenticalToFederate(name))
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindInputObjectTypeDefinition, Ref: ref})
	} else {
		r.valueTypeSet[name] = NewFieldedValueType(document, ast.NodeKindInputValueDefinition, refs)
	}
	r.lastInputRef = ref
}

func (r *removeDuplicateFieldedValueTypesVisitor) EnterInterfaceTypeDefinition(ref int) {
	if ref <= r.lastInterfaceRef {
		return
	}
	document := r.document
	name := document.InterfaceTypeDefinitionNameString(ref)
	refs := document.InterfaceTypeDefinitions[ref].FieldsDefinition.Refs
	iFace, exists := r.valueTypeSet[name]
	if exists {
		if !iFace.AreFieldsIdentical(refs) {
			r.StopWithExternalErr(operationreport.ErrDuplicateValueTypesMustBeIdenticalToFederate(name))
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindInterfaceTypeDefinition, Ref: ref})
	} else {
		r.valueTypeSet[name] = NewFieldedValueType(document, ast.NodeKindFieldDefinition, refs)
	}
	r.lastInterfaceRef = ref
}

func (r *removeDuplicateFieldedValueTypesVisitor) EnterObjectTypeDefinition(ref int) {
	if ref <= r.lastObjectRef {
		return
	}
	document := r.document
	name := document.ObjectTypeDefinitionNameString(ref)
	refs := document.ObjectTypeDefinitions[ref].FieldsDefinition.Refs
	object, exists := r.valueTypeSet[name]
	if exists {
		if !object.AreFieldsIdentical(refs) {
			r.StopWithExternalErr(operationreport.ErrDuplicateValueTypesMustBeIdenticalToFederate(name))
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindObjectTypeDefinition, Ref: ref})
	} else {
		r.valueTypeSet[name] = NewFieldedValueType(document, ast.NodeKindFieldDefinition, refs)
	}
	r.lastObjectRef = ref
}

func (r *removeDuplicateFieldedValueTypesVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.rootNodesToRemove != nil {
		r.document.DeleteRootNodes(r.rootNodesToRemove)
	}
}
