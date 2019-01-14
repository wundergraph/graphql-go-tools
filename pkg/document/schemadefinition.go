package document

import (
	"fmt"
)

// SchemaDefinition as specified in:
// http://facebook.github.io/graphql/draft/#SchemaDefinition
type SchemaDefinition struct {
	Query        ByteSlice
	Mutation     ByteSlice
	Subscription ByteSlice
	Directives   []int
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
