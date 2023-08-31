package astvisitor

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

var (
	ErrDocumentMustNotBeNil   = fmt.Errorf("document must not be nil")
	ErrDefinitionMustNotBeNil = fmt.Errorf("definition must not be nil when walking operations")
)

type SkipVisitors []int

func (s SkipVisitors) Allow(planner interface{}) bool {
	p, ok := planner.(VisitorIdentifier)
	if !ok {
		return true
	}
	currentID := p.ID()

	for _, skippedID := range s {
		if skippedID == currentID {
			return false
		}
	}
	return true
}

func newSkipVisitors(skips []int, planner interface{}, allowedToVisit bool) SkipVisitors {
	p, ok := planner.(VisitorIdentifier)
	if !ok {
		return skips
	}
	currentID := p.ID()

	for k, skippedID := range skips {
		if skippedID == currentID {
			if allowedToVisit {
				// if visiting was allowed explicitly we have to remove the skip for the nested nodes
				newSkips := make([]int, 0, len(skips))
				newSkips = append(newSkips, skips[:k]...)
				newSkips = append(newSkips, skips[k+1:]...)
				return newSkips
			} else {
				return skips // nothing to do as we already skipped this visitor
			}
		}
	}
	if allowedToVisit {
		return skips
	}
	newSkips := make([]int, 0, len(skips)+1)
	newSkips = append(newSkips, skips...)
	return append(newSkips, currentID) // add new skipped planner index
}

// Walker orchestrates the process of walking an AST and calling all registered callbacks
// Always use NewWalker to instantiate a new Walker
type Walker struct {
	// Ancestors is the slice of Nodes to the current Node in a callback
	// don't keep a reference to this slice, always copy it if you want to work with it after the callback returned
	Ancestors []ast.Node
	// Path is the slice of PathItems leading to the current Node
	// don't keep a reference to this slice, always copy it if you want to work with it after the callback returned
	Path ast.Path
	// EnclosingTypeDefinition is the TypeDefinition Node of the parent object of the current callback
	// e.g. if the current callback is a Field the EnclosingTypeDefinition will be the TypeDefinition of the parent object of such Field
	EnclosingTypeDefinition ast.Node
	// SelectionsBefore is the slice of references to selections before the current selection
	// This is only valid when inside a SelectionSet
	SelectionsBefore []int
	// SelectionsAfter is the slice of references to selections before the current selection
	// This is only valid when inside a SelectionSet
	SelectionsAfter []int
	// Report is the object to collect errors when walking the AST
	Report          *operationreport.Report
	CurrentRef      int
	CurrentKind     ast.NodeKind
	document        *ast.Document
	definition      *ast.Document
	visitors        visitors
	Depth           int
	typeDefinitions []ast.Node
	stop            bool
	skip            bool
	revisit         bool
	filter          VisitorFilter
	deferred        []func()
}

// NewWalker returns a fully initialized Walker
func NewWalker(ancestorSize int) Walker {
	return Walker{
		Ancestors:       make([]ast.Node, 0, ancestorSize),
		Path:            make([]ast.PathItem, 0, ancestorSize),
		typeDefinitions: make([]ast.Node, 0, ancestorSize),
		deferred:        make([]func(), 0, 8),
	}
}

type (
	// EnterOperationDefinitionVisitor is the callback when the walker enters an operation definition
	EnterOperationDefinitionVisitor interface {
		// EnterOperationDefinition gets called when the walker enters an operation definition
		// ref is the reference to the operation definition on the AST
		EnterOperationDefinition(ref int)
	}
	// LeaveOperationDefinitionVisitor is the callback when the walker leaves an operation definition
	LeaveOperationDefinitionVisitor interface {
		// LeaveOperationDefinition gets called when the walker leaves an operation definition
		// ref is the reference to the operation definition on the AST
		LeaveOperationDefinition(ref int)
	}
	// OperationDefinitionVisitor is the callback when the walker enters or leaves an operation
	OperationDefinitionVisitor interface {
		EnterOperationDefinitionVisitor
		LeaveOperationDefinitionVisitor
	}
	// EnterSelectionSetVisitor is the callback when the walker enters a selection set
	EnterSelectionSetVisitor interface {
		// EnterSelectionSet gets called when the walker enters a selection set
		// ref is the reference to the selection set on the AST
		EnterSelectionSet(ref int)
	}
	// LeaveSelectionSetVisitor is the callback when the walker leaves a selection set visitor
	LeaveSelectionSetVisitor interface {
		// LeaveSelectionSet gets called when the walker leaves a selection set
		// ref is the reference to the selection set on the AST
		LeaveSelectionSet(ref int)
	}
	// SelectionSetVisitor is the callback when the walker enters or leaves a selection set
	SelectionSetVisitor interface {
		EnterSelectionSetVisitor
		LeaveSelectionSetVisitor
	}
	// EnterFieldVisitor is the callback when the walker enters a field
	EnterFieldVisitor interface {
		// EnterField gets called when the walker enters a field
		// ref is the reference to the selection set on the AST
		EnterField(ref int)
	}
	// LeaveFieldVisitor is the callback when the walker leaves a field
	LeaveFieldVisitor interface {
		// LeaveField gets called when the walker leaves a field
		// ref is the reference to the selection set on the AST
		LeaveField(ref int)
	}
	// FieldVisitor is the callback when the walker enters or leaves a field
	FieldVisitor interface {
		EnterFieldVisitor
		LeaveFieldVisitor
	}
	// EnterArgumentVisitor is the callback when the walker enters an argument
	EnterArgumentVisitor interface {
		// EnterArgument gets called when the walker enters an argument
		// ref is the reference to the selection set on the AST
		EnterArgument(ref int)
	}
	// LeaveArgumentVisitor is the callback when the walker leaves an argument
	LeaveArgumentVisitor interface {
		// LeaveArgument gets called when the walker leaves an argument
		// ref is the reference to the selection set on the AST
		LeaveArgument(ref int)
	}
	// ArgumentVisitor is the callback when the walker enters or leaves an argument
	ArgumentVisitor interface {
		EnterArgumentVisitor
		LeaveArgumentVisitor
	}
	// EnterFragmentSpreadVisitor is the callback when the walker enters a fragment spread
	EnterFragmentSpreadVisitor interface {
		// EnterFragmentSpread gets called when the walker enters a fragment spread
		// ref is the reference to the selection set on the AST
		EnterFragmentSpread(ref int)
	}
	// LeaveFragmentSpreadVisitor is the callback when the walker leaves a fragment spread
	LeaveFragmentSpreadVisitor interface {
		// LeaveFragmentSpread gets called when the walker leaves a fragment spread
		// ref is the reference to the selection set on the AST
		LeaveFragmentSpread(ref int)
	}
	// FragmentSpreadVisitor is the callback when the walker enters or leaves a fragment spread
	FragmentSpreadVisitor interface {
		EnterFragmentSpreadVisitor
		LeaveFragmentSpreadVisitor
	}
	// EnterInlineFragmentVisitor is the callback when the walker enters an inline fragment
	EnterInlineFragmentVisitor interface {
		// EnterInlineFragment gets called when the walker enters an inline fragment
		// ref is the reference to the selection set on the AST
		EnterInlineFragment(ref int)
	}
	// LeaveInlineFragmentVisitor is the callback when the walker leaves an inline fragment
	LeaveInlineFragmentVisitor interface {
		// LeaveInlineFragment gets called when the walker leaves an inline fragment
		// ref is the reference to the selection set on the AST
		LeaveInlineFragment(ref int)
	}
	// InlineFragmentVisitor is the callback when the walker enters or leaves an inline fragment
	InlineFragmentVisitor interface {
		EnterInlineFragmentVisitor
		LeaveInlineFragmentVisitor
	}
	// EnterFragmentDefinitionVisitor is the callback when the walker enters a fragment definition
	EnterFragmentDefinitionVisitor interface {
		// EnterFragmentDefinition gets called when the walker enters a fragment definition
		// ref is the reference to the selection set on the AST
		EnterFragmentDefinition(ref int)
	}
	// LeaveFragmentDefinitionVisitor is the callback when the walker leaves a fragment definition
	LeaveFragmentDefinitionVisitor interface {
		// LeaveFragmentDefinition gets called when the walker leaves a fragment definition
		// ref is the reference to the selection set on the AST
		LeaveFragmentDefinition(ref int)
	}
	// FragmentDefinitionVisitor is the callback when the walker enters or leaves a fragment definition
	FragmentDefinitionVisitor interface {
		EnterFragmentDefinitionVisitor
		LeaveFragmentDefinitionVisitor
	}
	// EnterVariableDefinitionVisitor is the callback when the walker enters a variable definition
	EnterVariableDefinitionVisitor interface {
		// EnterVariableDefinition gets called when the walker enters a variable definition
		// ref is the reference to the selection set on the AST
		EnterVariableDefinition(ref int)
	}
	// LeaveVariableDefinitionVisitor is the callback when the walker leaves a variable definition
	LeaveVariableDefinitionVisitor interface {
		// LeaveVariableDefinition gets called when the walker leaves a variable definition
		// ref is the reference to the selection set on the AST
		LeaveVariableDefinition(ref int)
	}
	// VariableDefinitionVisitor is the callback when the walker enters or leaves a variable definition
	VariableDefinitionVisitor interface {
		EnterVariableDefinitionVisitor
		LeaveVariableDefinitionVisitor
	}
	// EnterDirectiveVisitor is the callback when the walker enters a directive
	EnterDirectiveVisitor interface {
		// EnterDirective gets called when the walker enters a directive
		// ref is the reference to the selection set on the AST
		EnterDirective(ref int)
	}
	// LeaveDirectiveVisitor is the callback when the walker leaves a directive
	LeaveDirectiveVisitor interface {
		// LeaveDirective gets called when the walker leaves a directive
		// ref is the reference to the selection set on the AST
		LeaveDirective(ref int)
	}
	// DirectiveVisitor is the callback when the walker enters or leaves a directive
	DirectiveVisitor interface {
		EnterDirectiveVisitor
		LeaveDirectiveVisitor
	}
	// EnterObjectTypeDefinitionVisitor is the callback when the walker enters an object type definition
	EnterObjectTypeDefinitionVisitor interface {
		// EnterObjectTypeDefinition gets called when the walker enters an object type definition
		// ref is the reference to the selection set on the AST
		EnterObjectTypeDefinition(ref int)
	}
	// LeaveObjectTypeDefinitionVisitor is the callback when the walker leaves an object type definition
	LeaveObjectTypeDefinitionVisitor interface {
		// LeaveObjectTypeDefinition gets called when the walker leaves an object type definition
		// ref is the reference to the selection set on the AST
		LeaveObjectTypeDefinition(ref int)
	}
	// ObjectTypeDefinitionVisitor is the callback when the walker enters or leaves an object type definition
	ObjectTypeDefinitionVisitor interface {
		EnterObjectTypeDefinitionVisitor
		LeaveObjectTypeDefinitionVisitor
	}
	// EnterObjectTypeExtensionVisitor is the callback when the walker enters an object type extension
	EnterObjectTypeExtensionVisitor interface {
		// EnterObjectTypeExtension gets called when the walker enters an object type extension
		// ref is the reference to the selection set on the AST
		EnterObjectTypeExtension(ref int)
	}
	// LeaveObjectTypeExtensionVisitor is the callback when the walker leaves an object type extension
	LeaveObjectTypeExtensionVisitor interface {
		// LeaveObjectTypeExtension gets called when the walker leaves an object type extension
		// ref is the reference to the selection set on the AST
		LeaveObjectTypeExtension(ref int)
	}
	// ObjectTypeExtensionVisitor is the callback when the walker enters or leaves an object type extension
	ObjectTypeExtensionVisitor interface {
		EnterObjectTypeExtensionVisitor
		LeaveObjectTypeExtensionVisitor
	}
	// EnterFieldDefinitionVisitor is the callback when the walker enters a field definition
	EnterFieldDefinitionVisitor interface {
		// EnterFieldDefinition gets called when the walker enters a field definition
		// ref is the reference to the selection set on the AST
		EnterFieldDefinition(ref int)
	}
	// LeaveFieldDefinitionVisitor is the callback when the walker leaves a field definition
	LeaveFieldDefinitionVisitor interface {
		// LeaveFieldDefinition gets called when the walker leaves a field definition
		// ref is the reference to the selection set on the AST
		LeaveFieldDefinition(ref int)
	}
	// FieldDefinitionVisitor is the callback when the walker enters or leaves a field definition
	FieldDefinitionVisitor interface {
		EnterFieldDefinitionVisitor
		LeaveFieldDefinitionVisitor
	}
	// EnterInputValueDefinitionVisitor is the callback when the walker enters an input value definition
	EnterInputValueDefinitionVisitor interface {
		// EnterInputValueDefinition gets called when the walker enters an input value definition
		// ref is the reference to the selection set on the AST
		EnterInputValueDefinition(ref int)
	}
	// LeaveInputValueDefinitionVisitor is the callback when the walker leaves an input value definition
	LeaveInputValueDefinitionVisitor interface {
		// LeaveInputValueDefinition gets called when the walker leaves an input value definition
		// ref is the reference to the selection set on the AST
		LeaveInputValueDefinition(ref int)
	}
	// InputValueDefinitionVisitor is the callback when the walker enters or leaves an input value definition
	InputValueDefinitionVisitor interface {
		EnterInputValueDefinitionVisitor
		LeaveInputValueDefinitionVisitor
	}
	// EnterInterfaceTypeDefinitionVisitor is the callback when the walker enters an interface type definition
	EnterInterfaceTypeDefinitionVisitor interface {
		// EnterInterfaceTypeDefinition gets called when the walker enters an interface type definition
		// ref is the reference to the selection set on the AST
		EnterInterfaceTypeDefinition(ref int)
	}
	// LeaveInterfaceTypeDefinitionVisitor is the callback when the walker leaves an interface type definition
	LeaveInterfaceTypeDefinitionVisitor interface {
		// LeaveInterfaceTypeDefinition gets called when the walker leaves an interface type definition
		// ref is the reference to the selection set on the AST
		LeaveInterfaceTypeDefinition(ref int)
	}
	// InterfaceTypeDefinitionVisitor is the callback when the walker enters or leaves an interface type definition
	InterfaceTypeDefinitionVisitor interface {
		EnterInterfaceTypeDefinitionVisitor
		LeaveInterfaceTypeDefinitionVisitor
	}
	// EnterInterfaceTypeExtensionVisitor is the callback when the walker enters an interface type extension
	EnterInterfaceTypeExtensionVisitor interface {
		// EnterInterfaceTypeExtension gets called when the walker enters an interface type extension
		// ref is the reference to the selection set on the AST
		EnterInterfaceTypeExtension(ref int)
	}
	// LeaveInterfaceTypeExtensionVisitor is the callback when the walker leaves an interface type extension
	LeaveInterfaceTypeExtensionVisitor interface {
		// LeaveInterfaceTypeExtension gets called when the walker leaves an interface type extension
		// ref is the reference to the selection set on the AST
		LeaveInterfaceTypeExtension(ref int)
	}
	// InterfaceTypeExtensionVisitor is the callback when the walker enters or leaves an interface type extension
	InterfaceTypeExtensionVisitor interface {
		EnterInterfaceTypeExtensionVisitor
		LeaveInterfaceTypeExtensionVisitor
	}
	// EnterScalarTypeDefinitionVisitor is the callback when the walker enters a scalar type definition
	EnterScalarTypeDefinitionVisitor interface {
		// EnterScalarTypeDefinition gets called when the walker enters a scalar type definition
		// ref is the reference to the selection set on the AST
		EnterScalarTypeDefinition(ref int)
	}
	// LeaveScalarTypeDefinitionVisitor is the callback when the walker leaves a scalar type definition
	LeaveScalarTypeDefinitionVisitor interface {
		// LeaveScalarTypeDefinition gets called when the walker leaves a scalar type definition
		// ref is the reference to the selection set on the AST
		LeaveScalarTypeDefinition(ref int)
	}
	// ScalarTypeDefinitionVisitor is the callback when the walker enters or leaves a scalar type definition
	ScalarTypeDefinitionVisitor interface {
		EnterScalarTypeDefinitionVisitor
		LeaveScalarTypeDefinitionVisitor
	}
	// EnterScalarTypeExtensionVisitor is the callback when the walker enters a scalar type extension
	EnterScalarTypeExtensionVisitor interface {
		// EnterScalarTypeExtension gets called when the walker enters a scalar type extension
		// ref is the reference to the selection set on the AST
		EnterScalarTypeExtension(ref int)
	}
	// LeaveScalarTypeExtensionVisitor is the callback when the walker leaves a scalar type extension
	LeaveScalarTypeExtensionVisitor interface {
		// LeaveScalarTypeExtension gets called when the walker leaves a scalar type extension
		// ref is the reference to the selection set on the AST
		LeaveScalarTypeExtension(ref int)
	}
	// ScalarTypeExtensionVisitor is the callback when the walker enters or leaves a scalar type extension
	ScalarTypeExtensionVisitor interface {
		EnterScalarTypeExtensionVisitor
		LeaveScalarTypeExtensionVisitor
	}
	// EnterUnionTypeDefinitionVisitor is the callback when the walker enters a union type definition
	EnterUnionTypeDefinitionVisitor interface {
		// EnterUnionTypeDefinition gets called when the walker enters a union type definition
		// ref is the reference to the selection set on the AST
		EnterUnionTypeDefinition(ref int)
	}
	// LeaveUnionTypeDefinitionVisitor is the callback when the walker leaves a union type definition
	LeaveUnionTypeDefinitionVisitor interface {
		// LeaveUnionTypeDefinition gets called when the walker leaves a union type definition
		// ref is the reference to the selection set on the AST
		LeaveUnionTypeDefinition(ref int)
	}
	// UnionTypeDefinitionVisitor is the callback when the walker enters or leaves a union type definition
	UnionTypeDefinitionVisitor interface {
		EnterUnionTypeDefinitionVisitor
		LeaveUnionTypeDefinitionVisitor
	}
	// EnterUnionTypeExtensionVisitor is the callback when the walker enters a union type extension
	EnterUnionTypeExtensionVisitor interface {
		// EnterUnionTypeExtension gets called when the walker enters a union type extension
		// ref is the reference to the selection set on the AST
		EnterUnionTypeExtension(ref int)
	}
	// LeaveUnionTypeExtensionVisitor is the callback when the walker leaves a union type extension
	LeaveUnionTypeExtensionVisitor interface {
		// LeaveUnionTypeExtension gets called when the walker leaves a union type extension
		// ref is the reference to the selection set on the AST
		LeaveUnionTypeExtension(ref int)
	}
	// UnionTypeExtensionVisitor is the callback when the walker enters or leaves a union type extension
	UnionTypeExtensionVisitor interface {
		EnterUnionTypeExtensionVisitor
		LeaveUnionTypeExtensionVisitor
	}
	// EnterUnionMemberTypeVisitor is the callback when the walker enters a union member type
	EnterUnionMemberTypeVisitor interface {
		// EnterUnionMemberType gets called when the walker enters a union member type
		// ref is the reference to the selection set on the AST
		EnterUnionMemberType(ref int)
	}
	// LeaveUnionMemberTypeVisitor is the callback when the walker leaves a union member type
	LeaveUnionMemberTypeVisitor interface {
		// LeaveUnionMemberType gets called when the walker leaves a union member type
		// ref is the reference to the selection set on the AST
		LeaveUnionMemberType(ref int)
	}
	// UnionMemberTypeVisitor is the callback when the walker enters or leaves a union member type
	UnionMemberTypeVisitor interface {
		EnterUnionMemberTypeVisitor
		LeaveUnionMemberTypeVisitor
	}
	// EnterEnumTypeDefinitionVisitor is the callback when the walker enters an enum type definition
	EnterEnumTypeDefinitionVisitor interface {
		// EnterEnumTypeDefinition gets called when the walker enters an enum type definition
		// ref is the reference to the selection set on the AST
		EnterEnumTypeDefinition(ref int)
	}
	// LeaveEnumTypeDefinitionVisitor is the callback when the walker leaves an enum type definition
	LeaveEnumTypeDefinitionVisitor interface {
		// LeaveEnumTypeDefinition gets called when the walker leaves an enum type definition
		// ref is the reference to the selection set on the AST
		LeaveEnumTypeDefinition(ref int)
	}
	// EnumTypeDefinitionVisitor is the callback when the walker enters or leaves an enum type definition
	EnumTypeDefinitionVisitor interface {
		EnterEnumTypeDefinitionVisitor
		LeaveEnumTypeDefinitionVisitor
	}
	// EnterEnumTypeExtensionVisitor is the callback when the walker enters an enum type extension
	EnterEnumTypeExtensionVisitor interface {
		// EnterEnumTypeExtension gets called when the walker enters an enum type extension
		// ref is the reference to the selection set on the AST
		EnterEnumTypeExtension(ref int)
	}
	// LeaveEnumTypeExtensionVisitor is the callback when the walker leaves an enum type extension
	LeaveEnumTypeExtensionVisitor interface {
		// LeaveEnumTypeExtension gets called when the walker leaves an enum type extension
		// ref is the reference to the selection set on the AST
		LeaveEnumTypeExtension(ref int)
	}
	// EnumTypeExtensionVisitor is the callback when the walker enters or leaves an enum type extension
	EnumTypeExtensionVisitor interface {
		EnterEnumTypeExtensionVisitor
		LeaveEnumTypeExtensionVisitor
	}
	// EnterEnumValueDefinitionVisitor is the callback when the walker enters an enum value definition
	EnterEnumValueDefinitionVisitor interface {
		// EnterEnumValueDefinition gets called when the walker enters an enum value definition
		// ref is the reference to the selection set on the AST
		EnterEnumValueDefinition(ref int)
	}
	// LeaveEnumValueDefinitionVisitor is the callback when the walker leaves an enum value definition
	LeaveEnumValueDefinitionVisitor interface {
		// LeaveEnumValueDefinition gets called when the walker leaves an enum value definition
		// ref is the reference to the selection set on the AST
		LeaveEnumValueDefinition(ref int)
	}
	// EnumValueDefinitionVisitor is the callback when the walker enters or leaves an enum value definition
	EnumValueDefinitionVisitor interface {
		EnterEnumValueDefinitionVisitor
		LeaveEnumValueDefinitionVisitor
	}
	// EnterInputObjectTypeDefinitionVisitor is the callback when the walker enters an input object type definition
	EnterInputObjectTypeDefinitionVisitor interface {
		// EnterInputObjectTypeDefinition gets called when the walker enters an input object type definition
		// ref is the reference to the selection set on the AST
		EnterInputObjectTypeDefinition(ref int)
	}
	// LeaveInputObjectTypeDefinitionVisitor is the callback when the walker leaves an input object type definition
	LeaveInputObjectTypeDefinitionVisitor interface {
		// LeaveInputObjectTypeDefinition gets called when the walker leaves an input object type definition
		// ref is the reference to the selection set on the AST
		LeaveInputObjectTypeDefinition(ref int)
	}
	// InputObjectTypeDefinitionVisitor is the callback when the walker enters or leaves an input object type definition
	InputObjectTypeDefinitionVisitor interface {
		EnterInputObjectTypeDefinitionVisitor
		LeaveInputObjectTypeDefinitionVisitor
	}
	// EnterInputObjectTypeExtensionVisitor is the callback when the walker enters an input object type extension
	EnterInputObjectTypeExtensionVisitor interface {
		// EnterInputObjectTypeExtension gets called when the walker enters an input object type extension
		// ref is the reference to the selection set on the AST
		EnterInputObjectTypeExtension(ref int)
	}
	// LeaveInputObjectTypeExtensionVisitor is the callback when the walker leaves an input object type extension
	LeaveInputObjectTypeExtensionVisitor interface {
		// LeaveInputObjectTypeExtension gets called when the walker leaves an input object type extension
		// ref is the reference to the selection set on the AST
		LeaveInputObjectTypeExtension(ref int)
	}
	// InputObjectTypeExtensionVisitor is the callback when the walker enters or leaves an input object type extension
	InputObjectTypeExtensionVisitor interface {
		EnterInputObjectTypeExtensionVisitor
		LeaveInputObjectTypeExtensionVisitor
	}
	// EnterDirectiveDefinitionVisitor is the callback when the walker enters a directive definition
	EnterDirectiveDefinitionVisitor interface {
		// EnterDirectiveDefinition gets called when the walker enters a directive definition
		// ref is the reference to the selection set on the AST
		EnterDirectiveDefinition(ref int)
	}
	// LeaveDirectiveDefinitionVisitor is the callback when the walker leaves a directive definition
	LeaveDirectiveDefinitionVisitor interface {
		// LeaveDirectiveDefinition gets called when the walker leaves a directive definition
		// ref is the reference to the selection set on the AST
		LeaveDirectiveDefinition(ref int)
	}
	// DirectiveDefinitionVisitor is the callback when the walker enters or leaves a directive definition
	DirectiveDefinitionVisitor interface {
		EnterDirectiveDefinitionVisitor
		LeaveDirectiveDefinitionVisitor
	}
	// EnterDirectiveLocationVisitor is the callback when the walker enters a directive location
	EnterDirectiveLocationVisitor interface {
		// EnterDirectiveLocation gets called when the walker enters a directive location
		// ref is the reference to the selection set on the AST
		EnterDirectiveLocation(location ast.DirectiveLocation)
	}
	// LeaveDirectiveLocationVisitor is the callback when the walker leaves a directive location
	LeaveDirectiveLocationVisitor interface {
		// LeaveDirectiveLocation gets called when the walker leaves a directive location
		// ref is the reference to the selection set on the AST
		LeaveDirectiveLocation(location ast.DirectiveLocation)
	}
	// DirectiveLocationVisitor is the callback when the walker enters or leaves a directive location
	DirectiveLocationVisitor interface {
		EnterDirectiveLocationVisitor
		LeaveDirectiveLocationVisitor
	}
	// EnterSchemaDefinitionVisitor is the callback when the walker enters a schema definition
	EnterSchemaDefinitionVisitor interface {
		// EnterSchemaDefinition gets called when the walker enters a schema definition
		// ref is the reference to the selection set on the AST
		EnterSchemaDefinition(ref int)
	}
	// LeaveSchemaDefinitionVisitor is the callback when the walker leaves a schema definition
	LeaveSchemaDefinitionVisitor interface {
		// LeaveSchemaDefinition gets called when the walker leaves a schema definition
		// ref is the reference to the selection set on the AST
		LeaveSchemaDefinition(ref int)
	}
	// SchemaDefinitionVisitor is the callback when the walker enters or leaves a schema definition
	SchemaDefinitionVisitor interface {
		EnterSchemaDefinitionVisitor
		LeaveSchemaDefinitionVisitor
	}
	// EnterSchemaExtensionVisitor is the callback when the walker enters a schema extension
	EnterSchemaExtensionVisitor interface {
		// EnterSchemaExtension gets called when the walker enters a schema extension
		// ref is the reference to the selection set on the AST
		EnterSchemaExtension(ref int)
	}
	// LeaveSchemaExtensionVisitor is the callback when the walker leaves a schema extension
	LeaveSchemaExtensionVisitor interface {
		// LeaveSchemaExtension gets called when the walker leaves a schema extension
		// ref is the reference to the selection set on the AST
		LeaveSchemaExtension(ref int)
	}
	// SchemaExtensionVisitor is the callback when the walker enters or leaves a schema extension
	SchemaExtensionVisitor interface {
		EnterSchemaExtensionVisitor
		LeaveSchemaExtensionVisitor
	}
	// EnterRootOperationTypeDefinitionVisitor is the callback when the walker enters a root operation type definition
	EnterRootOperationTypeDefinitionVisitor interface {
		// EnterRootOperationTypeDefinition gets called when the walker enters a root operation type definition
		// ref is the reference to the selection set on the AST
		EnterRootOperationTypeDefinition(ref int)
	}
	// LeaveRootOperationTypeDefinitionVisitor is the callback when the walker leaves a root operation type definition
	LeaveRootOperationTypeDefinitionVisitor interface {
		// LeaveRootOperationTypeDefinition gets called when the walker leaves a root operation type definition
		// ref is the reference to the selection set on the AST
		LeaveRootOperationTypeDefinition(ref int)
	}
	// RootOperationTypeDefinitionVisitor is the callback when the walker enters or leaves a root operation type definition
	RootOperationTypeDefinitionVisitor interface {
		EnterRootOperationTypeDefinitionVisitor
		LeaveRootOperationTypeDefinitionVisitor
	}
	// TypeSystemVisitor is the callback when the walker enters or leaves any of the type definitions
	TypeSystemVisitor interface {
		ObjectTypeDefinitionVisitor
		ObjectTypeExtensionVisitor
		FieldDefinitionVisitor
		InputValueDefinitionVisitor
		InterfaceTypeDefinitionVisitor
		InterfaceTypeExtensionVisitor
		ScalarTypeDefinitionVisitor
		ScalarTypeExtensionVisitor
		UnionTypeDefinitionVisitor
		UnionTypeExtensionVisitor
		UnionMemberTypeVisitor
		EnumTypeDefinitionVisitor
		EnumTypeExtensionVisitor
		EnumValueDefinitionVisitor
		InputObjectTypeDefinitionVisitor
		InputObjectTypeExtensionVisitor
		DirectiveDefinitionVisitor
		DirectiveLocationVisitor
		SchemaDefinitionVisitor
		SchemaExtensionVisitor
		RootOperationTypeDefinitionVisitor
	}
	// ExecutableVisitor is the callback when the walker enters or leaves any of the executable definitions
	ExecutableVisitor interface {
		OperationDefinitionVisitor
		SelectionSetVisitor
		FieldVisitor
		ArgumentVisitor
		FragmentSpreadVisitor
		InlineFragmentVisitor
		FragmentDefinitionVisitor
		VariableDefinitionVisitor
		DirectiveVisitor
	}
	// EnterDocumentVisitor is the callback when the walker enters a document
	EnterDocumentVisitor interface {
		// EnterDocument gets called when the walker enters a document
		EnterDocument(operation, definition *ast.Document)
	}
	LeaveDocumentVisitor interface {
		// LeaveDocument gets called when the walker leaves a document
		LeaveDocument(operation, definition *ast.Document)
	}
	// DocumentVisitor is the callback when the walker enters or leaves a document
	DocumentVisitor interface {
		EnterDocumentVisitor
		LeaveDocumentVisitor
	}
	// AllNodesVisitor is the callback when the walker enters or leaves any Node
	AllNodesVisitor interface {
		DocumentVisitor
		TypeSystemVisitor
		ExecutableVisitor
	}
	// VisitorFilter can be defined to prevent specific visitors from getting invoked
	VisitorFilter interface {
		AllowVisitor(kind VisitorKind, ref int, visitor interface{}, ancestorSkip SkipVisitors) bool
	}

	VisitorIdentifier interface {
		ID() int
		SetID(id int)
	}
)

type VisitorKind int

const (
	EnterOperation VisitorKind = iota + 1
	LeaveOperation
	EnterSelectionSet
	LeaveSelectionSet
	EnterField
	LeaveField
	EnterArgument
	LeaveArgument
	EnterFragmentSpread
	LeaveFragmentSpread
	EnterInlineFragment
	LeaveInlineFragment
	EnterFragmentDefinition
	LeaveFragmentDefinition
	EnterDocument
	LeaveDocument
	EnterVariableDefinition
	LeaveVariableDefinition
	EnterDirective
	LeaveDirective
	EnterObjectTypeDefinition
	LeaveObjectTypeDefinition
	EnterFieldDefinition
	LeaveFieldDefinition
	EnterInputValueDefinition
	LeaveInputValueDefinition
	EnterInterfaceTypeDefinition
	LeaveInterfaceTypeDefinition
	EnterInterfaceTypeExtension
	LeaveInterfaceTypeExtension
	EnterObjectTypeExtension
	LeaveObjectTypeExtension
	EnterScalarTypeDefinition
	LeaveScalarTypeDefinition
	EnterScalarTypeExtension
	LeaveScalarTypeExtension
	EnterUnionTypeDefinition
	LeaveUnionTypeDefinition
	EnterUnionTypeExtension
	LeaveUnionTypeExtension
	EnterUnionMemberType
	LeaveUnionMemberType
	EnterEnumTypeDefinition
	LeaveEnumTypeDefinition
	EnterEnumTypeExtension
	LeaveEnumTypeExtension
	EnterEnumValueDefinition
	LeaveEnumValueDefinition
	EnterInputObjectTypeDefinition
	LeaveInputObjectTypeDefinition
	EnterInputObjectTypeExtension
	LeaveInputObjectTypeExtension
	EnterDirectiveDefinition
	LeaveDirectiveDefinition
	EnterDirectiveLocation
	LeaveDirectiveLocation
	EnterSchemaDefinition
	LeaveSchemaDefinition
	EnterSchemaExtension
	LeaveSchemaExtension
	EnterRootOperationTypeDefinition
	LeaveRootOperationTypeDefinition
)

type visitors struct {
	enterOperation                   []EnterOperationDefinitionVisitor
	leaveOperation                   []LeaveOperationDefinitionVisitor
	enterSelectionSet                []EnterSelectionSetVisitor
	leaveSelectionSet                []LeaveSelectionSetVisitor
	enterField                       []EnterFieldVisitor
	leaveField                       []LeaveFieldVisitor
	enterArgument                    []EnterArgumentVisitor
	leaveArgument                    []LeaveArgumentVisitor
	enterFragmentSpread              []EnterFragmentSpreadVisitor
	leaveFragmentSpread              []LeaveFragmentSpreadVisitor
	enterInlineFragment              []EnterInlineFragmentVisitor
	leaveInlineFragment              []LeaveInlineFragmentVisitor
	enterFragmentDefinition          []EnterFragmentDefinitionVisitor
	leaveFragmentDefinition          []LeaveFragmentDefinitionVisitor
	enterDocument                    []EnterDocumentVisitor
	leaveDocument                    []LeaveDocumentVisitor
	enterVariableDefinition          []EnterVariableDefinitionVisitor
	leaveVariableDefinition          []LeaveVariableDefinitionVisitor
	enterDirective                   []EnterDirectiveVisitor
	leaveDirective                   []LeaveDirectiveVisitor
	enterObjectTypeDefinition        []EnterObjectTypeDefinitionVisitor
	leaveObjectTypeDefinition        []LeaveObjectTypeDefinitionVisitor
	enterFieldDefinition             []EnterFieldDefinitionVisitor
	leaveFieldDefinition             []LeaveFieldDefinitionVisitor
	enterInputValueDefinition        []EnterInputValueDefinitionVisitor
	leaveInputValueDefinition        []LeaveInputValueDefinitionVisitor
	enterInterfaceTypeDefinition     []EnterInterfaceTypeDefinitionVisitor
	leaveInterfaceTypeDefinition     []LeaveInterfaceTypeDefinitionVisitor
	enterInterfaceTypeExtension      []EnterInterfaceTypeExtensionVisitor
	leaveInterfaceTypeExtension      []LeaveInterfaceTypeExtensionVisitor
	enterObjectTypeExtension         []EnterObjectTypeExtensionVisitor
	leaveObjectTypeExtension         []LeaveObjectTypeExtensionVisitor
	enterScalarTypeDefinition        []EnterScalarTypeDefinitionVisitor
	leaveScalarTypeDefinition        []LeaveScalarTypeDefinitionVisitor
	enterScalarTypeExtension         []EnterScalarTypeExtensionVisitor
	leaveScalarTypeExtension         []LeaveScalarTypeExtensionVisitor
	enterUnionTypeDefinition         []EnterUnionTypeDefinitionVisitor
	leaveUnionTypeDefinition         []LeaveUnionTypeDefinitionVisitor
	enterUnionTypeExtension          []EnterUnionTypeExtensionVisitor
	leaveUnionTypeExtension          []LeaveUnionTypeExtensionVisitor
	enterUnionMemberType             []EnterUnionMemberTypeVisitor
	leaveUnionMemberType             []LeaveUnionMemberTypeVisitor
	enterEnumTypeDefinition          []EnterEnumTypeDefinitionVisitor
	leaveEnumTypeDefinition          []LeaveEnumTypeDefinitionVisitor
	enterEnumTypeExtension           []EnterEnumTypeExtensionVisitor
	leaveEnumTypeExtension           []LeaveEnumTypeExtensionVisitor
	enterEnumValueDefinition         []EnterEnumValueDefinitionVisitor
	leaveEnumValueDefinition         []LeaveEnumValueDefinitionVisitor
	enterInputObjectTypeDefinition   []EnterInputObjectTypeDefinitionVisitor
	leaveInputObjectTypeDefinition   []LeaveInputObjectTypeDefinitionVisitor
	enterInputObjectTypeExtension    []EnterInputObjectTypeExtensionVisitor
	leaveInputObjectTypeExtension    []LeaveInputObjectTypeExtensionVisitor
	enterDirectiveDefinition         []EnterDirectiveDefinitionVisitor
	leaveDirectiveDefinition         []LeaveDirectiveDefinitionVisitor
	enterDirectiveLocation           []EnterDirectiveLocationVisitor
	leaveDirectiveLocation           []LeaveDirectiveLocationVisitor
	enterSchemaDefinition            []EnterSchemaDefinitionVisitor
	leaveSchemaDefinition            []LeaveSchemaDefinitionVisitor
	enterSchemaExtension             []EnterSchemaExtensionVisitor
	leaveSchemaExtension             []LeaveSchemaExtensionVisitor
	enterRootOperationTypeDefinition []EnterRootOperationTypeDefinitionVisitor
	leaveRootOperationTypeDefinition []LeaveRootOperationTypeDefinitionVisitor
}

// ResetVisitors empties all registered visitors / unregisters all callbacks
func (w *Walker) ResetVisitors() {
	w.visitors.enterOperation = w.visitors.enterOperation[:0]
	w.visitors.leaveOperation = w.visitors.leaveOperation[:0]
	w.visitors.enterSelectionSet = w.visitors.enterSelectionSet[:0]
	w.visitors.leaveSelectionSet = w.visitors.leaveSelectionSet[:0]
	w.visitors.enterField = w.visitors.enterField[:0]
	w.visitors.leaveField = w.visitors.leaveField[:0]
	w.visitors.enterArgument = w.visitors.enterArgument[:0]
	w.visitors.leaveArgument = w.visitors.leaveArgument[:0]
	w.visitors.enterFragmentSpread = w.visitors.enterFragmentSpread[:0]
	w.visitors.leaveFragmentSpread = w.visitors.leaveFragmentSpread[:0]
	w.visitors.enterInlineFragment = w.visitors.enterInlineFragment[:0]
	w.visitors.leaveInlineFragment = w.visitors.leaveInlineFragment[:0]
	w.visitors.enterFragmentDefinition = w.visitors.enterFragmentDefinition[:0]
	w.visitors.leaveFragmentDefinition = w.visitors.leaveFragmentDefinition[:0]
	w.visitors.enterDocument = w.visitors.enterDocument[:0]
	w.visitors.leaveDocument = w.visitors.leaveDocument[:0]
	w.visitors.enterVariableDefinition = w.visitors.enterVariableDefinition[:0]
	w.visitors.leaveVariableDefinition = w.visitors.leaveVariableDefinition[:0]
	w.visitors.enterDirective = w.visitors.enterDirective[:0]
	w.visitors.leaveDirective = w.visitors.leaveDirective[:0]
	w.visitors.enterObjectTypeDefinition = w.visitors.enterObjectTypeDefinition[:0]
	w.visitors.leaveObjectTypeDefinition = w.visitors.leaveObjectTypeDefinition[:0]
	w.visitors.enterFieldDefinition = w.visitors.enterFieldDefinition[:0]
	w.visitors.leaveFieldDefinition = w.visitors.leaveFieldDefinition[:0]
	w.visitors.enterInputValueDefinition = w.visitors.enterInputValueDefinition[:0]
	w.visitors.leaveInputValueDefinition = w.visitors.leaveInputValueDefinition[:0]
	w.visitors.enterInterfaceTypeDefinition = w.visitors.enterInterfaceTypeDefinition[:0]
	w.visitors.leaveInterfaceTypeDefinition = w.visitors.leaveInterfaceTypeDefinition[:0]
	w.visitors.enterInterfaceTypeExtension = w.visitors.enterInterfaceTypeExtension[:0]
	w.visitors.leaveInterfaceTypeExtension = w.visitors.leaveInterfaceTypeExtension[:0]
	w.visitors.enterObjectTypeExtension = w.visitors.enterObjectTypeExtension[:0]
	w.visitors.leaveObjectTypeExtension = w.visitors.leaveObjectTypeExtension[:0]
	w.visitors.enterScalarTypeDefinition = w.visitors.enterScalarTypeDefinition[:0]
	w.visitors.leaveScalarTypeDefinition = w.visitors.leaveScalarTypeDefinition[:0]
	w.visitors.enterScalarTypeExtension = w.visitors.enterScalarTypeExtension[:0]
	w.visitors.leaveScalarTypeExtension = w.visitors.leaveScalarTypeExtension[:0]
	w.visitors.enterUnionTypeDefinition = w.visitors.enterUnionTypeDefinition[:0]
	w.visitors.leaveUnionTypeDefinition = w.visitors.leaveUnionTypeDefinition[:0]
	w.visitors.enterUnionTypeExtension = w.visitors.enterUnionTypeExtension[:0]
	w.visitors.leaveUnionTypeExtension = w.visitors.leaveUnionTypeExtension[:0]
	w.visitors.enterUnionMemberType = w.visitors.enterUnionMemberType[:0]
	w.visitors.leaveUnionMemberType = w.visitors.leaveUnionMemberType[:0]
	w.visitors.enterEnumTypeDefinition = w.visitors.enterEnumTypeDefinition[:0]
	w.visitors.leaveEnumTypeDefinition = w.visitors.leaveEnumTypeDefinition[:0]
	w.visitors.enterEnumTypeExtension = w.visitors.enterEnumTypeExtension[:0]
	w.visitors.leaveEnumTypeExtension = w.visitors.leaveEnumTypeExtension[:0]
	w.visitors.enterEnumValueDefinition = w.visitors.enterEnumValueDefinition[:0]
	w.visitors.leaveEnumValueDefinition = w.visitors.leaveEnumValueDefinition[:0]
	w.visitors.enterInputObjectTypeDefinition = w.visitors.enterInputObjectTypeDefinition[:0]
	w.visitors.leaveInputObjectTypeDefinition = w.visitors.leaveInputObjectTypeDefinition[:0]
	w.visitors.enterInputObjectTypeExtension = w.visitors.enterInputObjectTypeExtension[:0]
	w.visitors.leaveInputObjectTypeExtension = w.visitors.leaveInputObjectTypeExtension[:0]
	w.visitors.enterDirectiveDefinition = w.visitors.enterDirectiveDefinition[:0]
	w.visitors.leaveDirectiveDefinition = w.visitors.leaveDirectiveDefinition[:0]
	w.visitors.enterDirectiveLocation = w.visitors.enterDirectiveLocation[:0]
	w.visitors.leaveDirectiveLocation = w.visitors.leaveDirectiveLocation[:0]
	w.visitors.enterSchemaDefinition = w.visitors.enterSchemaDefinition[:0]
	w.visitors.leaveSchemaDefinition = w.visitors.leaveSchemaDefinition[:0]
	w.visitors.enterSchemaExtension = w.visitors.enterSchemaExtension[:0]
	w.visitors.leaveSchemaExtension = w.visitors.leaveSchemaExtension[:0]
	w.visitors.enterRootOperationTypeDefinition = w.visitors.enterRootOperationTypeDefinition[:0]
	w.visitors.leaveRootOperationTypeDefinition = w.visitors.leaveRootOperationTypeDefinition[:0]
	w.deferred = w.deferred[:0]
}

func (w *Walker) setCurrent(kind ast.NodeKind, ref int) {
	w.CurrentKind = kind
	w.CurrentRef = ref
}

func (w *Walker) RegisterExecutableVisitor(visitor ExecutableVisitor) {
	w.RegisterOperationDefinitionVisitor(visitor)
	w.RegisterSelectionSetVisitor(visitor)
	w.RegisterFieldVisitor(visitor)
	w.RegisterArgumentVisitor(visitor)
	w.RegisterFragmentSpreadVisitor(visitor)
	w.RegisterInlineFragmentVisitor(visitor)
	w.RegisterFragmentDefinitionVisitor(visitor)
	w.RegisterVariableDefinitionVisitor(visitor)
	w.RegisterDirectiveVisitor(visitor)
}

func (w *Walker) RegisterTypeSystemVisitor(visitor TypeSystemVisitor) {
	w.RegisterObjectTypeDefinitionVisitor(visitor)
	w.RegisterObjectTypeExtensionVisitor(visitor)
	w.RegisterFieldDefinitionVisitor(visitor)
	w.RegisterInputValueDefinitionVisitor(visitor)
	w.RegisterInterfaceTypeDefinitionVisitor(visitor)
	w.RegisterInterfaceTypeExtensionVisitor(visitor)
	w.RegisterScalarTypeDefinitionVisitor(visitor)
	w.RegisterScalarTypeExtensionVisitor(visitor)
	w.RegisterUnionTypeDefinitionVisitor(visitor)
	w.RegisterUnionTypeExtensionVisitor(visitor)
	w.RegisterUnionMemberTypeVisitor(visitor)
	w.RegisterEnumTypeDefinitionVisitor(visitor)
	w.RegisterEnumTypeExtensionVisitor(visitor)
	w.RegisterEnumValueDefinitionVisitor(visitor)
	w.RegisterInputObjectTypeDefinitionVisitor(visitor)
	w.RegisterInputObjectTypeExtensionVisitor(visitor)
	w.RegisterDirectiveDefinitionVisitor(visitor)
	w.RegisterDirectiveLocationVisitor(visitor)
	w.RegisterSchemaDefinitionVisitor(visitor)
	w.RegisterSchemaExtensionVisitor(visitor)
	w.RegisterRootOperationTypeDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterRootOperationTypeDefinitionVisitor(visitor EnterRootOperationTypeDefinitionVisitor) {
	w.visitors.enterRootOperationTypeDefinition = append(w.visitors.enterRootOperationTypeDefinition, visitor)
}

func (w *Walker) RegisterLeaveRootOperationTypeDefinitionVisitor(visitor LeaveRootOperationTypeDefinitionVisitor) {
	w.visitors.leaveRootOperationTypeDefinition = append(w.visitors.leaveRootOperationTypeDefinition, visitor)
}

func (w *Walker) RegisterRootOperationTypeDefinitionVisitor(visitor RootOperationTypeDefinitionVisitor) {
	w.RegisterEnterRootOperationTypeDefinitionVisitor(visitor)
	w.RegisterLeaveRootOperationTypeDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterSchemaDefinitionVisitor(visitor EnterSchemaDefinitionVisitor) {
	w.visitors.enterSchemaDefinition = append(w.visitors.enterSchemaDefinition, visitor)
}

func (w *Walker) RegisterLeaveSchemaDefinitionVisitor(visitor LeaveSchemaDefinitionVisitor) {
	w.visitors.leaveSchemaDefinition = append(w.visitors.leaveSchemaDefinition, visitor)
}

func (w *Walker) RegisterSchemaDefinitionVisitor(visitor SchemaDefinitionVisitor) {
	w.RegisterEnterSchemaDefinitionVisitor(visitor)
	w.RegisterLeaveSchemaDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterSchemaExtensionVisitor(visitor EnterSchemaExtensionVisitor) {
	w.visitors.enterSchemaExtension = append(w.visitors.enterSchemaExtension, visitor)
}

func (w *Walker) RegisterLeaveSchemaExtensionVisitor(visitor LeaveSchemaExtensionVisitor) {
	w.visitors.leaveSchemaExtension = append(w.visitors.leaveSchemaExtension, visitor)
}

func (w *Walker) RegisterSchemaExtensionVisitor(visitor SchemaExtensionVisitor) {
	w.RegisterEnterSchemaExtensionVisitor(visitor)
	w.RegisterLeaveSchemaExtensionVisitor(visitor)
}

func (w *Walker) RegisterEnterDirectiveLocationVisitor(visitor EnterDirectiveLocationVisitor) {
	w.visitors.enterDirectiveLocation = append(w.visitors.enterDirectiveLocation, visitor)
}

func (w *Walker) RegisterLeaveDirectiveLocationVisitor(visitor LeaveDirectiveLocationVisitor) {
	w.visitors.leaveDirectiveLocation = append(w.visitors.leaveDirectiveLocation, visitor)
}

func (w *Walker) RegisterDirectiveLocationVisitor(visitor DirectiveLocationVisitor) {
	w.RegisterEnterDirectiveLocationVisitor(visitor)
	w.RegisterLeaveDirectiveLocationVisitor(visitor)
}

func (w *Walker) RegisterEnterDirectiveDefinitionVisitor(visitor EnterDirectiveDefinitionVisitor) {
	w.visitors.enterDirectiveDefinition = append(w.visitors.enterDirectiveDefinition, visitor)
}

func (w *Walker) RegisterLeaveDirectiveDefinitionVisitor(visitor LeaveDirectiveDefinitionVisitor) {
	w.visitors.leaveDirectiveDefinition = append(w.visitors.leaveDirectiveDefinition, visitor)
}

func (w *Walker) RegisterDirectiveDefinitionVisitor(visitor DirectiveDefinitionVisitor) {
	w.RegisterEnterDirectiveDefinitionVisitor(visitor)
	w.RegisterLeaveDirectiveDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterUnionMemberTypeVisitor(visitor EnterUnionMemberTypeVisitor) {
	w.visitors.enterUnionMemberType = append(w.visitors.enterUnionMemberType, visitor)
}

func (w *Walker) RegisterLeaveUnionMemberTypeVisitor(visitor LeaveUnionMemberTypeVisitor) {
	w.visitors.leaveUnionMemberType = append(w.visitors.leaveUnionMemberType, visitor)
}

func (w *Walker) RegisterUnionMemberTypeVisitor(visitor UnionMemberTypeVisitor) {
	w.RegisterEnterUnionMemberTypeVisitor(visitor)
	w.RegisterLeaveUnionMemberTypeVisitor(visitor)
}

func (w *Walker) RegisterEnterInputObjectTypeDefinitionVisitor(visitor EnterInputObjectTypeDefinitionVisitor) {
	w.visitors.enterInputObjectTypeDefinition = append(w.visitors.enterInputObjectTypeDefinition, visitor)
}

func (w *Walker) RegisterLeaveInputObjectTypeDefinitionVisitor(visitor LeaveInputObjectTypeDefinitionVisitor) {
	w.visitors.leaveInputObjectTypeDefinition = append(w.visitors.leaveInputObjectTypeDefinition, visitor)
}

func (w *Walker) RegisterInputObjectTypeDefinitionVisitor(visitor InputObjectTypeDefinitionVisitor) {
	w.RegisterEnterInputObjectTypeDefinitionVisitor(visitor)
	w.RegisterLeaveInputObjectTypeDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterInputObjectTypeExtensionVisitor(visitor EnterInputObjectTypeExtensionVisitor) {
	w.visitors.enterInputObjectTypeExtension = append(w.visitors.enterInputObjectTypeExtension, visitor)
}

func (w *Walker) RegisterLeaveInputObjectTypeExtensionVisitor(visitor LeaveInputObjectTypeExtensionVisitor) {
	w.visitors.leaveInputObjectTypeExtension = append(w.visitors.leaveInputObjectTypeExtension, visitor)
}

func (w *Walker) RegisterInputObjectTypeExtensionVisitor(visitor InputObjectTypeExtensionVisitor) {
	w.RegisterEnterInputObjectTypeExtensionVisitor(visitor)
	w.RegisterLeaveInputObjectTypeExtensionVisitor(visitor)
}

func (w *Walker) RegisterEnterEnumTypeDefinitionVisitor(visitor EnterEnumTypeDefinitionVisitor) {
	w.visitors.enterEnumTypeDefinition = append(w.visitors.enterEnumTypeDefinition, visitor)
}

func (w *Walker) RegisterLeaveEnumTypeDefinitionVisitor(visitor LeaveEnumTypeDefinitionVisitor) {
	w.visitors.leaveEnumTypeDefinition = append(w.visitors.leaveEnumTypeDefinition, visitor)
}

func (w *Walker) RegisterEnumTypeDefinitionVisitor(visitor EnumTypeDefinitionVisitor) {
	w.RegisterEnterEnumTypeDefinitionVisitor(visitor)
	w.RegisterLeaveEnumTypeDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterEnumTypeExtensionVisitor(visitor EnterEnumTypeExtensionVisitor) {
	w.visitors.enterEnumTypeExtension = append(w.visitors.enterEnumTypeExtension, visitor)
}

func (w *Walker) RegisterLeaveEnumTypeExtensionVisitor(visitor LeaveEnumTypeExtensionVisitor) {
	w.visitors.leaveEnumTypeExtension = append(w.visitors.leaveEnumTypeExtension, visitor)
}

func (w *Walker) RegisterEnumTypeExtensionVisitor(visitor EnumTypeExtensionVisitor) {
	w.RegisterEnterEnumTypeExtensionVisitor(visitor)
	w.RegisterLeaveEnumTypeExtensionVisitor(visitor)
}

func (w *Walker) RegisterEnterEnumValueDefinitionVisitor(visitor EnterEnumValueDefinitionVisitor) {
	w.visitors.enterEnumValueDefinition = append(w.visitors.enterEnumValueDefinition, visitor)
}

func (w *Walker) RegisterLeaveEnumValueDefinitionVisitor(visitor LeaveEnumValueDefinitionVisitor) {
	w.visitors.leaveEnumValueDefinition = append(w.visitors.leaveEnumValueDefinition, visitor)
}

func (w *Walker) RegisterEnumValueDefinitionVisitor(visitor EnumValueDefinitionVisitor) {
	w.RegisterEnterEnumValueDefinitionVisitor(visitor)
	w.RegisterLeaveEnumValueDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterUnionTypeDefinitionVisitor(visitor EnterUnionTypeDefinitionVisitor) {
	w.visitors.enterUnionTypeDefinition = append(w.visitors.enterUnionTypeDefinition, visitor)
}

func (w *Walker) RegisterLeaveUnionTypeDefinitionVisitor(visitor LeaveUnionTypeDefinitionVisitor) {
	w.visitors.leaveUnionTypeDefinition = append(w.visitors.leaveUnionTypeDefinition, visitor)
}

func (w *Walker) RegisterUnionTypeDefinitionVisitor(visitor UnionTypeDefinitionVisitor) {
	w.RegisterEnterUnionTypeDefinitionVisitor(visitor)
	w.RegisterLeaveUnionTypeDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterUnionTypeExtensionVisitor(visitor EnterUnionTypeExtensionVisitor) {
	w.visitors.enterUnionTypeExtension = append(w.visitors.enterUnionTypeExtension, visitor)
}

func (w *Walker) RegisterLeaveUnionTypeExtensionVisitor(visitor LeaveUnionTypeExtensionVisitor) {
	w.visitors.leaveUnionTypeExtension = append(w.visitors.leaveUnionTypeExtension, visitor)
}

func (w *Walker) RegisterUnionTypeExtensionVisitor(visitor UnionTypeExtensionVisitor) {
	w.RegisterEnterUnionTypeExtensionVisitor(visitor)
	w.RegisterLeaveUnionTypeExtensionVisitor(visitor)
}

func (w *Walker) RegisterEnterScalarTypeExtensionVisitor(visitor EnterScalarTypeExtensionVisitor) {
	w.visitors.enterScalarTypeExtension = append(w.visitors.enterScalarTypeExtension, visitor)
}

func (w *Walker) RegisterLeaveScalarTypeExtensionVisitor(visitor LeaveScalarTypeExtensionVisitor) {
	w.visitors.leaveScalarTypeExtension = append(w.visitors.leaveScalarTypeExtension, visitor)
}

func (w *Walker) RegisterScalarTypeExtensionVisitor(visitor ScalarTypeExtensionVisitor) {
	w.RegisterEnterScalarTypeExtensionVisitor(visitor)
	w.RegisterLeaveScalarTypeExtensionVisitor(visitor)
}

func (w *Walker) RegisterEnterScalarTypeDefinitionVisitor(visitor EnterScalarTypeDefinitionVisitor) {
	w.visitors.enterScalarTypeDefinition = append(w.visitors.enterScalarTypeDefinition, visitor)
}

func (w *Walker) RegisterLeaveScalarTypeDefinitionVisitor(visitor LeaveScalarTypeDefinitionVisitor) {
	w.visitors.leaveScalarTypeDefinition = append(w.visitors.leaveScalarTypeDefinition, visitor)
}

func (w *Walker) RegisterScalarTypeDefinitionVisitor(visitor ScalarTypeDefinitionVisitor) {
	w.RegisterEnterScalarTypeDefinitionVisitor(visitor)
	w.RegisterLeaveScalarTypeDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterInterfaceTypeExtensionVisitor(visitor EnterInterfaceTypeExtensionVisitor) {
	w.visitors.enterInterfaceTypeExtension = append(w.visitors.enterInterfaceTypeExtension, visitor)
}

func (w *Walker) RegisterLeaveInterfaceTypeExtensionVisitor(visitor LeaveInterfaceTypeExtensionVisitor) {
	w.visitors.leaveInterfaceTypeExtension = append(w.visitors.leaveInterfaceTypeExtension, visitor)
}

func (w *Walker) RegisterInterfaceTypeExtensionVisitor(visitor InterfaceTypeExtensionVisitor) {
	w.RegisterEnterInterfaceTypeExtensionVisitor(visitor)
	w.RegisterLeaveInterfaceTypeExtensionVisitor(visitor)
}

func (w *Walker) RegisterEnterInterfaceTypeDefinitionVisitor(visitor EnterInterfaceTypeDefinitionVisitor) {
	w.visitors.enterInterfaceTypeDefinition = append(w.visitors.enterInterfaceTypeDefinition, visitor)
}

func (w *Walker) RegisterLeaveInterfaceTypeDefinitionVisitor(visitor LeaveInterfaceTypeDefinitionVisitor) {
	w.visitors.leaveInterfaceTypeDefinition = append(w.visitors.leaveInterfaceTypeDefinition, visitor)
}

func (w *Walker) RegisterInterfaceTypeDefinitionVisitor(visitor InterfaceTypeDefinitionVisitor) {
	w.RegisterEnterInterfaceTypeDefinitionVisitor(visitor)
	w.RegisterLeaveInterfaceTypeDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterInputValueDefinitionVisitor(visitor EnterInputValueDefinitionVisitor) {
	w.visitors.enterInputValueDefinition = append(w.visitors.enterInputValueDefinition, visitor)
}

func (w *Walker) RegisterLeaveInputValueDefinitionVisitor(visitor LeaveInputValueDefinitionVisitor) {
	w.visitors.leaveInputValueDefinition = append(w.visitors.leaveInputValueDefinition, visitor)
}

func (w *Walker) RegisterInputValueDefinitionVisitor(visitor InputValueDefinitionVisitor) {
	w.RegisterEnterInputValueDefinitionVisitor(visitor)
	w.RegisterLeaveInputValueDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterFieldDefinitionVisitor(visitor EnterFieldDefinitionVisitor) {
	w.visitors.enterFieldDefinition = append(w.visitors.enterFieldDefinition, visitor)
}

func (w *Walker) RegisterLeaveFieldDefinitionVisitor(visitor LeaveFieldDefinitionVisitor) {
	w.visitors.leaveFieldDefinition = append(w.visitors.leaveFieldDefinition, visitor)
}

func (w *Walker) RegisterFieldDefinitionVisitor(visitor FieldDefinitionVisitor) {
	w.RegisterEnterFieldDefinitionVisitor(visitor)
	w.RegisterLeaveFieldDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterObjectTypeExtensionVisitor(visitor EnterObjectTypeExtensionVisitor) {
	w.visitors.enterObjectTypeExtension = append(w.visitors.enterObjectTypeExtension, visitor)
}

func (w *Walker) RegisterLeaveObjectTypeExtensionVisitor(visitor LeaveObjectTypeExtensionVisitor) {
	w.visitors.leaveObjectTypeExtension = append(w.visitors.leaveObjectTypeExtension, visitor)
}

func (w *Walker) RegisterObjectTypeExtensionVisitor(visitor ObjectTypeExtensionVisitor) {
	w.RegisterEnterObjectTypeExtensionVisitor(visitor)
	w.RegisterLeaveObjectTypeExtensionVisitor(visitor)
}

func (w *Walker) RegisterEnterObjectTypeDefinitionVisitor(visitor EnterObjectTypeDefinitionVisitor) {
	w.visitors.enterObjectTypeDefinition = append(w.visitors.enterObjectTypeDefinition, visitor)
}

func (w *Walker) RegisterLeaveObjectTypeDefinitionVisitor(visitor LeaveObjectTypeDefinitionVisitor) {
	w.visitors.leaveObjectTypeDefinition = append(w.visitors.leaveObjectTypeDefinition, visitor)
}

func (w *Walker) RegisterObjectTypeDefinitionVisitor(visitor ObjectTypeDefinitionVisitor) {
	w.RegisterEnterObjectTypeDefinitionVisitor(visitor)
	w.RegisterLeaveObjectTypeDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterFieldVisitor(visitor EnterFieldVisitor) {
	w.visitors.enterField = append(w.visitors.enterField, visitor)
}

func (w *Walker) RegisterLeaveFieldVisitor(visitor LeaveFieldVisitor) {
	w.visitors.leaveField = append(w.visitors.leaveField, visitor)
}

func (w *Walker) RegisterFieldVisitor(visitor FieldVisitor) {
	w.RegisterEnterFieldVisitor(visitor)
	w.RegisterLeaveFieldVisitor(visitor)
}

func (w *Walker) RegisterEnterSelectionSetVisitor(visitor EnterSelectionSetVisitor) {
	w.visitors.enterSelectionSet = append(w.visitors.enterSelectionSet, visitor)
}

func (w *Walker) RegisterLeaveSelectionSetVisitor(visitor LeaveSelectionSetVisitor) {
	w.visitors.leaveSelectionSet = append(w.visitors.leaveSelectionSet, visitor)
}

func (w *Walker) RegisterSelectionSetVisitor(visitor SelectionSetVisitor) {
	w.RegisterEnterSelectionSetVisitor(visitor)
	w.RegisterLeaveSelectionSetVisitor(visitor)
}

func (w *Walker) RegisterEnterArgumentVisitor(visitor EnterArgumentVisitor) {
	w.visitors.enterArgument = append(w.visitors.enterArgument, visitor)
}

func (w *Walker) RegisterLeaveArgumentVisitor(visitor LeaveArgumentVisitor) {
	w.visitors.leaveArgument = append(w.visitors.leaveArgument, visitor)
}

func (w *Walker) RegisterArgumentVisitor(visitor ArgumentVisitor) {
	w.RegisterEnterArgumentVisitor(visitor)
	w.RegisterLeaveArgumentVisitor(visitor)
}

func (w *Walker) RegisterEnterFragmentSpreadVisitor(visitor EnterFragmentSpreadVisitor) {
	w.visitors.enterFragmentSpread = append(w.visitors.enterFragmentSpread, visitor)
}

func (w *Walker) RegisterLeaveFragmentSpreadVisitor(visitor LeaveFragmentSpreadVisitor) {
	w.visitors.leaveFragmentSpread = append(w.visitors.leaveFragmentSpread, visitor)
}

func (w *Walker) RegisterFragmentSpreadVisitor(visitor FragmentSpreadVisitor) {
	w.RegisterEnterFragmentSpreadVisitor(visitor)
	w.RegisterLeaveFragmentSpreadVisitor(visitor)
}

func (w *Walker) RegisterEnterInlineFragmentVisitor(visitor EnterInlineFragmentVisitor) {
	w.visitors.enterInlineFragment = append(w.visitors.enterInlineFragment, visitor)
}

func (w *Walker) RegisterLeaveInlineFragmentVisitor(visitor LeaveInlineFragmentVisitor) {
	w.visitors.leaveInlineFragment = append(w.visitors.leaveInlineFragment, visitor)
}

func (w *Walker) RegisterInlineFragmentVisitor(visitor InlineFragmentVisitor) {
	w.RegisterEnterInlineFragmentVisitor(visitor)
	w.RegisterLeaveInlineFragmentVisitor(visitor)
}

func (w *Walker) RegisterEnterFragmentDefinitionVisitor(visitor EnterFragmentDefinitionVisitor) {
	w.visitors.enterFragmentDefinition = append(w.visitors.enterFragmentDefinition, visitor)
}

func (w *Walker) RegisterLeaveFragmentDefinitionVisitor(visitor LeaveFragmentDefinitionVisitor) {
	w.visitors.leaveFragmentDefinition = append(w.visitors.leaveFragmentDefinition, visitor)
}

func (w *Walker) RegisterFragmentDefinitionVisitor(visitor FragmentDefinitionVisitor) {
	w.RegisterEnterFragmentDefinitionVisitor(visitor)
	w.RegisterLeaveFragmentDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterVariableDefinitionVisitor(visitor EnterVariableDefinitionVisitor) {
	w.visitors.enterVariableDefinition = append(w.visitors.enterVariableDefinition, visitor)
}

func (w *Walker) RegisterLeaveVariableDefinitionVisitor(visitor LeaveVariableDefinitionVisitor) {
	w.visitors.leaveVariableDefinition = append(w.visitors.leaveVariableDefinition, visitor)
}

func (w *Walker) RegisterVariableDefinitionVisitor(visitor VariableDefinitionVisitor) {
	w.RegisterEnterVariableDefinitionVisitor(visitor)
	w.RegisterLeaveVariableDefinitionVisitor(visitor)
}

func (w *Walker) RegisterEnterOperationVisitor(visitor EnterOperationDefinitionVisitor) {
	w.visitors.enterOperation = append(w.visitors.enterOperation, visitor)
}

func (w *Walker) RegisterLeaveOperationVisitor(visitor LeaveOperationDefinitionVisitor) {
	w.visitors.leaveOperation = append(w.visitors.leaveOperation, visitor)
}

func (w *Walker) RegisterOperationDefinitionVisitor(visitor OperationDefinitionVisitor) {
	w.RegisterEnterOperationVisitor(visitor)
	w.RegisterLeaveOperationVisitor(visitor)
}

func (w *Walker) RegisterEnterDirectiveVisitor(visitor EnterDirectiveVisitor) {
	w.visitors.enterDirective = append(w.visitors.enterDirective, visitor)
}

func (w *Walker) RegisterLeaveDirectiveVisitor(visitor LeaveDirectiveVisitor) {
	w.visitors.leaveDirective = append(w.visitors.leaveDirective, visitor)
}

func (w *Walker) RegisterDirectiveVisitor(visitor DirectiveVisitor) {
	w.RegisterEnterDirectiveVisitor(visitor)
	w.RegisterLeaveDirectiveVisitor(visitor)
}

func (w *Walker) RegisterAllNodesVisitor(visitor AllNodesVisitor) {
	w.RegisterDocumentVisitor(visitor)
	w.RegisterExecutableVisitor(visitor)
	w.RegisterTypeSystemVisitor(visitor)
}

func (w *Walker) RegisterEnterDocumentVisitor(visitor EnterDocumentVisitor) {
	w.visitors.enterDocument = append(w.visitors.enterDocument, visitor)
}

func (w *Walker) RegisterLeaveDocumentVisitor(visitor LeaveDocumentVisitor) {
	w.visitors.leaveDocument = append(w.visitors.leaveDocument, visitor)
}

func (w *Walker) RegisterDocumentVisitor(visitor DocumentVisitor) {
	w.RegisterEnterDocumentVisitor(visitor)
	w.RegisterLeaveDocumentVisitor(visitor)
}

func (w *Walker) SetVisitorFilter(filter VisitorFilter) {
	w.filter = filter
}

// Walk initiates the walker to start walking the AST from the top root Node
func (w *Walker) Walk(document, definition *ast.Document, report *operationreport.Report) {
	if report == nil {
		w.Report = &operationreport.Report{}
	} else {
		w.Report = report
	}
	w.Ancestors = w.Ancestors[:0]
	w.Path = w.Path[:0]
	w.typeDefinitions = w.typeDefinitions[:0]
	w.document = document
	w.definition = definition
	w.Depth = 0
	w.stop = false
	w.walk()
}

// DefferOnEnterField runs the provided func() after the current batch of visitors
// This gives you the possibility to execute some code that should e.g. run after all EnterField Visitors
func (w *Walker) DefferOnEnterField(fn func()) {
	w.deferred = append(w.deferred, fn)
}

func (w *Walker) runOnEnterFieldDeferred() {
	if len(w.deferred) == 0 {
		return
	}
	for i := range w.deferred {
		w.deferred[i]()
	}
	w.deferred = w.deferred[:0]
}

func (w *Walker) appendAncestor(ref int, kind ast.NodeKind) {
	w.Ancestors = append(w.Ancestors, ast.Node{
		Kind: kind,
		Ref:  ref,
	})

	var typeName ast.ByteSlice

	switch kind {
	case ast.NodeKindOperationDefinition:
		operationType := w.document.OperationDefinitions[ref].OperationType
		switch operationType {
		case ast.OperationTypeQuery:
			typeName = w.definition.Index.QueryTypeName
			w.Path = append(w.Path, ast.PathItem{
				Kind:      ast.FieldName,
				FieldName: literal.QUERY,
			})
		case ast.OperationTypeMutation:
			typeName = w.definition.Index.MutationTypeName
			w.Path = append(w.Path, ast.PathItem{
				Kind:      ast.FieldName,
				FieldName: literal.MUTATION,
			})
		case ast.OperationTypeSubscription:
			typeName = w.definition.Index.SubscriptionTypeName
			w.Path = append(w.Path, ast.PathItem{
				Kind:      ast.FieldName,
				FieldName: literal.SUBSCRIPTION,
			})
		default:
			w.StopWithExternalErr(operationreport.ErrInvalidOperationType(operationType))
			return
		}
		if len(typeName) == 0 {
			w.StopWithExternalErr(operationreport.ErrOperationTypeUndefined(operationType))
			return
		}
	case ast.NodeKindInlineFragment:
		if !w.document.InlineFragmentHasTypeCondition(ref) {
			return
		}
		typeName = w.document.InlineFragmentTypeConditionName(ref)
		w.Path = append(w.Path, ast.PathItem{
			Kind:      ast.InlineFragmentName,
			FieldName: typeName,
		})
	case ast.NodeKindFragmentDefinition:
		typeName = w.document.FragmentDefinitionTypeName(ref)
		w.Path = append(w.Path, ast.PathItem{
			Kind:       ast.FieldName,
			ArrayIndex: 0,
			FieldName:  typeName,
		})
	case ast.NodeKindField:
		fieldName := w.document.FieldNameBytes(ref)
		w.Path = append(w.Path, ast.PathItem{
			Kind:       ast.FieldName,
			ArrayIndex: 0,
			FieldName:  w.document.FieldAliasOrNameBytes(ref),
		})
		if bytes.Equal(fieldName, literal.TYPENAME) {
			typeName = literal.STRING
		}
		fields := w.definition.NodeFieldDefinitions(w.typeDefinitions[len(w.typeDefinitions)-1])
		for _, i := range fields {
			if bytes.Equal(fieldName, w.definition.FieldDefinitionNameBytes(i)) {
				typeName = w.definition.ResolveTypeNameBytes(w.definition.FieldDefinitionType(i))
				break
			}
		}
		if typeName == nil {
			typeName := w.definition.NodeNameBytes(w.typeDefinitions[len(w.typeDefinitions)-1])
			w.StopWithExternalErr(operationreport.ErrFieldUndefinedOnType(fieldName, typeName))
			return
		}
	case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
		w.EnclosingTypeDefinition = ast.Node{
			Kind: kind,
			Ref:  ref,
		}
		return
	default:
		return
	}

	var exists bool
	w.EnclosingTypeDefinition, exists = w.definition.Index.FirstNonExtensionNodeByNameBytes(typeName)
	if !exists {
		w.StopWithExternalErr(operationreport.ErrTypeUndefined(typeName))
		return
	}

	w.typeDefinitions = append(w.typeDefinitions, w.EnclosingTypeDefinition)
}

func (w *Walker) removeLastAncestor() {

	ancestor := w.Ancestors[len(w.Ancestors)-1]
	w.Ancestors = w.Ancestors[:len(w.Ancestors)-1]

	switch ancestor.Kind {
	case ast.NodeKindOperationDefinition, ast.NodeKindFragmentDefinition:
		w.Path = w.Path[:len(w.Path)-1]
		w.typeDefinitions = w.typeDefinitions[:len(w.typeDefinitions)-1]
		w.EnclosingTypeDefinition.Kind = ast.NodeKindUnknown
		w.EnclosingTypeDefinition.Ref = -1
	case ast.NodeKindInlineFragment:
		if w.document.InlineFragmentHasTypeCondition(ancestor.Ref) {
			w.Path = w.Path[:len(w.Path)-1]
			w.typeDefinitions = w.typeDefinitions[:len(w.typeDefinitions)-1]
			w.EnclosingTypeDefinition = w.typeDefinitions[len(w.typeDefinitions)-1]
		}
	case ast.NodeKindField:
		w.Path = w.Path[:len(w.Path)-1]
		w.typeDefinitions = w.typeDefinitions[:len(w.typeDefinitions)-1]
		w.EnclosingTypeDefinition = w.typeDefinitions[len(w.typeDefinitions)-1]
	case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition:
		w.EnclosingTypeDefinition.Ref = -1
		w.EnclosingTypeDefinition.Kind = ast.NodeKindUnknown
	default:
		return
	}
}

func (w *Walker) increaseDepth() {
	w.Depth++
}

func (w *Walker) decreaseDepth() {
	w.Depth--
}

func (w *Walker) walk() {

	if w.document == nil {
		w.Report.AddInternalError(ErrDocumentMustNotBeNil)
		return
	}

	skipFor := make(SkipVisitors, 0, 4)

	for i := 0; i < len(w.visitors.enterDocument); {
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterDocument, 0, w.visitors.enterDocument[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterDocument[i], allowedToVisit)

		if allowedToVisit {
			w.visitors.enterDocument[i].EnterDocument(w.document, w.definition)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			return
		}
		i++
	}

	for i := range w.document.RootNodes {
		switch w.document.RootNodes[i].Kind {
		case ast.NodeKindOperationDefinition:
			if w.definition == nil {
				w.Report.AddInternalError(ErrDefinitionMustNotBeNil)
				return
			}
			w.walkOperationDefinition(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindFragmentDefinition:
			if w.definition == nil {
				w.Report.AddInternalError(ErrDefinitionMustNotBeNil)
				return
			}
			w.walkFragmentDefinition(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindSchemaDefinition:
			w.walkSchemaDefinition(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindSchemaExtension:
			w.walkSchemaExtension(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindDirectiveDefinition:
			w.walkDirectiveDefinition(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindObjectTypeDefinition:
			w.walkObjectTypeDefinition(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindObjectTypeExtension:
			w.walkObjectTypeExtension(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindInterfaceTypeDefinition:
			w.walkInterfaceTypeDefinition(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindInterfaceTypeExtension:
			w.walkInterfaceTypeExtension(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindScalarTypeDefinition:
			w.walkScalarTypeDefinition(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindScalarTypeExtension:
			w.walkScalarTypeExtension(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindUnionTypeDefinition:
			w.walkUnionTypeDefinition(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindUnionTypeExtension:
			w.walkUnionTypeExtension(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindEnumTypeDefinition:
			w.walkEnumTypeDefinition(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindEnumTypeExtension:
			w.walkEnumTypeExtension(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindInputObjectTypeDefinition:
			w.walkInputObjectTypeDefinition(w.document.RootNodes[i].Ref, skipFor)
		case ast.NodeKindInputObjectTypeExtension:
			w.walkInputObjectTypeExtension(w.document.RootNodes[i].Ref, skipFor)
		}

		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			return
		}
	}

	for i := len(w.visitors.leaveDocument) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveDocument[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveDocument, 0, w.visitors.leaveDocument[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveDocument[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveDocument[i].LeaveDocument(w.document, w.definition)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			return
		}
		i--
	}
}

func (w *Walker) walkOperationDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterOperation); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterOperation[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterOperation, ref, w.visitors.enterOperation[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterOperation[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterOperation[i].EnterOperationDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindOperationDefinition)
	if w.stop {
		return
	}

	if w.document.OperationDefinitions[ref].HasVariableDefinitions {
		for _, i := range w.document.OperationDefinitions[ref].VariableDefinitions.Refs {
			w.walkVariableDefinition(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.OperationDefinitions[ref].HasDirectives {
		for _, i := range w.document.OperationDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.OperationDefinitions[ref].HasSelections {
		w.walkSelectionSet(w.document.OperationDefinitions[ref].SelectionSet, skipFor)
		if w.stop {
			return
		}
	}

	w.removeLastAncestor()

	for i := len(w.visitors.leaveOperation) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveOperation[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveOperation, ref, w.visitors.leaveOperation[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveOperation[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveOperation[i].LeaveOperationDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkVariableDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterVariableDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterVariableDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterVariableDefinition, ref, w.visitors.enterVariableDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterVariableDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterVariableDefinition[i].EnterVariableDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindVariableDefinition)
	if w.stop {
		return
	}

	if w.document.VariableDefinitions[ref].HasDirectives {
		for _, i := range w.document.VariableDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	for i := len(w.visitors.leaveVariableDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveVariableDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveVariableDefinition, ref, w.visitors.leaveVariableDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveVariableDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveVariableDefinition[i].LeaveVariableDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkSelectionSet(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterSelectionSet); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterSelectionSet[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterSelectionSet, ref, w.visitors.enterSelectionSet[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterSelectionSet[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterSelectionSet[i].EnterSelectionSet(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindSelectionSet)
	if w.stop {
		return
	}

RefsChanged:
	for {
		refs := make([]int, 0, len(w.document.SelectionSets[ref].SelectionRefs))
		refs = append(refs, w.document.SelectionSets[ref].SelectionRefs...)

		for i, j := range refs {

			w.SelectionsBefore = refs[:i]
			w.SelectionsAfter = refs[i+1:]

			switch w.document.Selections[j].Kind {
			case ast.SelectionKindField:
				w.walkField(w.document.Selections[j].Ref, skipFor)
			case ast.SelectionKindFragmentSpread:
				w.walkFragmentSpread(w.document.Selections[j].Ref, skipFor)
			case ast.SelectionKindInlineFragment:
				w.walkInlineFragment(w.document.Selections[j].Ref, skipFor)
			}

			if w.stop {
				return
			}
			if !w.refsEqual(refs, w.document.SelectionSets[ref].SelectionRefs) {
				continue RefsChanged
			}
		}
		break
	}

	w.removeLastAncestor()

	for i := len(w.visitors.leaveSelectionSet) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveSelectionSet[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveSelectionSet, ref, w.visitors.leaveSelectionSet[i], skipFor)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveSelectionSet[i].LeaveSelectionSet(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkField(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	selectionsBefore := w.SelectionsBefore
	selectionsAfter := w.SelectionsAfter

	w.setCurrent(ast.NodeKindField, ref)

	for i := 0; i < len(w.visitors.enterField); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterField[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterField, ref, w.visitors.enterField[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterField[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterField[i].EnterField(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.runOnEnterFieldDeferred()

	w.appendAncestor(ref, ast.NodeKindField)
	if w.stop {
		return
	}

	if len(w.document.Fields[ref].Arguments.Refs) != 0 {
		for _, i := range w.document.Fields[ref].Arguments.Refs {
			w.walkArgument(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.Fields[ref].HasDirectives {
		for _, i := range w.document.Fields[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.Fields[ref].HasSelections {
		w.walkSelectionSet(w.document.Fields[ref].SelectionSet, skipFor)
	}

	w.removeLastAncestor()

	w.SelectionsBefore = selectionsBefore
	w.SelectionsAfter = selectionsAfter

	w.setCurrent(ast.NodeKindField, ref)

	for i := len(w.visitors.leaveField) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveField[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveField, ref, w.visitors.leaveField[i], skipFor)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveField[i].LeaveField(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkDirective(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindDirective, ref)

	for i := 0; i < len(w.visitors.enterDirective); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterDirective[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterDirective, ref, w.visitors.enterDirective[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterDirective[i], allowedToVisit)
		if allowedToVisit && ancestorAllowed {
			w.visitors.enterDirective[i].EnterDirective(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindDirective)
	if w.stop {
		return
	}

	if w.document.Directives[ref].HasArguments {
		for _, i := range w.document.Directives[ref].Arguments.Refs {
			w.walkArgument(i, skipFor)
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindDirective, ref)

	for i := len(w.visitors.leaveDirective) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveDirective[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveDirective, ref, w.visitors.leaveDirective[i], skipFor)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveDirective[i].LeaveDirective(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkArgument(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindArgument, ref)

	for i := 0; i < len(w.visitors.enterArgument); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterArgument[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterArgument, ref, w.visitors.enterArgument[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterArgument[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterArgument[i].EnterArgument(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	for i := len(w.visitors.leaveArgument) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveArgument[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveArgument, ref, w.visitors.leaveArgument[i], skipFor)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveArgument[i].LeaveArgument(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkFragmentSpread(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindFragmentSpread, ref)

	for i := 0; i < len(w.visitors.enterFragmentSpread); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterFragmentSpread[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterFragmentSpread, ref, w.visitors.enterFragmentSpread[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterFragmentSpread[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterFragmentSpread[i].EnterFragmentSpread(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindFragmentSpread)
	if w.stop {
		return
	}

	if w.document.FragmentSpreads[ref].HasDirectives {
		for _, i := range w.document.FragmentSpreads[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
		}
	}

	w.removeLastAncestor()

	for i := len(w.visitors.leaveFragmentSpread) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveFragmentSpread[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveFragmentSpread, ref, w.visitors.leaveFragmentSpread[i], skipFor)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveFragmentSpread[i].LeaveFragmentSpread(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkInlineFragment(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	selectionsBefore := w.SelectionsBefore
	selectionsAfter := w.SelectionsAfter

	w.setCurrent(ast.NodeKindInlineFragment, ref)

	for i := 0; i < len(w.visitors.enterInlineFragment); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterInlineFragment[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterInlineFragment, ref, w.visitors.enterInlineFragment[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterInlineFragment[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterInlineFragment[i].EnterInlineFragment(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindInlineFragment)
	if w.stop {
		return
	}

	if w.document.InlineFragments[ref].HasDirectives {
		for _, i := range w.document.InlineFragments[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
		}
	}

	if w.document.InlineFragments[ref].HasSelections {
		w.walkSelectionSet(w.document.InlineFragments[ref].SelectionSet, skipFor)
		if w.stop {
			return
		}
	}

	w.removeLastAncestor()

	w.SelectionsBefore = selectionsBefore
	w.SelectionsAfter = selectionsAfter

	w.setCurrent(ast.NodeKindInlineFragment, ref)

	for i := len(w.visitors.leaveInlineFragment) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveInlineFragment[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveInlineFragment, ref, w.visitors.leaveInlineFragment[i], skipFor)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveInlineFragment[i].LeaveInlineFragment(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkFragmentDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindFragmentDefinition, ref)

	for i := 0; i < len(w.visitors.enterFragmentDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterFragmentDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterFragmentDefinition, ref, w.visitors.enterFragmentDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterFragmentDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterFragmentDefinition[i].EnterFragmentDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindFragmentDefinition)
	if w.stop {
		return
	}

	if w.document.FragmentDefinitions[ref].HasDirectives {
		for _, i := range w.document.FragmentDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
		}
	}

	if w.document.FragmentDefinitions[ref].HasSelections {
		w.walkSelectionSet(w.document.FragmentDefinitions[ref].SelectionSet, skipFor)
		if w.stop {
			return
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindFragmentDefinition, ref)

	for i := len(w.visitors.leaveFragmentDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveFragmentDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveFragmentDefinition, ref, w.visitors.leaveFragmentDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveFragmentDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveFragmentDefinition[i].LeaveFragmentDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkObjectTypeDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindObjectTypeDefinition, ref)

	for i := 0; i < len(w.visitors.enterObjectTypeDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterObjectTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterObjectTypeDefinition, ref, w.visitors.enterObjectTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterObjectTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterObjectTypeDefinition[i].EnterObjectTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindObjectTypeDefinition)
	if w.stop {
		return
	}

	if w.document.ObjectTypeDefinitions[ref].HasDirectives {
		for _, i := range w.document.ObjectTypeDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.ObjectTypeDefinitions[ref].HasFieldDefinitions {
		for _, i := range w.document.ObjectTypeDefinitions[ref].FieldsDefinition.Refs {
			w.walkFieldDefinition(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindObjectTypeDefinition, ref)

	for i := len(w.visitors.leaveObjectTypeDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveObjectTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveObjectTypeDefinition, ref, w.visitors.leaveObjectTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveObjectTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveObjectTypeDefinition[i].LeaveObjectTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkObjectTypeExtension(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindObjectTypeExtension, ref)

	for i := 0; i < len(w.visitors.enterObjectTypeExtension); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterObjectTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterObjectTypeExtension, ref, w.visitors.enterObjectTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterObjectTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterObjectTypeExtension[i].EnterObjectTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindObjectTypeExtension)
	if w.stop {
		return
	}

	if w.document.ObjectTypeExtensions[ref].HasDirectives {
		for _, i := range w.document.ObjectTypeExtensions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.ObjectTypeExtensions[ref].HasFieldDefinitions {
		for _, i := range w.document.ObjectTypeExtensions[ref].FieldsDefinition.Refs {
			w.walkFieldDefinition(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindObjectTypeExtension, ref)

	for i := len(w.visitors.leaveObjectTypeExtension) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveObjectTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveObjectTypeExtension, ref, w.visitors.leaveObjectTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveObjectTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveObjectTypeExtension[i].LeaveObjectTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkFieldDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindFieldDefinition, ref)

	for i := 0; i < len(w.visitors.enterFieldDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterFieldDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterFieldDefinition, ref, w.visitors.enterFieldDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterFieldDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterFieldDefinition[i].EnterFieldDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindFieldDefinition)
	if w.stop {
		return
	}

	if w.document.FieldDefinitions[ref].HasArgumentsDefinitions {
		for _, i := range w.document.FieldDefinitions[ref].ArgumentsDefinition.Refs {
			w.walkInputValueDefinition(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.FieldDefinitions[ref].HasDirectives {
		for _, i := range w.document.FieldDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindFieldDefinition, ref)

	for i := len(w.visitors.leaveFieldDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveFieldDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveFieldDefinition, ref, w.visitors.leaveFieldDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveFieldDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveFieldDefinition[i].LeaveFieldDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkInputValueDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindInputValueDefinition, ref)

	for i := 0; i < len(w.visitors.enterInputValueDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterInputValueDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterInputValueDefinition, ref, w.visitors.enterInputValueDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterInputValueDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterInputValueDefinition[i].EnterInputValueDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindInputValueDefinition)
	if w.stop {
		return
	}

	if w.document.InputValueDefinitions[ref].HasDirectives {
		for _, i := range w.document.InputValueDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindInputValueDefinition, ref)

	for i := len(w.visitors.leaveInputValueDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveInputValueDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveInputValueDefinition, ref, w.visitors.leaveInputValueDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveInputValueDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveInputValueDefinition[i].LeaveInputValueDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkInterfaceTypeDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindInterfaceTypeDefinition, ref)

	for i := 0; i < len(w.visitors.enterInterfaceTypeDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterInterfaceTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterInterfaceTypeDefinition, ref, w.visitors.enterInterfaceTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterInterfaceTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterInterfaceTypeDefinition[i].EnterInterfaceTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindInterfaceTypeDefinition)
	if w.stop {
		return
	}

	if w.document.InterfaceTypeDefinitions[ref].HasDirectives {
		for _, i := range w.document.InterfaceTypeDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.InterfaceTypeDefinitions[ref].HasFieldDefinitions {
		for _, i := range w.document.InterfaceTypeDefinitions[ref].FieldsDefinition.Refs {
			w.walkFieldDefinition(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindInterfaceTypeDefinition, ref)

	for i := len(w.visitors.leaveInterfaceTypeDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveInterfaceTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveInterfaceTypeDefinition, ref, w.visitors.leaveInterfaceTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveInterfaceTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveInterfaceTypeDefinition[i].LeaveInterfaceTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkInterfaceTypeExtension(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindInterfaceTypeExtension, ref)

	for i := 0; i < len(w.visitors.enterInterfaceTypeExtension); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterInterfaceTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterInterfaceTypeExtension, ref, w.visitors.enterInterfaceTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterInterfaceTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterInterfaceTypeExtension[i].EnterInterfaceTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindInterfaceTypeExtension)
	if w.stop {
		return
	}

	if w.document.InterfaceTypeExtensions[ref].HasDirectives {
		for _, i := range w.document.InterfaceTypeExtensions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.InterfaceTypeExtensions[ref].HasFieldDefinitions {
		for _, i := range w.document.InterfaceTypeExtensions[ref].FieldsDefinition.Refs {
			w.walkFieldDefinition(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindInterfaceTypeExtension, ref)

	for i := len(w.visitors.leaveInterfaceTypeExtension) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveInterfaceTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveInterfaceTypeExtension, ref, w.visitors.leaveInterfaceTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveInterfaceTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveInterfaceTypeExtension[i].LeaveInterfaceTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkScalarTypeDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindScalarTypeDefinition, ref)

	for i := 0; i < len(w.visitors.enterScalarTypeDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterScalarTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterScalarTypeDefinition, ref, w.visitors.enterScalarTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterScalarTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterScalarTypeDefinition[i].EnterScalarTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindScalarTypeDefinition)
	if w.stop {
		return
	}

	if w.document.ScalarTypeDefinitions[ref].HasDirectives {
		for _, i := range w.document.ScalarTypeDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindScalarTypeDefinition, ref)

	for i := len(w.visitors.leaveScalarTypeDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveScalarTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveScalarTypeDefinition, ref, w.visitors.leaveScalarTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveScalarTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveScalarTypeDefinition[i].LeaveScalarTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkScalarTypeExtension(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindScalarTypeExtension, ref)

	for i := 0; i < len(w.visitors.enterScalarTypeExtension); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterScalarTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterScalarTypeExtension, ref, w.visitors.enterScalarTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterScalarTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterScalarTypeExtension[i].EnterScalarTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindScalarTypeExtension)
	if w.stop {
		return
	}

	if w.document.ScalarTypeExtensions[ref].HasDirectives {
		for _, i := range w.document.ScalarTypeExtensions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindScalarTypeExtension, ref)

	for i := len(w.visitors.leaveScalarTypeExtension) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveScalarTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveScalarTypeExtension, ref, w.visitors.leaveScalarTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveScalarTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveScalarTypeExtension[i].LeaveScalarTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkUnionTypeDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindUnionTypeDefinition, ref)

	for i := 0; i < len(w.visitors.enterUnionTypeDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterUnionTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterUnionTypeDefinition, ref, w.visitors.enterUnionTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterUnionTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterUnionTypeDefinition[i].EnterUnionTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindUnionTypeDefinition)
	if w.stop {
		return
	}

	if w.document.UnionTypeDefinitions[ref].HasDirectives {
		for _, i := range w.document.UnionTypeDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.UnionTypeDefinitions[ref].HasUnionMemberTypes {
		for _, i := range w.document.UnionTypeDefinitions[ref].UnionMemberTypes.Refs {
			w.walkUnionMemberType(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindUnionTypeDefinition, ref)

	for i := len(w.visitors.leaveUnionTypeDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveUnionTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveUnionTypeDefinition, ref, w.visitors.leaveUnionTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveUnionTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveUnionTypeDefinition[i].LeaveUnionTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkUnionTypeExtension(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindUnionTypeExtension, ref)

	for i := 0; i < len(w.visitors.enterUnionTypeExtension); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterUnionTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterUnionTypeExtension, ref, w.visitors.enterUnionTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterUnionTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterUnionTypeExtension[i].EnterUnionTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindUnionTypeExtension)
	if w.stop {
		return
	}

	if w.document.UnionTypeExtensions[ref].HasDirectives {
		for _, i := range w.document.UnionTypeExtensions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.UnionTypeExtensions[ref].HasUnionMemberTypes {
		for _, i := range w.document.UnionTypeExtensions[ref].UnionMemberTypes.Refs {
			w.walkUnionMemberType(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindUnionTypeExtension, ref)

	for i := len(w.visitors.leaveUnionTypeExtension) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveUnionTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveUnionTypeExtension, ref, w.visitors.leaveUnionTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveUnionTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveUnionTypeExtension[i].LeaveUnionTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkUnionMemberType(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindUnionMemberType, ref)

	for i := 0; i < len(w.visitors.enterUnionMemberType); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterUnionMemberType[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterUnionMemberType, ref, w.visitors.enterUnionMemberType[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterUnionMemberType[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterUnionMemberType[i].EnterUnionMemberType(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	for i := len(w.visitors.leaveUnionMemberType) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveUnionMemberType[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveUnionMemberType, ref, w.visitors.leaveUnionMemberType[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveUnionMemberType[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveUnionMemberType[i].LeaveUnionMemberType(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkEnumTypeDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindEnumTypeDefinition, ref)

	for i := 0; i < len(w.visitors.enterEnumTypeDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterEnumTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterEnumTypeDefinition, ref, w.visitors.enterEnumTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterEnumTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterEnumTypeDefinition[i].EnterEnumTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindEnumTypeDefinition)
	if w.stop {
		return
	}

	if w.document.EnumTypeDefinitions[ref].HasDirectives {
		for _, i := range w.document.EnumTypeDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.EnumTypeDefinitions[ref].HasEnumValuesDefinition {
		for _, i := range w.document.EnumTypeDefinitions[ref].EnumValuesDefinition.Refs {
			w.walkEnumValueDefinition(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindEnumTypeDefinition, ref)

	for i := len(w.visitors.leaveEnumTypeDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveEnumTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveEnumTypeDefinition, ref, w.visitors.leaveEnumTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveEnumTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveEnumTypeDefinition[i].LeaveEnumTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkEnumTypeExtension(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindEnumTypeExtension, ref)

	for i := 0; i < len(w.visitors.enterEnumTypeExtension); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterEnumTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterEnumTypeExtension, ref, w.visitors.enterEnumTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterEnumTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterEnumTypeExtension[i].EnterEnumTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindEnumTypeExtension)
	if w.stop {
		return
	}

	if w.document.EnumTypeExtensions[ref].HasDirectives {
		for _, i := range w.document.EnumTypeExtensions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.EnumTypeExtensions[ref].HasEnumValuesDefinition {
		for _, i := range w.document.EnumTypeExtensions[ref].EnumValuesDefinition.Refs {
			w.walkEnumValueDefinition(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindEnumTypeExtension, ref)

	for i := len(w.visitors.leaveEnumTypeExtension) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveEnumTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveEnumTypeExtension, ref, w.visitors.leaveEnumTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveEnumTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveEnumTypeExtension[i].LeaveEnumTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkEnumValueDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindEnumValueDefinition, ref)

	for i := 0; i < len(w.visitors.enterEnumValueDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterEnumValueDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterEnumValueDefinition, ref, w.visitors.enterEnumValueDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterEnumValueDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterEnumValueDefinition[i].EnterEnumValueDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindEnumValueDefinition)
	if w.stop {
		return
	}

	if w.document.EnumValueDefinitions[ref].HasDirectives {
		for _, i := range w.document.EnumValueDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindEnumValueDefinition, ref)

	for i := len(w.visitors.leaveEnumValueDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveEnumValueDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveEnumValueDefinition, ref, w.visitors.leaveEnumValueDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveEnumValueDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveEnumValueDefinition[i].LeaveEnumValueDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkInputObjectTypeDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindInputObjectTypeDefinition, ref)

	for i := 0; i < len(w.visitors.enterInputObjectTypeDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterInputObjectTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterInputObjectTypeDefinition, ref, w.visitors.enterInputObjectTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterInputObjectTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterInputObjectTypeDefinition[i].EnterInputObjectTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindInputObjectTypeDefinition)
	if w.stop {
		return
	}

	if w.document.InputObjectTypeDefinitions[ref].HasDirectives {
		for _, i := range w.document.InputObjectTypeDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.InputObjectTypeDefinitions[ref].HasInputFieldsDefinition {
		for _, i := range w.document.InputObjectTypeDefinitions[ref].InputFieldsDefinition.Refs {
			w.walkInputValueDefinition(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindInputObjectTypeDefinition, ref)

	for i := len(w.visitors.leaveInputObjectTypeDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveInputObjectTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveInputObjectTypeDefinition, ref, w.visitors.leaveInputObjectTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveInputObjectTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveInputObjectTypeDefinition[i].LeaveInputObjectTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkInputObjectTypeExtension(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindInputObjectTypeExtension, ref)

	for i := 0; i < len(w.visitors.enterInputObjectTypeExtension); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterInputObjectTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterInputObjectTypeExtension, ref, w.visitors.enterInputObjectTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterInputObjectTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterInputObjectTypeExtension[i].EnterInputObjectTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindInputObjectTypeExtension)
	if w.stop {
		return
	}

	if w.document.InputObjectTypeExtensions[ref].HasDirectives {
		for _, i := range w.document.InputObjectTypeExtensions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	if w.document.InputObjectTypeExtensions[ref].HasInputFieldsDefinition {
		for _, i := range w.document.InputObjectTypeExtensions[ref].InputFieldsDefinition.Refs {
			w.walkInputValueDefinition(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindInputObjectTypeExtension, ref)

	for i := len(w.visitors.leaveInputObjectTypeExtension) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveInputObjectTypeExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveInputObjectTypeExtension, ref, w.visitors.leaveInputObjectTypeExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveInputObjectTypeExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveInputObjectTypeExtension[i].LeaveInputObjectTypeExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkDirectiveDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindDirectiveDefinition, ref)

	for i := 0; i < len(w.visitors.enterDirectiveDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterDirectiveDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterDirectiveDefinition, ref, w.visitors.enterDirectiveDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterDirectiveDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterDirectiveDefinition[i].EnterDirectiveDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindDirectiveDefinition)
	if w.stop {
		return
	}

	if w.document.DirectiveDefinitions[ref].HasArgumentsDefinitions {
		for _, i := range w.document.DirectiveDefinitions[ref].ArgumentsDefinition.Refs {
			w.walkInputValueDefinition(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	iter := w.document.DirectiveDefinitions[ref].DirectiveLocations.Iterable()
	for iter.Next() {
		w.walkDirectiveLocation(iter.Value(), skipFor)
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindDirectiveDefinition, ref)

	for i := len(w.visitors.leaveDirectiveDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveDirectiveDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveDirectiveDefinition, ref, w.visitors.leaveDirectiveDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveDirectiveDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveDirectiveDefinition[i].LeaveDirectiveDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkDirectiveLocation(location ast.DirectiveLocation, skipFor SkipVisitors) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterDirectiveLocation); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterDirectiveLocation[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterDirectiveLocation, 0, w.visitors.enterDirectiveLocation[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterDirectiveLocation[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterDirectiveLocation[i].EnterDirectiveLocation(location)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	for i := len(w.visitors.leaveDirectiveLocation) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveDirectiveLocation[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveDirectiveLocation, 0, w.visitors.leaveDirectiveLocation[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveDirectiveLocation[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveDirectiveLocation[i].LeaveDirectiveLocation(location)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkSchemaDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindSchemaDefinition, ref)

	for i := 0; i < len(w.visitors.enterSchemaDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterSchemaDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterSchemaDefinition, ref, w.visitors.enterSchemaDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterSchemaDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterSchemaDefinition[i].EnterSchemaDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindSchemaDefinition)
	if w.stop {
		return
	}

	if w.document.SchemaDefinitions[ref].HasDirectives {
		for _, i := range w.document.SchemaDefinitions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	for _, i := range w.document.SchemaDefinitions[ref].RootOperationTypeDefinitions.Refs {
		w.walkRootOperationTypeDefinition(i, skipFor)
		if w.stop {
			return
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindSchemaDefinition, ref)

	for i := len(w.visitors.leaveSchemaDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveSchemaDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveSchemaDefinition, ref, w.visitors.leaveSchemaDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveSchemaDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveSchemaDefinition[i].LeaveSchemaDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkSchemaExtension(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	w.setCurrent(ast.NodeKindSchemaExtension, ref)

	for i := 0; i < len(w.visitors.enterSchemaExtension); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterSchemaExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterSchemaExtension, ref, w.visitors.enterSchemaExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterSchemaExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterSchemaExtension[i].EnterSchemaExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	w.appendAncestor(ref, ast.NodeKindSchemaExtension)
	if w.stop {
		return
	}

	if w.document.SchemaExtensions[ref].HasDirectives {
		for _, i := range w.document.SchemaExtensions[ref].Directives.Refs {
			w.walkDirective(i, skipFor)
			if w.stop {
				return
			}
		}
	}

	for _, i := range w.document.SchemaExtensions[ref].RootOperationTypeDefinitions.Refs {
		w.walkRootOperationTypeDefinition(i, skipFor)
		if w.stop {
			return
		}
	}

	w.removeLastAncestor()

	w.setCurrent(ast.NodeKindSchemaExtension, ref)

	for i := len(w.visitors.leaveSchemaExtension) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveSchemaExtension[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveSchemaExtension, ref, w.visitors.leaveSchemaExtension[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveSchemaExtension[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveSchemaExtension[i].LeaveSchemaExtension(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) walkRootOperationTypeDefinition(ref int, skipFor SkipVisitors) {
	w.increaseDepth()

	for i := 0; i < len(w.visitors.enterRootOperationTypeDefinition); {
		ancestorAllowed := skipFor.Allow(w.visitors.enterRootOperationTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(EnterRootOperationTypeDefinition, ref, w.visitors.enterRootOperationTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.enterRootOperationTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.enterRootOperationTypeDefinition[i].EnterRootOperationTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i++
	}

	for i := len(w.visitors.leaveRootOperationTypeDefinition) - 1; i > -1; {
		ancestorAllowed := skipFor.Allow(w.visitors.leaveRootOperationTypeDefinition[i])
		allowedToVisit := w.filter == nil || w.filter.AllowVisitor(LeaveRootOperationTypeDefinition, ref, w.visitors.leaveRootOperationTypeDefinition[i], skipFor)
		skipFor = newSkipVisitors(skipFor, w.visitors.leaveRootOperationTypeDefinition[i], allowedToVisit)

		if allowedToVisit && ancestorAllowed {
			w.visitors.leaveRootOperationTypeDefinition[i].LeaveRootOperationTypeDefinition(ref)
		}
		if w.revisit {
			w.revisit = false
			continue
		}
		if w.stop {
			return
		}
		if w.skip {
			w.skip = false
			w.decreaseDepth()
			return
		}
		i--
	}

	w.decreaseDepth()
}

func (w *Walker) refsEqual(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (w *Walker) SkipNode() {
	w.skip = true
}

func (w *Walker) Stop() {
	w.stop = true
}

func (w *Walker) RevisitNode() {
	w.revisit = true
}

func (w *Walker) StopWithInternalErr(err error) {
	w.stop = true
	w.Report.AddInternalError(err)
}

func (w *Walker) HandleInternalErr(err error) bool {
	if err != nil {
		w.StopWithInternalErr(err)
		return true
	}
	return false
}

func (w *Walker) StopWithExternalErr(err operationreport.ExternalError) {
	w.stop = true
	err.Path = w.Path
	w.Report.AddExternalError(err)
}

func (w *Walker) StopWithErr(internal error, external operationreport.ExternalError) {
	w.stop = true
	external.Path = w.Path
	w.Report.AddInternalError(internal)
	w.Report.AddExternalError(external)
}

func (w *Walker) ArgumentInputValueDefinition(argument int) (definition int, exits bool) {
	argumentName := w.document.ArgumentNameBytes(argument)
	ancestor := w.Ancestors[len(w.Ancestors)-1]
	switch ancestor.Kind {
	case ast.NodeKindField:
		fieldName := w.document.FieldNameBytes(ancestor.Ref)
		fieldTypeDef := w.typeDefinitions[len(w.typeDefinitions)-2]
		definition = w.definition.NodeFieldDefinitionArgumentDefinitionByName(fieldTypeDef, fieldName, argumentName)
		exits = definition != -1
	case ast.NodeKindDirective:
		directiveName := w.document.DirectiveNameBytes(ancestor.Ref)
		definition = w.definition.DirectiveArgumentInputValueDefinition(directiveName, argumentName)
		exits = definition != -1
	}
	return
}

// FieldDefinitionWithExists
// Deprecated: use FieldDefinition
func (w *Walker) FieldDefinitionWithExists(field int) (definition int, exists bool) {
	return w.FieldDefinition(field)
}

// FieldDefinition - returns field definition ref from schema definition
// field - ref to a field from operation
// returns:
// definition - ref to a field definition from schema definition document if it is exists otherwise returns ast.InvalidRef
// exists - true if field exists
func (w *Walker) FieldDefinition(field int) (definition int, exists bool) {
	fieldName := w.document.FieldNameBytes(field)
	return w.definition.NodeFieldDefinitionByName(w.EnclosingTypeDefinition, fieldName)
}

func (w *Walker) Ancestor() ast.Node {
	if len(w.Ancestors) == 0 {
		return ast.InvalidNode
	}
	return w.Ancestors[len(w.Ancestors)-1]
}

func (w *Walker) AncestorNameBytes() ast.ByteSlice {
	if len(w.Ancestors) == 0 {
		return nil
	}
	return w.document.NodeNameBytes(w.Ancestors[len(w.Ancestors)-1])
}

func (w *Walker) FieldDefinitionDirectiveArgumentValueByName(field int, directiveName, argumentName ast.ByteSlice) (ast.Value, bool) {
	definition, exists := w.FieldDefinition(field)
	if !exists {
		return ast.Value{}, false
	}

	directive, exists := w.definition.FieldDefinitionDirectiveByName(definition, directiveName)
	if !exists {
		return ast.Value{}, false
	}

	return w.definition.DirectiveArgumentValueByName(directive, argumentName)
}
