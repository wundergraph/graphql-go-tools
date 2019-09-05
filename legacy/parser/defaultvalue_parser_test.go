package parser

import (
	"github.com/jensneuse/graphql-go-tools/legacy/document"
	"testing"
)

func TestParser_parseDefaultValue(t *testing.T) {
	t.Run("integer", func(t *testing.T) {
		run("= 2", mustParseDefaultValue(document.ValueTypeInt))
	})
	t.Run("bool", func(t *testing.T) {
		run("= true", mustParseDefaultValue(document.ValueTypeBoolean))
	})
	t.Run("invalid", func(t *testing.T) {
		run("true", mustPanic(mustParseDefaultValue(document.ValueTypeBoolean)))
	})
}
