package sdlmerge

import (
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type removeDuplicateFieldedSharedTypesVisitor struct {
	*astvisitor.Walker
	document          *ast.Document
	entitySet         map[string]bool
	sharedTypeSet     map[string]FieldedSharedType
	rootNodesToRemove []ast.Node
	lastInputRef      int
	lastInterfaceRef  int
	lastObjectRef     int
}

func newRemoveDuplicateFieldedSharedTypesVisitor() *removeDuplicateFieldedSharedTypesVisitor {
	return &removeDuplicateFieldedSharedTypesVisitor{
		nil,
		nil,
		make(map[string]bool),
		make(map[string]FieldedSharedType),
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
	document := r.document
	name := document.InputObjectTypeDefinitionNameString(ref)
	refs := document.InputObjectTypeDefinitions[ref].InputFieldsDefinition.Refs
	input, exists := r.sharedTypeSet[name]
	if exists {
		if !input.AreFieldsIdentical(refs) {
			r.StopWithExternalErr(operationreport.ErrSharedTypesMustBeIdenticalToFederate(name))
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindInputObjectTypeDefinition, Ref: ref})
	} else {
		r.sharedTypeSet[name] = NewFieldedSharedType(document, ast.NodeKindInputValueDefinition, refs)
	}
	r.lastInputRef = ref
}

func (r *removeDuplicateFieldedSharedTypesVisitor) EnterInterfaceTypeDefinition(ref int) {
	if ref <= r.lastInterfaceRef {
		return
	}
	document := r.document
	name := document.InterfaceTypeDefinitionNameString(ref)
	interfaceType := document.InterfaceTypeDefinitions[ref]
	r.checkForDuplicateEntity(interfaceType.HasDirectives, interfaceType.Directives.Refs, name)
	refs := interfaceType.FieldsDefinition.Refs
	iFace, exists := r.sharedTypeSet[name]
	if exists {
		if !iFace.AreFieldsIdentical(refs) {
			r.StopWithExternalErr(operationreport.ErrSharedTypesMustBeIdenticalToFederate(name))
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindInterfaceTypeDefinition, Ref: ref})
	} else {
		r.sharedTypeSet[name] = NewFieldedSharedType(document, ast.NodeKindFieldDefinition, refs)
	}
	r.lastInterfaceRef = ref
}

func (r *removeDuplicateFieldedSharedTypesVisitor) EnterObjectTypeDefinition(ref int) {
	if ref <= r.lastObjectRef {
		return
	}
	document := r.document
	name := document.ObjectTypeDefinitionNameString(ref)
	objectType := document.ObjectTypeDefinitions[ref]
	r.checkForDuplicateEntity(objectType.HasDirectives, objectType.Directives.Refs, name)
	refs := objectType.FieldsDefinition.Refs
	object, exists := r.sharedTypeSet[name]
	if exists {
		if !object.AreFieldsIdentical(refs) {
			r.StopWithExternalErr(operationreport.ErrSharedTypesMustBeIdenticalToFederate(name))
		}
		r.rootNodesToRemove = append(r.rootNodesToRemove, ast.Node{Kind: ast.NodeKindObjectTypeDefinition, Ref: ref})
	} else {
		r.sharedTypeSet[name] = NewFieldedSharedType(document, ast.NodeKindFieldDefinition, refs)
	}
	r.lastObjectRef = ref
}

func (r *removeDuplicateFieldedSharedTypesVisitor) LeaveDocument(_, _ *ast.Document) {
	if r.rootNodesToRemove != nil {
		r.document.DeleteRootNodes(r.rootNodesToRemove)
	}
}

func (r *removeDuplicateFieldedSharedTypesVisitor) checkForDuplicateEntity(hasDirectives bool, directiveRefs []int, name string) {
	if r.entitySet[name] {
		r.StopWithExternalErr(operationreport.ErrEntitiesMustNotBeSharedTypes(name))
	}
	if !hasDirectives {
		return
	}
	for _, directiveRef := range directiveRefs {
		if r.document.DirectiveNameString(directiveRef) == "key" {
			if _, exists := r.sharedTypeSet[name]; exists {
				r.StopWithExternalErr(operationreport.ErrEntitiesMustNotBeSharedTypes(name))
			}
			r.entitySet[name] = true
			return
		}
	}
}
