package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/representationvariable"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

const keyBuilderDefinition = `
	scalar String
	scalar Int

	type Product {
		upc: String!
		sku: String!
		name: String!
		brand: Brand!
	}

	type Brand {
		id: String!
		name: String!
	}
`

// newKeyBuilderFederation builds initialized federation metadata (entity index
// + parsed key selection sets) for the given Product key sets.
func newKeyBuilderFederation(t *testing.T, keySelectionSets ...string) plan.FederationMetaData {
	t.Helper()
	keys := make(plan.FederationFieldConfigurations, 0, len(keySelectionSets))
	for _, selectionSet := range keySelectionSets {
		keys = append(keys, plan.FederationFieldConfiguration{
			TypeName:     "Product",
			SelectionSet: selectionSet,
		})
	}
	metadata := &plan.DataSourceMetadata{
		FederationMetaData: plan.FederationMetaData{Keys: keys},
	}
	require.NoError(t, metadata.Init())
	return metadata.FederationMetaData
}

func newKeyBuilder(t *testing.T, definition *ast.Document, fed plan.FederationMetaData) *cacheKeyBuilder {
	t.Helper()
	return &cacheKeyBuilder{
		federation: map[string]plan.FederationMetaData{"products": fed},
		definition: definition,
	}
}

func parseKeyBuilderDefinition(t *testing.T) *ast.Document {
	t.Helper()
	definition, report := astparser.ParseGraphqlDocumentString(keyBuilderDefinition)
	require.False(t, report.HasErrors(), "parse definition: %v", report)
	return &definition
}

func productEntityInfo() *resolve.FetchInfo {
	return &resolve.FetchInfo{
		DataSourceID: "products",
		RootFields:   []resolve.GraphCoordinate{{TypeName: "Product"}},
	}
}

func TestCacheKeyBuilderEntitySpec(t *testing.T) {
	definition := parseKeyBuilderDefinition(t)

	t.Run("single @key", func(t *testing.T) {
		builder := newKeyBuilder(t, definition, newKeyBuilderFederation(t, "upc"))
		spec, ok := builder.buildEntitySpec(productEntityInfo())
		require.True(t, ok)
		assert.Equal(t, resolve.CacheKeySpec{
			Scope:    resolve.CacheScopeEntity,
			TypeName: "Product",
			Candidates: []resolve.CacheKeyCandidate{
				{
					Representation: &resolve.Object{
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("Product")},
							},
							{
								Name:        []byte("upc"),
								Value:       &resolve.String{Path: []string{"upc"}},
								OnTypeNames: [][]byte{[]byte("Product")},
							},
						},
					},
				},
			},
		}, spec)
	})

	t.Run("multiple @key sets ordered by selection set, composite + nested", func(t *testing.T) {
		// Deliberately registered in reverse order; candidates must come out
		// sorted by selection-set string ("sku brand { id }" < "upc").
		builder := newKeyBuilder(t, definition, newKeyBuilderFederation(t, "upc", "sku brand { id }"))
		spec, ok := builder.buildEntitySpec(productEntityInfo())
		require.True(t, ok)
		assert.Equal(t, resolve.CacheKeySpec{
			Scope:    resolve.CacheScopeEntity,
			TypeName: "Product",
			Candidates: []resolve.CacheKeyCandidate{
				{
					Representation: &resolve.Object{
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("Product")},
							},
							{
								Name:        []byte("sku"),
								Value:       &resolve.String{Path: []string{"sku"}},
								OnTypeNames: [][]byte{[]byte("Product")},
							},
							{
								Name: []byte("brand"),
								Value: &resolve.Object{
									Path: []string{"brand"},
									Fields: []*resolve.Field{
										{
											Name:  []byte("id"),
											Value: &resolve.String{Path: []string{"id"}},
										},
									},
								},
								OnTypeNames: [][]byte{[]byte("Product")},
							},
						},
					},
				},
				{
					Representation: &resolve.Object{
						Nullable: true,
						Fields: []*resolve.Field{
							{
								Name:        []byte("__typename"),
								Value:       &resolve.String{Path: []string{"__typename"}},
								OnTypeNames: [][]byte{[]byte("Product")},
							},
							{
								Name:        []byte("upc"),
								Value:       &resolve.String{Path: []string{"upc"}},
								OnTypeNames: [][]byte{[]byte("Product")},
							},
						},
					},
				},
			},
		}, spec)
	})

	t.Run("no @key means no spec", func(t *testing.T) {
		builder := newKeyBuilder(t, definition, newKeyBuilderFederation(t))
		spec, ok := builder.buildEntitySpec(productEntityInfo())
		assert.False(t, ok)
		assert.Equal(t, resolve.CacheKeySpec{}, spec)
	})

	t.Run("unknown datasource means no spec", func(t *testing.T) {
		builder := newKeyBuilder(t, definition, newKeyBuilderFederation(t, "upc"))
		spec, ok := builder.buildEntitySpec(&resolve.FetchInfo{
			DataSourceID: "unknown",
			RootFields:   []resolve.GraphCoordinate{{TypeName: "Product"}},
		})
		assert.False(t, ok)
		assert.Equal(t, resolve.CacheKeySpec{}, spec)
	})

	t.Run("nil info and missing root fields mean no spec", func(t *testing.T) {
		builder := newKeyBuilder(t, definition, newKeyBuilderFederation(t, "upc"))
		_, ok := builder.buildEntitySpec(nil)
		assert.False(t, ok)
		_, ok = builder.buildEntitySpec(&resolve.FetchInfo{DataSourceID: "products"})
		assert.False(t, ok)
	})

	t.Run("a broken @key candidate is skipped, others survive", func(t *testing.T) {
		builder := newKeyBuilder(t, definition, newKeyBuilderFederation(t, "doesNotExist", "upc"))
		spec, ok := builder.buildEntitySpec(productEntityInfo())
		require.True(t, ok)
		require.Len(t, spec.Candidates, 1)
		assert.Equal(t, resolve.CacheKeyCandidate{
			Representation: &resolve.Object{
				Nullable: true,
				Fields: []*resolve.Field{
					{
						Name:        []byte("__typename"),
						Value:       &resolve.String{Path: []string{"__typename"}},
						OnTypeNames: [][]byte{[]byte("Product")},
					},
					{
						Name:        []byte("upc"),
						Value:       &resolve.String{Path: []string{"upc"}},
						OnTypeNames: [][]byte{[]byte("Product")},
					},
				},
			},
		}, spec.Candidates[0])
	})

	t.Run("every @key broken means no spec", func(t *testing.T) {
		builder := newKeyBuilder(t, definition, newKeyBuilderFederation(t, "doesNotExist"))
		spec, ok := builder.buildEntitySpec(productEntityInfo())
		assert.False(t, ok)
		assert.Equal(t, resolve.CacheKeySpec{}, spec)
	})

	t.Run("no pointer aliasing: mutating the source federation leaves the spec intact", func(t *testing.T) {
		fed := newKeyBuilderFederation(t, "upc")
		builder := newKeyBuilder(t, definition, fed)
		spec, ok := builder.buildEntitySpec(productEntityInfo())
		require.True(t, ok)

		before := resolve.CacheKeySpec{
			Scope:      spec.Scope,
			TypeName:   spec.TypeName,
			Candidates: spec.Candidates,
		}
		// Mutate the source federation metadata AND the builder's map copy.
		fed.Keys[0].TypeName = "Mutated"
		fed.Keys[0].SelectionSet = "mutated"
		builderFed := builder.federation["products"]
		if len(builderFed.Keys) > 0 {
			builderFed.Keys[0].TypeName = "AlsoMutated"
		}

		assert.True(t, before.Equals(spec))
		assert.Equal(t, "Product", spec.TypeName)
		assert.Equal(t, [][]byte{[]byte("Product")}, spec.Candidates[0].Representation.Fields[0].OnTypeNames)
	})

	t.Run("candidate node equals the datasource representation node for the same @key", func(t *testing.T) {
		fed := newKeyBuilderFederation(t, "sku brand { id }")
		builder := newKeyBuilder(t, definition, fed)
		spec, ok := builder.buildEntitySpec(productEntityInfo())
		require.True(t, ok)
		require.Len(t, spec.Candidates, 1)

		datasourceNode, err := representationvariable.BuildRepresentationVariableNode(definition, plan.FederationFieldConfiguration{
			TypeName:     "Product",
			SelectionSet: "sku brand { id }",
		}, fed)
		require.NoError(t, err)
		assert.Equal(t, datasourceNode, spec.Candidates[0].Representation)
	})
}
