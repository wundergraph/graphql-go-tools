package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseVariableDefinitions(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run("($foo : bar)",
			mustParseVariableDefinitions(
				node(
					hasName("foo"),
					nodeType(
						hasTypeName("bar"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 2,
						LineEnd:   1,
						CharEnd:   12,
					}),
				),
			),
		)
	})
	t.Run("multiple", func(t *testing.T) {
		run("($foo : bar $baz : bat)",
			mustParseVariableDefinitions(
				node(
					hasName("foo"),
					nodeType(
						hasTypeName("bar"),
					),
				),
				node(
					hasName("baz"),
					nodeType(
						hasTypeName("bat"),
					),
				),
			),
		)
	})
	t.Run("with default", func(t *testing.T) {
		run(`($foo : bar! = "me" $baz : bat)`,
			mustParseVariableDefinitions(
				node(
					hasName("foo"),
					nodeType(
						hasTypeKind(document.TypeKindNON_NULL),
						ofType(
							hasTypeKind(document.TypeKindNAMED),
							hasTypeName("bar"),
						),
					),
					hasDefaultValue(
						hasValueType(document.ValueTypeString),
						hasByteSliceValue("me"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 2,
						LineEnd:   1,
						CharEnd:   20,
					}),
				),
				node(
					hasName("baz"),
					nodeType(
						hasTypeKind(document.TypeKindNAMED),
						hasTypeName("bat"),
					),
					hasPosition(position.Position{
						LineStart: 1,
						CharStart: 21,
						LineEnd:   1,
						CharEnd:   31,
					}),
				),
			),
		)
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("($foo : bar!",
			mustPanic(mustParseVariableDefinitions()))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("($foo . bar!)",
			mustPanic(mustParseVariableDefinitions()))
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("($foo : bar! = . )",
			mustPanic(mustParseVariableDefinitions()))
	})
	t.Run("invalid 4", func(t *testing.T) {
		run("($foo : bar! = \"Baz! )",
			mustPanic(mustParseVariableDefinitions()))
	})
}
