package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/keyword"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseInputFieldsDefinition(t *testing.T) {
	t.Run("simple input fields definition", func(t *testing.T) {
		run("{inputValue: Int}",
			mustParseInputFieldsDefinition(
				hasInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   1,
					CharEnd:   18,
				}),
			),
		)
	})
	t.Run("optional", func(t *testing.T) {
		run(" ", mustParseInputFieldsDefinition())
	})
	t.Run("multiple", func(t *testing.T) {
		run("{inputValue: Int, outputValue: String}",
			mustParseInputFieldsDefinition(
				hasInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
					node(
						hasName("outputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("String"),
						),
					),
				),
				hasPosition(position.Position{
					LineStart: 1,
					CharStart: 1,
					LineEnd:   1,
					CharEnd:   39,
				}),
			),
		)
	})
	t.Run("optional", func(t *testing.T) {
		run("inputValue: Int}",
			mustParseInputFieldsDefinition(),
			mustParseLiteral(keyword.IDENT, "inputValue"),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run("{{inputValue: Int}",
			mustPanic(mustParseInputFieldsDefinition()),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("{inputValue: Int",
			mustPanic(mustParseInputFieldsDefinition()),
		)
	})
}
