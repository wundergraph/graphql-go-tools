package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseScalarTypeDefinition(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run("scalar JSON", mustParseScalarTypeDefinition(
			node(
				hasName("JSON"),
			),
		))
	})
	t.Run("with directives", func(t *testing.T) {
		run(`scalar JSON @fromTop(to: "bottom") @fromBottom(to: "top")`, mustParseScalarTypeDefinition(
			node(
				hasName("JSON"),
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
					CharEnd:   58,
				}),
			),
		))
	})
	t.Run("invalid 1", func(t *testing.T) {
		run("scalar 1337",
			mustPanic(
				mustParseScalarTypeDefinition(
					node(
						hasName("1337"),
					),
				),
			),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("scalar JSON @foo(bar: .)",
			mustPanic(
				mustParseScalarTypeDefinition(
					node(
						hasName("1337"),
					),
				),
			),
		)
	})
	t.Run("invalid 3", func(t *testing.T) {
		run("notscalar JSON",
			mustPanic(
				mustParseScalarTypeDefinition(),
			),
		)
	})
}
