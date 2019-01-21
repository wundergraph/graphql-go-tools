package document

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
)

// SchemaDefinition as specified in:
// http://facebook.github.io/graphql/draft/#SchemaDefinition
type SchemaDefinition struct {
	Query        ByteSlice
	Mutation     ByteSlice
	Subscription ByteSlice
	Directives   []int
	Position     position.Position
}

func (s SchemaDefinition) NodeName() ByteSliceReference {
	panic("implement me")
}

func (s SchemaDefinition) NodeAlias() ByteSliceReference {
	panic("implement me")
}

func (s SchemaDefinition) NodeDescription() ByteSliceReference {
	panic("implement me")
}

func (s SchemaDefinition) NodeArguments() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeArgumentsDefinition() int {
	panic("implement me")
}

func (s SchemaDefinition) NodeDirectives() []int {
	return s.Directives
}

func (s SchemaDefinition) NodeEnumValuesDefinition() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeFields() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeFieldsDefinition() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeFragmentSpreads() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeInlineFragments() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeVariableDefinitions() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeType() int {
	panic("implement me")
}

func (s SchemaDefinition) NodeOperationType() OperationType {
	panic("implement me")
}

func (s SchemaDefinition) NodeValue() int {
	panic("implement me")
}

func (s SchemaDefinition) NodeDefaultValue() int {
	panic("implement me")
}

func (s SchemaDefinition) NodeImplementsInterfaces() []ByteSliceReference {
	panic("implement me")
}

func (s SchemaDefinition) NodeInputValueDefinitions() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeSchemaDefinition() SchemaDefinition {
	panic("implement me")
}

func (s SchemaDefinition) NodeScalarTypeDefinitions() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeObjectTypeDefinitions() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeInterfaceTypeDefinitions() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeUnionTypeDefinitions() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeEnumTypeDefinitions() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeInputObjectTypeDefinitions() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeDirectiveDefinitions() []int {
	panic("implement me")
}

func (s SchemaDefinition) NodeUnionMemberTypes() []ByteSliceReference {
	panic("implement me")
}

func (s SchemaDefinition) NodeValueType() ValueType {
	panic("implement me")
}

func (s SchemaDefinition) NodeValueReference() int {
	panic("implement me")
}

func (s SchemaDefinition) NodePosition() position.Position {
	return s.Position
}

func (s SchemaDefinition) NodeInputFieldsDefinition() int {
	panic("implement me")
}

// ObjectName returns the struct name for ease of use
func (s SchemaDefinition) ObjectName() string {
	return "SchemaDefinition"
}

// DirectiveLocation returns the related directive location of SchemaDefinition
func (s SchemaDefinition) DirectiveLocation() DirectiveLocation {
	return DirectiveLocationSCHEMA
}

// IsDefined returns a bool depending on whether SchemaDefinition has already
// been defined
func (s SchemaDefinition) IsDefined() bool {
	return len(s.Query) != 0 || len(s.Mutation) != 0 || len(s.Subscription) != 0
}

// SetOperationType sets the operationType and operationName and will return an error in case of setting one value multiple times
func (s *SchemaDefinition) SetOperationType(operationType, operationName ByteSlice) error {

	switch string(operationType) {
	case "query":
		if s.Query != nil {
			return fmt.Errorf("setOperationType: operationName for operationType '%s' already set", operationType)
		}
		s.Query = operationName
		return nil
	case "mutation":
		if s.Mutation != nil {
			return fmt.Errorf("setOperationType: operationName for operationType '%s' already set", operationType)
		}
		s.Mutation = operationName
		return nil
	case "subscription":
		if s.Subscription != nil {
			return fmt.Errorf("setOperationType: operationName for operationType '%s' already set", operationType)
		}
		s.Subscription = operationName
		return nil
	default:
		return fmt.Errorf("setOperationType: unknown operationType '%s' expected one of: [query,subscription,mutation]", string(operationType))
	}
}

// RootOperationTypeDefinition as specified in
// http://facebook.github.io/graphql/draft/#RootOperationTypeDefinition
type RootOperationTypeDefinition string
