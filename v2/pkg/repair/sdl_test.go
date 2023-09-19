package repair

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeprinter"
)

func TestSDL(t *testing.T) {
	t.Run("should remove empty input object type definitions", func(t *testing.T) {
		input := `
type String
schema {
	query: Query
	mutation: Mutation
}
type Query { foo(a: A): String! }
input A {
	b: B
}
input B {}
type Mutation {
	foo: String!
	bar: Boolean!
}
`
		expected := `
type String
schema {
	query: Query
	mutation: Mutation
}
type Query { foo: String! }
type Mutation {
	foo: String
	bar: Boolean
}
`
		actual, err := SDL(input, OptionsSDL{
			SetAllMutationFieldsNullable: true,
		})
		assert.NoError(t, err)
		assert.Equal(t, unsafeprinter.Prettify(expected), unsafeprinter.Prettify(actual))
	})
}
