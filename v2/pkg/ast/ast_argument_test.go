package ast_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
)

func TestArgumentSetsAreEqualUnordered(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, left, right string
		want              bool
	}{
		{"Empty", ``, ``, true},
		{"MatchingOrder", `(z: $id, a: 2)`, `(z: $id, a: 2)`, true},
		{"ReorderedArguments", `(a: 1, b: 2, c: 3)`, `(a: 1, c: 3, b: 2)`, true},
		{"ConflictingArguments", `(a: 1, b: 2)`, `(b: 2, a: 3)`, false},
		{"MatchingObjectOrder", `(input: {z: 1, a: 2})`, `(input: {z: 1, a: 2})`, true},
		{"ReorderedNestedObjects", `(input: [{nested: {a: 1, b: 2}}, {a: 3}])`, `(input: [{nested: {b: 2, a: 1}}, {a: 3}])`, true},
		{"ConflictingObjectFields", `(input: {a: 1, b: 2})`, `(input: {b: 3, a: 1})`, false},
		{"MissingArgument", `(id: 1)`, ``, false},
		{"DifferentArgumentNames", `(id: 1)`, `(other: 1)`, false},
		{"DifferentVariables", `(id: $a)`, `(id: $b)`, false},
		{"DifferentObjectFieldNames", `(input: {a: 1})`, `(input: {b: 1})`, false},
		{"MissingObjectField", `(input: {a: 1})`, `(input: {a: 1, b: 2})`, false},
		{"ListOrder", `(input: [1, 2])`, `(input: [2, 1])`, false},
		{"ListLength", `(input: [1])`, `(input: [1, 2])`, false},
		{"ValueKinds", `(id: 1)`, `(id: "1")`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			operation, report := astparser.ParseGraphqlDocumentString(fmt.Sprintf(`{ left%s right%s }`, tc.left, tc.right))
			require.False(t, report.HasErrors(), report.Error())
			before, err := astprinter.PrintString(&operation)
			require.NoError(t, err)

			left, right := operation.FieldArguments(0), operation.FieldArguments(1)
			assert.Equal(t, tc.want, operation.ArgumentSetsAreEqualUnordered(left, right))
			assert.Equal(t, tc.want, operation.ArgumentSetsAreEqualUnordered(right, left))
			after, err := astprinter.PrintString(&operation)
			assert.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}
