package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"testing"
)

func TestParser_parseSchemaDefinition(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(`	schema {
						query: Query
						mutation: Mutation
						subscription: Subscription 
					}`,
			mustParseSchemaDefinition(
				hasSchemaOperationTypeName(document.OperationTypeQuery, "Query"),
				hasSchemaOperationTypeName(document.OperationTypeMutation, "Mutation"),
				hasSchemaOperationTypeName(document.OperationTypeSubscription, "Subscription"),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`	schema {
						query : Query	
						mutation : Mutation
						subscription : Subscription
						query: Query2 
					}`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`	schema @fromTop(to: "bottom") @fromBottom(to: "top") {
						query: Query
						mutation: Mutation
						subscription: Subscription
					}`,
			mustParseSchemaDefinition(
				hasSchemaOperationTypeName(document.OperationTypeQuery, "Query"),
				hasSchemaOperationTypeName(document.OperationTypeMutation, "Mutation"),
				hasSchemaOperationTypeName(document.OperationTypeSubscription, "Subscription"),
				hasDirectives(
					node(
						hasName("fromTop"),
					),
					node(
						hasName("fromBottom"),
					),
				),
			))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run(`schema  @foo(bar: .) { query: Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`schema ( query: Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run(`schema { query. Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 4", func(t *testing.T) {
		run(`schema { query: 1337 }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 5", func(t *testing.T) {
		run(`schema { query: Query )`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
	t.Run("invalid 6", func(t *testing.T) {
		run(`notschema { query: Query }`,
			mustPanic(mustParseSchemaDefinition()),
		)
	})
}
