package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseOperationDefinition(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run(`query allGophers($color: String)@rename(index: 3) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasName("allGophers"),
					hasVariableDefinitions(
						node(
							hasName("color"),
						),
					),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasFields(
						node(
							hasName("name"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   3,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("without directive", func(t *testing.T) {
		run(` query allGophers($color: String) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasName("allGophers"),
					hasVariableDefinitions(
						node(
							hasName("color"),
						),
					),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("without variables", func(t *testing.T) {
		run(`query allGophers@rename(index: 3) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasName("allGophers"),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("unnamed", func(t *testing.T) {
		run(`query ($color: String)@rename(index: 3) {
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasFields(
						node(
							hasName("name"),
						),
					),
				),
			),
		)
	})
	t.Run("all optional", func(t *testing.T) {
		run(`{
					name
				}`,
			mustParseOperationDefinition(
				node(
					hasOperationType(document.OperationTypeQuery),
					hasFields(
						node(
							hasName("name"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   3,
						CharEnd:   6,
					}),
				),
			),
		)
	})
	t.Run("invalid ", func(t *testing.T) {
		run(` query allGophers($color: [String!) {
					name
				}`,
			mustPanic(
				mustParseOperationDefinition(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophers"),
						hasVariableDefinitions(
							node(
								hasName("color"),
							),
						),
						hasFields(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
	t.Run("invalid ", func(t *testing.T) {
		run(` query allGophers($color: String!) @foo(bar: .) {
					name
				}`,
			mustPanic(
				mustParseOperationDefinition(
					node(
						hasOperationType(document.OperationTypeQuery),
						hasName("allGophers"),
						hasVariableDefinitions(
							node(
								hasName("color"),
							),
						),
						hasFields(
							node(
								hasName("name"),
							),
						),
					),
				),
			),
		)
	})
}
