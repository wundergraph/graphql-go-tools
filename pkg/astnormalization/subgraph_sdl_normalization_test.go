package astnormalization

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeSubgraphSDL(t *testing.T) {
	run := func(name, input, expectedOutput string, norm registerNormalizeFunc) {
		inDoc, _ := astparser.ParseGraphqlDocumentString(input)
		outDoc, _ := astparser.ParseGraphqlDocumentString(expectedOutput)
		expectedOutput, _ = astprinter.PrintString(&outDoc, nil)
		walker := astvisitor.NewWalker(48)
		norm(&walker)
		walker.Walk(&inDoc, nil, nil)
		input, _ = astprinter.PrintString(&inDoc, nil)
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, input, expectedOutput)
		})
	}
	run("implicit extend root operation from schema",
		`schema {
			query: QueryName
			mutation: MutationName
		}
		type QueryName @hello {
		}
		type MutationName {
			field: String!
		}
		scalar String
		`,
		`
		schema { query: QueryName mutation: MutationName }
		extend type QueryName @hello{}
		extend type MutationName { field: String! }
		scalar String
	`, registerNormalizeFunc(implicitExtendRootOperation))
	run("don't implicitly extend empty schema root operation",
		`schema {
			query: QueryName
		}
		type QueryName {
		}
		`,
		`
		schema { query: QueryName }
		type QueryName {}
	`, registerNormalizeFunc(implicitExtendRootOperation))
	run("don't implicitly extend empty object root operation",
		`type Query {}
		type Mutation {
			field: String!
		}
		type Subscription @directive {
		}
		`,
		`
		type Query {}
		extend type Mutation { field: String! }
		extend type Subscription @directive {}
	`, registerNormalizeFunc(implicitExtendRootOperation))
	run("implicitly extend object root operation with definitions and directives",
		`type Query {}
		type Mutation {
			field: String!
		}
		type Subscription @directive {
			newUser: ID!
		}
		`,
		`
		type Query {}
		extend type Mutation { field: String! }
		extend type Subscription @directive { newUser: ID! }
	`, registerNormalizeFunc(implicitExtendRootOperation))
}
