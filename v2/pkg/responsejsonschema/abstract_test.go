package responsejsonschema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildResponseSchema_InterfaceVariants(t *testing.T) {
	definitionInput := `
		directive @inaccessible on OBJECT

		type Query {
			node: Node
			nodes: [Node!]!
		}

		interface Node {
			id: ID!
		}

		type Product implements Node {
			id: ID!
			sku: String!
		}

		type User implements Node {
			id: ID!
			username: String!
		}

		type InternalNode implements Node @inaccessible {
			id: ID!
			secret: String!
		}
	`
	operationDirectFirst := `
		query {
			node {
				__typename
				id
				... on User {
					username
				}
				...ProductFields
				... on InternalNode {
					secret
				}
			}
		}

		fragment ProductFields on Product {
			sku
		}
	`
	operationFragmentFirst := `
		fragment ProductFields on Product {
			sku
		}

		query {
			node {
				...ProductFields
				... on InternalNode {
					secret
				}
				id
				... on User {
					username
				}
				__typename
			}
		}
	`

	directFirst := buildSchema(t, definitionInput, operationDirectFirst, []string{"node"})
	fragmentFirst := buildSchema(t, definitionInput, operationFragmentFirst, []string{"node"})
	require.Equal(t, directFirst, fragmentFirst)
	require.Equal(t, []string{"Product", "User"}, abstractVariantTypeNames(t, directFirst))

	requireSchemaValidation(
		t,
		directFirst,
		[]string{
			`null`,
			`{"__typename":"Product","id":"product-1","sku":"SKU-1"}`,
			`{"__typename":"User","id":"user-1","username":"ada"}`,
		},
		[]string{
			`{"id":"user-1","username":"ada"}`,
			`{"__typename":"InternalNode","id":"internal-1","secret":"hidden"}`,
			`{"__typename":"User","id":"user-1","sku":"SKU-1"}`,
			`{"__typename":"Product","id":"product-1","sku":"SKU-1","username":"ada"}`,
		},
	)

	listSchema := buildSchema(
		t,
		definitionInput,
		`query {
			nodes {
				__typename
				id
				... on Product { sku }
				... on User { username }
			}
		}`,
		[]string{"nodes"},
	)
	requireSchemaValidation(
		t,
		listSchema,
		[]string{
			`[]`,
			`[{"__typename":"User","id":"user-1","username":"ada"},{"__typename":"Product","id":"product-1","sku":"SKU-1"}]`,
		},
		[]string{
			`null`,
			`[null]`,
			`[{"__typename":"User","id":"user-1","sku":"SKU-1"}]`,
		},
	)
}

func TestBuildResponseSchema_UnionVariants(t *testing.T) {
	definitionInput := `
		directive @inaccessible on OBJECT

		type Query {
			search: SearchResult!
			searches: [SearchResult]!
		}

		interface Identified {
			id: ID!
		}

		union SearchResult = Product | User | InternalResult

		type Product implements Identified {
			id: ID!
			sku: String!
		}

		type User implements Identified {
			id: ID!
			username: String!
		}

		type InternalResult implements Identified @inaccessible {
			id: ID!
			secret: String!
		}
	`
	selection := `
		__typename
		... on Identified { id }
		... on Product { sku }
		... on User { username }
		... on InternalResult { secret }
	`

	schema := buildSchema(
		t,
		definitionInput,
		`query {
			search {`+selection+`}
		}`,
		[]string{"search"},
	)
	require.Equal(t, []string{"Product", "User"}, abstractVariantTypeNames(t, schema))
	requireSchemaValidation(
		t,
		schema,
		[]string{
			`{"__typename":"Product","id":"product-1","sku":"SKU-1"}`,
			`{"__typename":"User","id":"user-1","username":"ada"}`,
		},
		[]string{
			`null`,
			`{"__typename":"InternalResult","id":"internal-1","secret":"hidden"}`,
			`{"__typename":"Product","id":"product-1","username":"ada"}`,
			`{"__typename":"User","id":"user-1","username":"ada","sku":"SKU-1"}`,
		},
	)

	listSchema := buildSchema(
		t,
		definitionInput,
		`query {
			searches {`+selection+`}
		}`,
		[]string{"searches"},
	)
	requireSchemaValidation(
		t,
		listSchema,
		[]string{
			`[]`,
			`[null,{"__typename":"Product","id":"product-1","sku":"SKU-1"}]`,
		},
		[]string{
			`null`,
			`[{"__typename":"User","id":"user-1","sku":"SKU-1"}]`,
		},
	)
}

func abstractVariantTypeNames(t *testing.T, schema json.RawMessage) []string {
	t.Helper()

	var root map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(schema, &root))
	variantContainer := schema
	if rawAnyOf, ok := root["anyOf"]; ok {
		var alternatives []json.RawMessage
		require.NoError(t, json.Unmarshal(rawAnyOf, &alternatives))
		for _, alternative := range alternatives {
			var decoded map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(alternative, &decoded))
			if _, ok := decoded["oneOf"]; ok {
				variantContainer = alternative
				break
			}
		}
	}

	var decodedContainer struct {
		OneOf []json.RawMessage `json:"oneOf"`
	}
	require.NoError(t, json.Unmarshal(variantContainer, &decodedContainer))
	require.NotEmpty(t, decodedContainer.OneOf)

	typeNames := make([]string, 0, len(decodedContainer.OneOf))
	for _, variant := range decodedContainer.OneOf {
		var decodedVariant struct {
			Properties           map[string]json.RawMessage `json:"properties"`
			Required             []string                   `json:"required"`
			AdditionalProperties *bool                      `json:"additionalProperties"`
		}
		require.NoError(t, json.Unmarshal(variant, &decodedVariant))
		require.Contains(t, decodedVariant.Required, "__typename")
		require.NotNil(t, decodedVariant.AdditionalProperties)
		require.False(t, *decodedVariant.AdditionalProperties)

		var typenameProperty struct {
			Const string `json:"const"`
		}
		require.NoError(t, json.Unmarshal(decodedVariant.Properties["__typename"], &typenameProperty))
		require.NotEmpty(t, typenameProperty.Const)
		typeNames = append(typeNames, typenameProperty.Const)
	}
	return typeNames
}
