package ast_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
)

func TestArgumentSetsAreEqualUnordered(t *testing.T) {
	for _, tc := range []struct {
		left, right string
		equal       bool
	}{
		{``, ``, true},
		{`(id: 1)`, `(id: 1)`, true},
		{`(z: 1, a: 2)`, `(z: 1, a: 2)`, true},
		{`(a: 1, b: 2, c: 3)`, `(c: 3, b: 2, a: 1)`, true},
		{`(input: {z: 1, a: 2})`, `(input: {z: 1, a: 2})`, true},
		{`(input: {a: 1, b: 2, c: 3})`, `(input: {c: 3, b: 2, a: 1})`, true},
		{`(a: 1, b: 2)`, `(b: 2, a: 3)`, false},
		{`(id: 1, name: "a")`, `(name: "a", id: 1)`, true},
		{`(input: {a: 1, b: 2})`, `(input: {b: 2, a: 1})`, true},
		{`(input: [{nested: {a: 1, b: 2}}, {a: 3}])`, `(input: [{nested: {b: 2, a: 1}}, {a: 3}])`, true},
		{`(id: $a)`, `(id: $a)`, true},
		{`(id: 1)`, `(id: 2)`, false},
		{`(id: 1)`, ``, false},
		{`(id: 1)`, `(other: 1)`, false},
		{`(id: $a)`, `(id: $b)`, false},
		{`(input: {a: 1})`, `(input: {b: 1})`, false},
		{`(input: {a: 1, b: 2})`, `(input: {b: 3, a: 1})`, false},
		{`(input: {a: 1})`, `(input: {a: 1, b: 2})`, false},
		{`(input: [1, 2])`, `(input: [2, 1])`, false},
		{`(input: [1])`, `(input: [1, 2])`, false},
		{`(id: 1)`, `(id: "1")`, false},
	} {
		t.Run(fmt.Sprintf("%s_vs_%s", tc.left, tc.right), func(t *testing.T) {
			operation, report := astparser.ParseGraphqlDocumentString(fmt.Sprintf(`{ left%s right%s }`, tc.left, tc.right))
			require.False(t, report.HasErrors(), report.Error())
			before, err := astprinter.PrintString(&operation)
			require.NoError(t, err)
			left, right := operation.FieldArguments(0), operation.FieldArguments(1)
			require.Equal(t, tc.equal, operation.ArgumentSetsAreEqualUnordered(left, right))
			require.Equal(t, tc.equal, operation.ArgumentSetsAreEqualUnordered(right, left))
			after, err := astprinter.PrintString(&operation)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
