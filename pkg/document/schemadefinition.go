package document

import (
	"fmt"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/literal"
)

// SchemaDefinition as specified in:
// http://facebook.github.io/graphql/draft/#SchemaDefinition
type SchemaDefinition struct {
	Query        string
	Mutation     string
	Subscription string
	Directives   Directives
}

// ObjectName returns the struct name for ease of use
func (s SchemaDefinition) ObjectName() string {
	return "SchemaDefinition"
}

// GetDirectives returns all directives of SchemaDefinition
func (s SchemaDefinition) GetDirectives() Directives {
	return s.Directives
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
func (s *SchemaDefinition) SetOperationType(operationType, operationName string) error {

	if operationType == literal.QUERY {
		if len(s.Query) == 0 {
			s.Query = operationName
			return nil
		}
	} else if operationType == literal.MUTATION {
		if len(s.Mutation) == 0 {
			s.Mutation = operationName
			return nil
		}
	} else if operationType == literal.SUBSCRIPTION {
		if len(s.Subscription) == 0 {
			s.Subscription = operationName
			return nil
		}
	} else {
		return fmt.Errorf("setOperationType: unknown operationType '%s' expected one of: [query,subscription,mutation]", operationType)
	}

	return fmt.Errorf("setOperationType: operationName for operationType '%s' already set", operationType)
}

// RootOperationTypeDefinition as specified in
// http://facebook.github.io/graphql/draft/#RootOperationTypeDefinition
type RootOperationTypeDefinition string
