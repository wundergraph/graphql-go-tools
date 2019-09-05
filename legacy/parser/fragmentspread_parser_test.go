package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/position"
	"testing"
)

func TestParser_parseFragmentSpread(t *testing.T) {
	t.Run("with directive", func(t *testing.T) {
		run("firstFragment @rename(index: 3)",
			mustParseFragmentSpread(
				node(
					hasName("firstFragment"),
					hasDirectives(
						node(
							hasName("rename"),
						),
					),
					hasPosition(position.Position{
						LineStart: 0, // default, see mustParseFragmentSpread
						CharStart: 0, // default, see mustParseFragmentSpread
						LineEnd:   1,
						CharEnd:   32,
					}),
				),
			),
		)
	})
	t.Run("all optional", func(t *testing.T) {
		run("firstFragment",
			mustParseFragmentSpread(
				node(
					hasName("firstFragment"),
				),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run("on", mustPanic(mustParseFragmentSpread(node())))
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("afragment @foo(bar: .)", mustPanic(mustParseFragmentSpread(node())))
	})
}
