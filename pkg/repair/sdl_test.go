package repair

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeprinter"
)

func TestSDL(t *testing.T) {
	t.Run("should remove empty input object type definitions", func(t *testing.T) {
		input := `
type String
type Query { foo(a: A): String! }
input A {
	b: B
}
input B {}
`
		expected := `
type String
type Query { foo: String! }
`
		actual,err := SDL(input)
		assert.NoError(t, err)
		assert.Equal(t, unsafeprinter.Prettify(expected),unsafeprinter.Prettify(actual))
	})
}
