package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexing/position"
	"testing"
)

func TestParser_parseArgumentSet(t *testing.T) {
	t.Run("string argument", func(t *testing.T) {
		run(`(name: "Gophus")`,
			mustParseArguments(
				node(
					hasName("name"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 2,
						CharEnd:   16,
					}),
				),
			),
		)
	})
	t.Run("multiple argument sets", func(t *testing.T) {
		run(`(name: "Gophus")(name2: "Gophus")`,
			mustParseArguments(
				node(
					hasName("name"),
				),
			),
			mustParseArguments(
				node(
					hasName("name2"),
				),
			),
		)
	})
	t.Run("multiple argument sets", func(t *testing.T) {
		run(`(name: "Gophus")()`,
			mustParseArguments(
				node(
					hasName("name"),
				),
			),
			mustPanic(mustParseArguments(
				node(
					hasName("name2"),
				),
			)),
		)
	})
	t.Run("string array argument", func(t *testing.T) {
		run(`(fooBars: ["foo","bar"])`,
			mustParseArguments(
				node(
					hasName("fooBars"),
				),
			),
		)
	})
	t.Run("int array argument", func(t *testing.T) {
		run(`(integers: [1,2,3])`,
			mustParseArguments(
				node(
					hasName("integers"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 2,
						CharEnd:   19,
					}),
				),
			),
		)
	})
	t.Run("multiple string arguments", func(t *testing.T) {
		run(`(name: "Gophus", surname: "Gophersson")`,
			mustParseArguments(
				node(
					hasName("name"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 2,
						CharEnd:   16,
					}),
				),
				node(
					hasName("surname"),
					hasPosition(position.Position{
						LineStart: 1,
						LineEnd:   1,
						CharStart: 18,
						CharEnd:   39,
					}),
				),
			),
		)
	})
	t.Run("invalid argument must err", func(t *testing.T) {
		run(`(name: "Gophus", surname: "Gophersson"`,
			mustPanic(mustParseArguments()))
	})
	t.Run("invalid argument must err 2", func(t *testing.T) {
		run(`((name: "Gophus", surname: "Gophersson")`,
			mustPanic(mustParseArguments()))
	})
	t.Run("invalid argument must err 3", func(t *testing.T) {
		run(`(name: .)`,
			mustPanic(mustParseArguments()))
	})
}
