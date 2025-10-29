package grpcdatasource

import (
	"errors"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

type requiredFieldVisitorConfig struct {
	// includeMemberType indicates if the member type should be included in the message.
	includeMemberType bool
	// skipFieldResolvers indicates if the field resolvers should be skipped.
	skipFieldResolvers bool
}

// requiredFieldsVisitor is a visitor that visits the required fields of a message.
type requiredFieldsVisitor struct {
	operation  *ast.Document
	definition *ast.Document

	walker              *astvisitor.Walker
	message             *RPCMessage
	fieldDefinitionRefs []int

	planCtx *rpcPlanningContext

	messageAncestors []*RPCMessage

	skipFieldResolvers bool
}

// newRequiredFieldsVisitor creates a new requiredFieldsVisitor.
// It registers the visitor with the walker and returns it.
func newRequiredFieldsVisitor(walker *astvisitor.Walker, message *RPCMessage, planCtx *rpcPlanningContext) *requiredFieldsVisitor {
	visitor := &requiredFieldsVisitor{
		walker:              walker,
		message:             message,
		planCtx:             planCtx,
		messageAncestors:    []*RPCMessage{},
		fieldDefinitionRefs: []int{},
	}

	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterEnterFieldVisitor(visitor)

	return visitor
}

// visitWithDefaults visits the required fields of a message.
// It creates a new document with the required fields and walks it.
// To achieve that we create a fragment with the required fields and walk it.
func (r *requiredFieldsVisitor) visitWithDefaults(definition *ast.Document, typeName, requiredFields string) error {
	return r.visit(definition, typeName, requiredFields, requiredFieldVisitorConfig{
		includeMemberType:  true,
		skipFieldResolvers: false,
	})
}

// visit visits the required fields of a message.
// The function can be provided with options to customize the visitor.
func (r *requiredFieldsVisitor) visit(definition *ast.Document, typeName, requiredFields string, options requiredFieldVisitorConfig) error {
	doc, report := plan.RequiredFieldsFragment(typeName, requiredFields, false)
	if report.HasErrors() {
		return report
	}

	if options.includeMemberType {
		r.message.MemberTypes = []string{typeName}
	}

	r.skipFieldResolvers = options.skipFieldResolvers

	r.walker.Walk(doc, definition, report)
	if report.HasErrors() {
		return report
	}

	return nil
}

// EnterDocument implements astvisitor.EnterDocumentVisitor.
func (r *requiredFieldsVisitor) EnterDocument(operation *ast.Document, definition *ast.Document) {
	if r.message == nil {
		r.walker.StopWithInternalErr(errors.New("unable to visit required fields. Message is required"))
		return
	}

	r.operation = operation
	r.definition = definition
}

// EnterSelectionSet implements astvisitor.SelectionSetVisitor.
func (r *requiredFieldsVisitor) EnterSelectionSet(ref int) {
	// Ignore the root selection set
	if r.walker.Ancestor().Kind == ast.NodeKindFragmentDefinition {
		return
	}

	if len(r.message.Fields) == 0 {
		r.walker.StopWithInternalErr(errors.New("cannot access last field: message has no fields"))
		return
	}

	lastField := &r.message.Fields[len(r.message.Fields)-1]
	if lastField.Message == nil {
		lastField.Message = r.planCtx.newMessageFromSelectionSet(r.walker.EnclosingTypeDefinition, ref)
	}

	r.messageAncestors = append(r.messageAncestors, r.message)
	r.message = lastField.Message

	if err := r.handleCompositeType(r.walker.EnclosingTypeDefinition); err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}
}

// LeaveSelectionSet implements astvisitor.SelectionSetVisitor.
func (r *requiredFieldsVisitor) LeaveSelectionSet(ref int) {
	if r.walker.Ancestor().Kind == ast.NodeKindFragmentDefinition {
		return
	}

	if len(r.messageAncestors) > 0 {
		r.message = r.messageAncestors[len(r.messageAncestors)-1]
		r.messageAncestors = r.messageAncestors[:len(r.messageAncestors)-1]
	}
}

// EnterField implements astvisitor.EnterFieldVisitor.
func (r *requiredFieldsVisitor) EnterField(ref int) {
	fieldName := r.operation.FieldNameString(ref)

	// prevent duplicate fields
	if r.message.Fields.Exists(fieldName, "") {
		return
	}

	fd, ok := r.walker.FieldDefinition(ref)
	if !ok {
		r.walker.StopWithInternalErr(fmt.Errorf("field definition not found for field %s", fieldName))
		return
	}

	if r.planCtx.isFieldResolver(ref, r.walker.InRootField()) && r.skipFieldResolvers {
		r.walker.SkipNode()
		return
	}

	field, err := r.planCtx.buildField(r.walker.EnclosingTypeDefinition, fd, fieldName, "")
	if err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	r.fieldDefinitionRefs = append(r.fieldDefinitionRefs, fd)
	r.message.Fields = append(r.message.Fields, field)
}

func (r *requiredFieldsVisitor) handleCompositeType(node ast.Node) error {
	if node.Ref < 0 {
		return nil
	}

	var (
		ok          bool
		oneOfType   OneOfType
		memberTypes []string
	)

	switch node.Kind {
	case ast.NodeKindField:
		return r.handleCompositeType(r.walker.EnclosingTypeDefinition)
	case ast.NodeKindInterfaceTypeDefinition:
		oneOfType = OneOfTypeInterface
		memberTypes, ok = r.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
		if !ok {
			return fmt.Errorf("interface type %s is not implemented by any object", r.definition.InterfaceTypeDefinitionNameString(node.Ref))
		}
	case ast.NodeKindUnionTypeDefinition:
		oneOfType = OneOfTypeUnion
		memberTypes, ok = r.definition.UnionTypeDefinitionMemberTypeNames(node.Ref)
		if !ok {
			return fmt.Errorf("union type %s is not defined", r.definition.UnionTypeDefinitionNameString(node.Ref))
		}
	default:
		return nil
	}

	r.message.OneOfType = oneOfType
	r.message.MemberTypes = memberTypes

	return nil
}
