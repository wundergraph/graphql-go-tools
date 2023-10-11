package plan

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestInterfaceSelectionRewriter_RewriteOperation(t *testing.T) {
	run := func(t *testing.T, definition string, dsConfiguration *DataSourceConfiguration, operation string, expectedOperation string) {
		t.Helper()

		op := unsafeparser.ParseGraphqlDocumentString(operation)
		def := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definition)

		fieldRef := ast.InvalidRef
		for ref, _ := range op.Fields {
			if op.FieldNameString(ref) == "iface" {
				fieldRef = ref
				break
			}
		}

		node, _ := def.Index.FirstNodeByNameStr("Query")

		rewriter := newInterfaceSelectionRewriter(&op, &def)
		rewriter.RewriteOperation(fieldRef, node, dsConfiguration)

		printedOp := unsafeprinter.PrettyPrint(&op, &def)
		expectedPretty := unsafeprinter.Prettify(expectedOperation)

		require.Equal(t, expectedPretty, printedOp)
	}

	t.Run("simple", func(t *testing.T) {
		definition := `
			interface Node {
				id: ID!
				name: String!
			}
			
			type User implements Node {
				id: ID!
				name: String!
			}

			type Admin implements Node {
				id: ID!
				name: String! @external
			}
			
			type Query {
				iface: Node!
			}
		`

		operation := `
			query {
				iface {
					name
				}
			}`

		expectedOperation := `
			query {
				iface {
					... on User {
						name
					}
					... on Admin {
						name
					}
				}
			}
		`

		dsConfiguration := dsb().
			RootNode("Query", "iface").
			RootNode("User", "id", "name").
			RootNode("Admin", "id").ds

		dsConfiguration.FederationMetaData.Keys = FederationFieldConfigurations{
			{
				TypeName:     "User",
				SelectionSet: "id",
			},
			{
				TypeName:     "Admin",
				SelectionSet: "id",
			},
		}

		run(t, definition, dsConfiguration, operation, expectedOperation)
	})
}
