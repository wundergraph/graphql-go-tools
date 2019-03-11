package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseDirectives(t *testing.T) {
	t.Run(`simple directive`, func(t *testing.T) {
		run(`@rename(index: 3)`,
			mustParseDirectives(
				node(
					hasName("rename"),
					hasArguments(
						node(
							hasName("index"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   18,
					}),
				),
			),
		)
	})
	t.Run("multiple directives", func(t *testing.T) {
		run(`@rename(index: 3)@moveto(index: 4)`,
			mustParseDirectives(
				node(
					hasName("rename"),
					hasArguments(
						node(
							hasName("index"),
						),
					),
				),
				node(
					hasName("moveto"),
					hasArguments(
						node(
							hasName("index"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 18,
						CharEnd:   35,
					}),
				),
			),
		)
	})
	t.Run("multiple arguments", func(t *testing.T) {
		run(`@rename(index: 3, count: 10)`,
			mustParseDirectives(
				node(
					hasName("rename"),
					hasArguments(
						node(
							hasName("index"),
						),
						node(
							hasName("count"),
						),
					),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 1,
						CharEnd:   29,
					}),
				),
			),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run(`@rename(index)`,
			mustPanic(mustParseDirectives()),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run(`@1337(index)`,
			mustPanic(mustParseDirectives()),
		)
	})
}
