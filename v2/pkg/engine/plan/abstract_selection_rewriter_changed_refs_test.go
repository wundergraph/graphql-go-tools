package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeprinter"
)

// TestFieldSelectionRewriter_ChangedFieldRefs verifies that after a rewrite the mapping
// between original and new field refs respects inline fragment type condition scopes.
// Fields with the same name at the same query depth but under non-intersecting type conditions
// must not be mapped to each other - otherwise a planner-added (skipped) field and
// a user-requested field are conflated and skip status propagates to the wrong refs.
func TestFieldSelectionRewriter_ChangedFieldRefs(t *testing.T) {
	definition := `
		interface Node {
			id: ID!
		}

		type A implements Node {
			id: ID!
		}

		type B implements Node {
			id: ID!
			externalField: String!
		}

		type C implements Node {
			id: ID!
		}

		type Query {
			nodes: Node
		}
	`

	// type C is absent in the upstream schema - a fragment on C triggers the rewrite
	upstreamDefinition := `
		interface Node {
			id: ID!
		}

		type A implements Node @key(fields: "id") {
			id: ID!
		}

		type B implements Node @key(fields: "id") {
			id: ID!
		}

		type Query {
			nodes: Node
		}
	`

	run := func(t *testing.T, operation string, expectedOperation string, expectedChangedRefs map[int][]int, expectedOrigins map[int][]int) {
		t.Helper()

		op := unsafeparser.ParseGraphqlDocumentString(operation)
		def := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(definition)

		fieldRef := ast.InvalidRef
		for ref := range op.Fields {
			if op.FieldNameString(ref) == "nodes" {
				fieldRef = ref
				break
			}
		}
		require.NotEqual(t, ast.InvalidRef, fieldRef)

		ds := dsb().
			RootNode("Query", "nodes").
			RootNode("A", "id").
			RootNode("B", "id").
			ChildNode("Node", "id").
			KeysMetadata(FederationFieldConfigurations{
				{TypeName: "A", SelectionSet: "id"},
				{TypeName: "B", SelectionSet: "id"},
			}).
			SchemaMergedWithBase(upstreamDefinition).
			DS()

		node, _ := def.Index.FirstNodeByNameStr("Query")

		rewriter, err := newFieldSelectionRewriter(&op, &def, ds)
		require.NoError(t, err)

		result, err := rewriter.RewriteFieldSelection(fieldRef, node)
		require.NoError(t, err)
		require.True(t, result.rewritten)

		assert.Equal(t, unsafeprinter.Prettify(expectedOperation), unsafeprinter.PrettyPrint(&op))
		assert.Equal(t, expectedChangedRefs, result.changedFieldRefs)
		assert.Equal(t, expectedOrigins, result.fieldRefOrigins)
	}

	t.Run("user field in one fragment - duplicated field in another fragment", func(t *testing.T) {
		// simulates: user requested id on A only; planner added a skipped key id inside the B fragment.
		// refs before: 0 - id in A, 1 - externalField in B, 2 - id in B, 3 - id in C, 4 - nodes
		// refs after: 5 - id in A, 6 - externalField in B, 7 - id in B
		run(t,
			`query { nodes { ... on A { id } ... on B { externalField id } ... on C { id } } }`,
			`query {
				nodes {
					... on A {
						id
					}
					... on B {
						externalField
						id
					}
				}
			}`,
			map[int][]int{
				0: {5},
				1: {6},
				2: {7},
				// ref 3 disappeared together with the fragment on C
			},
			map[int][]int{
				4: {4},
				5: {0}, // id in A originates only from the user field - must not inherit skip status from ref 2
				6: {1},
				7: {2}, // id in B originates only from the planner field - stays skipped
			},
		)
	})

	t.Run("user field on interface level - duplicated field in fragment", func(t *testing.T) {
		// simulates: user requested id for all nodes; planner added a skipped key id inside the B fragment.
		// refs before: 0 - id on interface level, 1 - externalField in B, 2 - id in B, 3 - id in C, 4 - nodes
		// refs after: 5 - id in A, 6 - id in B, 7 - externalField in B
		run(t,
			`query { nodes { id ... on B { externalField id } ... on C { id } } }`,
			`query {
				nodes {
					... on A {
						id
					}
					... on B {
						id
						externalField
					}
				}
			}`,
			map[int][]int{
				0: {5, 6},
				1: {7},
				2: {6},
			},
			map[int][]int{
				4: {4},
				5: {0},
				6: {0, 2}, // merged user and planner fields - user field wins, must stay in the response
				7: {1},
			},
		)
	})

	t.Run("aliased duplicate does not conflate with the field name", func(t *testing.T) {
		// aliased: id and id are distinct response positions and must be tracked separately
		// refs before: 0 - aliased: id on interface level, 1 - externalField in B, 2 - id in B, 3 - id in C, 4 - nodes
		// refs after: 5 - aliased: id in A, 6 - aliased: id in B, 7 - externalField in B, 8 - id in B
		run(t,
			`query { nodes { aliased: id ... on B { externalField id } ... on C { id } } }`,
			`query {
				nodes {
					... on A {
						aliased: id
					}
					... on B {
						aliased: id
						externalField
						id
					}
				}
			}`,
			map[int][]int{
				0: {5, 6},
				1: {7},
				2: {8},
			},
			map[int][]int{
				4: {4},
				5: {0},
				6: {0},
				7: {1},
				8: {2},
			},
		)
	})
}

// TestNodeSelectionVisitor_UpdateSkipFieldRefs verifies the skip propagation semantics:
// a field ref present after a rewrite is skipped only when all original refs
// occupying the same response position were skipped.
func TestNodeSelectionVisitor_UpdateSkipFieldRefs(t *testing.T) {
	c := &nodeSelectionVisitor{
		skipFieldsRefs: []int{2, 9},
	}

	c.updateSkipFieldRefs(map[int][]int{
		5: {0},    // origin is a user field - stays visible
		6: {0, 2}, // user and planner fields merged - stays visible
		7: {2},    // origin is a planner field - becomes skipped
		9: {0, 9}, // previously skipped ref survived a merge with a user field - must be unskipped
	})

	assert.ElementsMatch(t, []int{2, 7}, c.skipFieldsRefs)
}
