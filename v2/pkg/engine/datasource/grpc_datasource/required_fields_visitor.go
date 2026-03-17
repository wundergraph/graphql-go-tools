package grpcdatasource

import (
	"errors"
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
)

type requiredFieldVisitorConfig struct {
	// includeMemberType indicates if the member type should be included in the message.
	includeMemberType bool
	// skipFieldResolvers indicates if the field resolvers should be skipped.
	skipFieldResolvers bool
	// referenceNestedMessages instrcuts the visitor the format the messages names to reference inner protobuf messages.
	// Inner messages have an fqn like "MyMessage.NestedMessage"
	referenceNestedMessages bool
}

// requiredFieldsVisitor is a visitor that visits the required fields of a message.
type requiredFieldsVisitor struct {
	operation  *ast.Document
	definition *ast.Document

	walker              *astvisitor.Walker
	message             *RPCMessage
	fieldDefinitionRefs []int

	mapping *GRPCMapping
	planCtx *rpcPlanningContext

	messageAncestors []*RPCMessage

	skipFieldResolvers      bool
	referenceNestedMessages bool
}

// newRequiredFieldsVisitor creates a new requiredFieldsVisitor.
// It registers the visitor with the walker and returns it.
func newRequiredFieldsVisitor(walker *astvisitor.Walker, message *RPCMessage, mapping *GRPCMapping) *requiredFieldsVisitor {
	visitor := &requiredFieldsVisitor{
		walker:              walker,
		message:             message,
		mapping:             mapping,
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
		includeMemberType:       true,
		skipFieldResolvers:      false,
		referenceNestedMessages: false,
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
	r.referenceNestedMessages = options.referenceNestedMessages

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

	// Create a planCtx scoped to this walk's operation document so that methods
	// like lastResponseField and newMessageFromSelectionSet use the correct
	// document refs (the visitor walks a fragment doc that may differ from the
	// outer planner's operation).
	r.planCtx = newRPCPlanningContext(operation, definition, r.mapping)
}

func (r *requiredFieldsVisitor) enterNestedField(ref int, inlineFragmentRef int) bool {
	lastField := r.planCtx.lastResponseField(r.message, inlineFragmentRef)
	if lastField == nil {
		return false
	}

	if lastField.Message == nil {
		lastField.Message = r.planCtx.newMessageFromSelectionSet(r.walker.EnclosingTypeDefinition, ref)
	}

	r.messageAncestors = append(r.messageAncestors, r.message)
	if r.referenceNestedMessages {
		lastField.Message.Name = r.formatNestedMessageName(lastField.Message.Name)
	}
	r.message = lastField.Message
	return true
}

// EnterSelectionSet implements astvisitor.SelectionSetVisitor.
func (r *requiredFieldsVisitor) EnterSelectionSet(ref int) {
	// If we don't select on a field, we can return.
	if r.walker.Ancestor().Kind != ast.NodeKindField {
		return
	}

	// Determine which inline fragment directly contains the field we are about
	// to descend into. When entering a field's selection set, the walker Ancestors
	// are: [..., (maybe inline fragment), parent selection set, field].
	// Ancestors[-3] is therefore the inline fragment directly wrapping the field, if any.
	inlineFragmentRef := inlineFragmentRefFromAncestors(r.walker.Ancestors)
	if !r.enterNestedField(ref, inlineFragmentRef) {
		return
	}

	if err := r.handleCompositeType(r.walker.EnclosingTypeDefinition); err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}
}

// LeaveSelectionSet implements astvisitor.SelectionSetVisitor.
func (r *requiredFieldsVisitor) LeaveSelectionSet(ref int) {
	if r.walker.Ancestor().Kind != ast.NodeKindField {
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
		r.walker.StopWithInternalErr(fmt.Errorf("RequiredFieldsVisitor: field definition not found for field %s", fieldName))
		return
	}

	if r.planCtx.isFieldResolver(ref, r.walker.InRootField()) && r.skipFieldResolvers {
		r.walker.SkipNode()
		return
	}

	field, err := r.planCtx.buildField(r.walker.EnclosingTypeDefinition.NameString(r.definition), fd, fieldName, "")
	if err != nil {
		r.walker.StopWithInternalErr(err)
		return
	}

	r.fieldDefinitionRefs = append(r.fieldDefinitionRefs, fd)
	// check if we are inside an inline fragment
	if ref := r.walker.ResolveInlineFragment(); ref != ast.InvalidRef {
		if r.message.FragmentFields == nil {
			r.message.FragmentFields = make(RPCFieldSelectionSet)
		}

		inlineFragmentName := r.operation.InlineFragmentTypeConditionNameString(ref)
		r.message.FragmentFields.Add(inlineFragmentName, field)
		return
	}

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
	r.message.Fields = nil

	return nil
}

// formatNestedMessageName formats the name of a nested message.
// It returns the full qualified name of the nested message.
func (r *requiredFieldsVisitor) formatNestedMessageName(name string) string {
	if len(r.messageAncestors) == 0 {
		return name
	}

	builder := strings.Builder{}
	builder.WriteString(r.messageAncestors[len(r.messageAncestors)-1].Name)
	builder.WriteString(".")
	builder.WriteString(name)

	return builder.String()
}
