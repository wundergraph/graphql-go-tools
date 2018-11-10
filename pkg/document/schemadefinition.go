package document

import "fmt"

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
	return s.Query != "" || s.Mutation != "" || s.Subscription != ""
}

// SetOperationType sets the operationType and operationName and will return an error in case of setting one value multiple times
func (s *SchemaDefinition) SetOperationType(operationType, operationName string) error {
	switch operationType {
	case "query":
		if s.Query == "" {
			s.Query = operationName
			return nil
		}
	case "mutation":
		if s.Mutation == "" {
			s.Mutation = operationName
			return nil
		}
	case "subscription":
		if s.Subscription == "" {
			s.Subscription = operationName
			return nil
		}
	default:
		return fmt.Errorf("setOperationType: unknown operationType '%s' expected one of: [query,subscription,mutation]", operationType)
	}

	return fmt.Errorf("setOperationType: operationName for operationType '%s' already set", operationType)
}

// RootOperationTypeDefinition as specified in
// http://facebook.github.io/graphql/draft/#RootOperationTypeDefinition
type RootOperationTypeDefinition []byte
