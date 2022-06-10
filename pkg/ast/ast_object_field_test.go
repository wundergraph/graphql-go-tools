package ast_test

import (
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_ObjectValueObjectFieldByName(t *testing.T) {
	testCases := []struct {
		name        string
		schema      string
		expectedRef int
		inputVal    func() (int, string)
	}{
		{
			name: "successfully get ref of object field",
			schema: `
			query {
			    testQuery(testInput: {firstVal: "hello"})
			}`,
			expectedRef: 0,
			inputVal: func() (int, string) {
				return 0, "firstVal"
			},
		},
		{
			name: "successfully get ref of object field multiple",
			schema: `
			query {
			    testQuery(testInput: {firstVal: "hello", secondVal: 1})
			}`,
			expectedRef: 1,
			inputVal: func() (int, string) {
				return 0, "secondVal"
			},
		},
		{
			name: "invalid object value",
			schema: `
			query {
			    testQuery(testInput: {firstVal: "hello"})
			}`,
			expectedRef: -1,
			inputVal: func() (int, string) {
				return 1, "firstVal"
			},
		},
		{
			name: "invalid object field name",
			schema: `
			query {
			    testQuery(testInput: {firstVal: "hello"})
			}`,
			expectedRef: -1,
			inputVal: func() (int, string) {
				return 0, "firstVals"
			},
		},
	}

	for _, scenario := range testCases {
		t.Run(scenario.name, func(t *testing.T) {
			doc := unsafeparser.ParseGraphqlDocumentString(scenario.schema)
			objectParam, valueNameParam := scenario.inputVal()
			assert.Equal(t, scenario.expectedRef, doc.ObjectValueObjectFieldByName(objectParam, ast.ByteSlice([]byte(valueNameParam))))
		})
	}
}
