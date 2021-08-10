package astnormalization

import (
	"testing"

	"github.com/jensneuse/graphql-go-tools/pkg/astparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/stretchr/testify/assert"
)

func runNormalizeSubgraphSDL(t *testing.T, name, input, expectedOutput string, norm ...registerNormalizeFunc) {
	inDoc, _ := astparser.ParseGraphqlDocumentString(input)
	outDoc, _ := astparser.ParseGraphqlDocumentString(expectedOutput)
	expectedOutput, _ = astprinter.PrintString(&outDoc, nil)
	walker := astvisitor.NewWalker(48)
	for i := range norm {
		norm[i](&walker)
	}
	walker.Walk(&inDoc, nil, nil)
	input, _ = astprinter.PrintString(&inDoc, nil)
	t.Run(name, func(t *testing.T) {
		assert.Equal(t, input, expectedOutput)
	})
}

func TestNormalizeSubgraphSDL(t *testing.T) {
	runNormalizeSubgraphSDL(t, "support both extends directive and implicit extend keyword",
		`
		type User @extends {
			field: String!
		}
		type Query @extends {}
		`,
		`
		extend type User { field: String! } extend type Query @extends {}
	`, registerNormalizeFunc(implicitExtendRootOperation), registerNormalizeFunc(extendsDirective))
	runNormalizeSubgraphSDL(t, "support both extends directive and implicit extend keyword in schema",
		`
		schema {
			query: AQuery
		}
		type User @extends @directiv2 {
			field: String!
		}
		type AQuery @key("id") {
			field: String
		}
		`,
		`
		schema {
			query: AQuery
		}
		extend type User @directiv2 { field: String! }
		extend type AQuery @key("id") {
			field: String
		}
	`, registerNormalizeFunc(implicitExtendRootOperation), registerNormalizeFunc(extendsDirective))
}
