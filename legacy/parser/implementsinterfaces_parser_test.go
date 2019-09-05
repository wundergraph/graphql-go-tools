package parser

import (
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/keyword"
	"testing"
)

func TestParser_parseImplementsInterfaces(t *testing.T) {
	t.Run("simple", func(t *testing.T) {
		run("implements Dogs",
			mustParseImplementsInterfaces("Dogs"),
		)
	})
	t.Run("multiple", func(t *testing.T) {
		run("implements Dogs & Cats & Mice",
			mustParseImplementsInterfaces("Mice", "Cats", "Dogs"),
		)
	})
	t.Run("multiple without &", func(t *testing.T) {
		run("implements Dogs & Cats Mice",
			mustParseImplementsInterfaces("Cats", "Dogs"),
			mustParseLiteral(keyword.IDENT, "Mice"),
		)
	})
	t.Run("invalid", func(t *testing.T) {
		run("implement Dogs & Cats Mice",
			mustParseImplementsInterfaces(),
			mustParseLiteral(keyword.IDENT, "implement"),
		)
	})
	t.Run("invalid 2", func(t *testing.T) {
		run("implements foo & .",
			mustPanic(mustParseImplementsInterfaces("foo", ".")),
		)
	})
}
