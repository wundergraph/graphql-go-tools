package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseInputValueDefinitions(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run("inputValue: Int",
			mustParseInputValueDefinitions(
				node(
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   16,
					}),
				),
			),
		)
	})
	t.Run("with default", func(t *testing.T) {
		run("inputValue: Int = 2",
			mustParseInputValueDefinitions(
				node(
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
				),
			),
		)
	})
	t.Run("with description", func(t *testing.T) {
		run(`"useful description"inputValue: Int = 2`,
			mustParseInputValueDefinitions(
				node(
					hasDescription("useful description"),
					hasName("inputValue"),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   40,
					}),
				),
			),
		)
	})
	t.Run("multiple with descriptions and defaults", func(t *testing.T) {
		run(`"this is a inputValue"inputValue: Int = 2, "this is a outputValue"outputValue: String = "Out"`,
			mustParseInputValueDefinitions(
				node(
					hasDescription("this is a outputValue"),
					hasName("outputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("String"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 44,
						LineEnd:   1,
						CharEnd:   94,
					}),
				),
				node(
					hasDescription("this is a inputValue"),
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   42,
					}),
				),
			),
		)
	})
	t.Run("with directives", func(t *testing.T) {
		run(`inputValue: Int @fromTop(to: "bottom") @fromBottom(to: "top")`,
			mustParseInputValueDefinitions(
				node(
					hasName("inputValue"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("Int"),
					),
					hasDirectives(
						node(
							hasName("fromTop"),
						),
						node(
							hasName("fromBottom"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 1,
						LineEnd:   1,
						CharEnd:   62,
					}),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("inputValue. foo",
			mustPanic(
				mustParseInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("inputValue: foo @bar(baz: .)",
			mustPanic(
				mustParseInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("inputValue: foo = [1!",
			mustPanic(
				mustParseInputValueDefinitions(
					node(
						hasName("inputValue"),
						nodeType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("Int"),
						),
					),
				),
			),
		)
	})
}
