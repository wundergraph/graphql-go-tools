package sdlmerge

import (
	"github.com/wundergraph/graphql-go-tools/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/pkg/operationreport"
)

type removeDuplicateFieldedSharedTypesVisitor struct {
	*astvisitor.Walker
	document          *ast.Document
	sharedTypeSet     map[string]fieldedSharedType
	rootNodesToRemove []ast.Node
	lastInputRef      int
	lastInterfaceRef  int
	lastObjectRef     int
}

func newRemoveDuplicateFieldedSharedTypesVisitor() *removeDuplicateFieldedSharedTypesVisitor {
	return &removeDuplicateFieldedSharedTypesVisitor{
		nil,
		nil,
		make(map[string]fieldedSharedType),
		nil,
		ast.InvalidRef,
		ast.InvalidRef,
		ast.InvalidRef,
	}
}

func (r *removeDuplicateFieldedSharedTypesVisitor) Register(walker *astvisitor.Walker) {
	r.Walker = walker
	walker.RegisterEnterDocumentVisitor(r)
	walker.RegisterEnterInputObjectTypeDefinitionVisitor(r)
	walker.RegisterEnterInterfaceTypeDefinitionVisitor(r)
	walker.RegisterEnterObjectTypeDefinitionVisitor(r)
	walker.RegisterLeaveDocumentVisitor(r)
}

func (r *removeDuplicateFieldedSharedTypesVisitor) EnterDocument(operation, _ *ast.Document) {
	r.document = operation
}

func (r *removeDuplicateFieldedSharedTypesVisitor) EnterInputObjectTypeDefinition(ref int) {
	if ref <= r.lastInputRef {
		return
	}
	name := r.document.InputObjectTypeDefinitionNameString(ref)
	refs := r.document.InputObjectTypeDefinitions[ref].InputFieldsDefinition.Refs
	input, exists := r.sharedTypeSet[name]
	if exists {
		if !input.areFieldsIdentical(refs) {
			r.StopWithExternalErr(operationreport.ErrSharedTypesMustBeIdenticalToFederate(name))
			return
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindInputObjectTypeDefinition, Ref: ref})
	} else {
		r.sharedTypeSet[name] = newFieldedSharedType(r.document, ast.NodeKindInputValueDefinition, refs)
	}
	r.lastInputRef = ref
}

func (r *removeDuplicateFieldedSharedTypesVisitor) EnterInterfaceTypeDefinition(ref int) {
	if ref <= r.lastInterfaceRef {
		return
	}
	name := r.document.InterfaceTypeDefinitionNameString(ref)
	interfaceType := r.document.InterfaceTypeDefinitions[ref]
	refs := interfaceType.FieldsDefinition.Refs
	iFace, exists := r.sharedTypeSet[name]
	if exists {
		if !iFace.areFieldsIdentical(refs) {
			r.StopWithExternalErr(operationreport.ErrSharedTypesMustBeIdenticalToFederate(name))
			return
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindInterfaceTypeDefinition, Ref: ref})
	} else {
		r.sharedTypeSet[name] = newFieldedSharedType(r.document, ast.NodeKindFieldDefinition, refs)
	}
	r.lastInterfaceRef = ref
}

func (r *removeDuplicateFieldedSharedTypesVisitor) EnterObjectTypeDefinition(ref int) {
	if ref <= r.lastObjectRef {
		return
	}
	name := r.document.ObjectTypeDefinitionNameString(ref)
	objectType := r.document.ObjectTypeDefinitions[ref]
	refs := objectType.FieldsDefinition.Refs
	object, exists := r.sharedTypeSet[name]
	if exists {
		if !object.areFieldsIdentical(refs) {
			r.StopWithExternalErr(operationreport.ErrSharedTypesMustBeIdenticalToFederate(name))
			return
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindObjectTypeDefinition, Ref: ref})
	} else {
		r.sharedTypeSet[name] = newFieldedSharedType(r.document, ast.NodeKindFieldDefinition, refs)
	}
	r.lastObjectRef = ref
}

func (r *removeDuplicateFieldedSharedTypesVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.rootNodesToRemove != nil {
		r.document.DeleteRootNodes(r.rootNodesToRemove)
	}
}
