package ast

import "github.com/jensneuse/graphql-go-tools/pkg/lexer/position"

type SchemaDefinition struct {
	SchemaLiteral                position.Position // schema
	HasDirectives                bool
	Directives                   DirectiveList                   // optional, e.g. @foo
	RootOperationTypeDefinitions RootOperationTypeDefinitionList // e.g. query: Query, mutation: Mutation, subscription: Subscription
}

func (s *SchemaDefinition) AddRootOperationTypeDefinitionRefs(refs ...int) {
	s.RootOperationTypeDefinitions.Refs = append(s.RootOperationTypeDefinitions.Refs, refs...)
}

func (d *Document) AddSchemaDefinition(schemaDefinition SchemaDefinition) (ref int) {
	d.SchemaDefinitions = append(d.SchemaDefinitions, schemaDefinition)
	return len(d.SchemaDefinitions) - 1
}

func (d *Document) ImportSchemaDefinition(queryTypeName, mutationTypeName, subscriptionTypeName string) (ref int) {
	var operationRefs []int

	if queryTypeName != "" {
		operationRefs = append(operationRefs, d.ImportRootOperationTypeDefinition(queryTypeName, OperationTypeQuery))
	}
	if mutationTypeName != "" {
		operationRefs = append(operationRefs, d.ImportRootOperationTypeDefinition(mutationTypeName, OperationTypeMutation))
	}
	if subscriptionTypeName != "" {
		operationRefs = append(operationRefs, d.ImportRootOperationTypeDefinition(subscriptionTypeName, OperationTypeSubscription))
	}

	schemaDefinition := SchemaDefinition{
		RootOperationTypeDefinitions: RootOperationTypeDefinitionList{
			Refs: operationRefs,
		},
	}

	ref = d.AddSchemaDefinition(schemaDefinition)
	node := Node{
		Kind: NodeKindSchemaDefinition,
		Ref:  ref,
	}
	d.AddRootNode(node)
	return
}
