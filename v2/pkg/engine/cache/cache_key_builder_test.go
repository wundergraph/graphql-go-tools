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
		price: Int!
		brand: Brand!
	}

	type Brand {
		id: String!
		name: String!
	}
`

func parseKeyBuilderDefinition(t *testing.T) *ast.Document {
	t.Helper()
	definition, report := astparser.ParseGraphqlDocumentString(keyBuilderDefinition)
	require.False(t, report.HasErrors(), "parse definition: %v", report)
	return &definition
}

// productRepresentation builds the merged representation node a Product entity
// fetch sends, through the planner's own pipeline: one node per RequiredFields
// selection set, merged into one. The FIRST selection set is the chosen @key;
// every further one is a @requires set, which the planner marks by naming the
// field carrying the directive.
func productRepresentation(t *testing.T, selectionSets ...string) *resolve.Object {
	t.Helper()
	configs := make([]plan.FederationFieldConfiguration, 0, len(selectionSets))
	for i, selectionSet := range selectionSets {
		config := plan.FederationFieldConfiguration{TypeName: "Product", SelectionSet: selectionSet}
		if i > 0 {
			config.FieldName = "shippingEstimate"
		}
		configs = append(configs, config)
	}
	return mergedRepresentation(t, plan.FederationMetaData{}, configs...)
}

// mergedRepresentation is productRepresentation for arbitrary types and
// federation metadata, so rows can build the abstract-path (per-concrete-type
// conditioned fields) and @interfaceObject (static __typename) node shapes the
// planner really emits.
func mergedRepresentation(t *testing.T, federation plan.FederationMetaData, configs ...plan.FederationFieldConfiguration) *resolve.Object {
	t.Helper()
	definition := parseKeyBuilderDefinition(t)
	nodes := make([]*resolve.Object, 0, len(configs))
	for _, config := range configs {
		// Caching is what the cache-side fixtures exercise, so the nodes carry
		// the @key marking the caching planner emits.
		node, err := representationvariable.BuildRepresentationVariableNode(definition, config, federation, true)
		require.NoError(t, err)
		nodes = append(nodes, node)
	}
	return representationvariable.MergeRepresentationVariableNodes(nodes)
}

// entityFetchWith wraps a representation node in an EntityFetch exactly as
// createConcreteSingleFetchTypes does: the ResolvableObjectVariable segment
// carrying the node sits in the fetch's item input template.
func entityFetchWith(node *resolve.Object) *resolve.EntityFetch {
	return &resolve.EntityFetch{
		Info: productEntityInfo(),
		Input: resolve.EntityInput{
			Item: resolve.InputTemplate{
				Segments: []resolve.TemplateSegment{resolve.NewResolvableObjectVariable(node).TemplateSegment()},
			},
		},
	}
}

// batchEntityFetchWith is entityFetchWith for the batched arm.
func batchEntityFetchWith(node *resolve.Object) *resolve.BatchEntityFetch {
	return &resolve.BatchEntityFetch{
		Info: productEntityInfo(),
		Input: resolve.BatchInput{
			Items: []resolve.InputTemplate{
				{Segments: []resolve.TemplateSegment{resolve.NewResolvableObjectVariable(node).TemplateSegment()}},
			},
		},
	}
}

func productEntityInfo() *resolve.FetchInfo {
	return &resolve.FetchInfo{
		DataSourceID:   "products",
		DataSourceName: "products",
		RootFields:     []resolve.GraphCoordinate{{TypeName: "Product"}},
	}
}

func TestCacheKeyBuilderEntitySpec(t *testing.T) {
	t.Run("single @key: the spec IS the fetch's representation node", func(t *testing.T) {
		node := productRepresentation(t, "upc")
		fetch := entityFetchWith(node)
		spec, ok := buildEntitySpec(fetch, fetch.Info)
		require.True(t, ok)
		assert.Equal(t, resolve.CacheKeySpec{
			Scope:    resolve.CacheScopeEntity,
			TypeName: "Product",
			Representation: &resolve.Object{
				Nullable: true,
				Fields: []*resolve.Field{
					{
						Name:        []byte("__typename"),
						Value:       &resolve.String{Path: []string{"__typename"}},
						OnTypeNames: [][]byte{[]byte("Product")},
					},
					{
						Name:                []byte("upc"),
						Value:               &resolve.String{Path: []string{"upc"}},
						OnTypeNames:         [][]byte{[]byte("Product")},
						CacheEntityKeyField: true,
					},
				},
			},
		}, spec)
		// Intentional identity: the node is plan-owned and read-only at
		// runtime, so the spec references it instead of copying it.
		assert.Same(t, node, spec.Representation)
	})

	t.Run("a multi-@key type still yields ONE spec: the key set THIS fetch sends", func(t *testing.T) {
		// Product declares both @key(upc) and @key(sku); this fetch resolves it
		// by sku, so sku alone is its identity — the other @key set is not
		// enumerated any more.
		node := productRepresentation(t, "sku")
		fetch := entityFetchWith(node)
		spec, ok := buildEntitySpec(fetch, fetch.Info)
		require.True(t, ok)
		assert.Equal(t, resolve.CacheKeySpec{
			Scope:    resolve.CacheScopeEntity,
			TypeName: "Product",
			Representation: &resolve.Object{
				Nullable: true,
				Fields: []*resolve.Field{
					{
						Name:        []byte("__typename"),
						Value:       &resolve.String{Path: []string{"__typename"}},
						OnTypeNames: [][]byte{[]byte("Product")},
					},
					{
						Name:                []byte("sku"),
						Value:               &resolve.String{Path: []string{"sku"}},
						OnTypeNames:         [][]byte{[]byte("Product")},
						CacheEntityKeyField: true,
					},
				},
			},
		}, spec)
	})

	t.Run("@requires fields are part of the identity, and marked as not-@key", func(t *testing.T) {
		// The planner merges the @key set with every @requires set into ONE
		// representation node; the key renders from all of it, while
		// CacheEntityKeyField keeps the @key subset the entity tag hashes
		// distinguishable inside the merged node.
		node := productRepresentation(t, "upc", "price brand { id }")
		fetch := entityFetchWith(node)
		spec, ok := buildEntitySpec(fetch, fetch.Info)
		require.True(t, ok)
		assert.Equal(t, resolve.CacheKeySpec{
			Scope:    resolve.CacheScopeEntity,
			TypeName: "Product",
			Representation: &resolve.Object{
				Nullable: true,
				Fields: []*resolve.Field{
					{
						Name:        []byte("__typename"),
						Value:       &resolve.String{Path: []string{"__typename"}},
						OnTypeNames: [][]byte{[]byte("Product")},
					},
					{
						Name:                []byte("upc"),
						Value:               &resolve.String{Path: []string{"upc"}},
						OnTypeNames:         [][]byte{[]byte("Product")},
						CacheEntityKeyField: true,
					},
					{
						Name:        []byte("price"),
						Value:       &resolve.Integer{Path: []string{"price"}},
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
		}, spec)
	})

	t.Run("batch entity fetch: the node comes from the per-item template", func(t *testing.T) {
		node := productRepresentation(t, "upc")
		fetch := batchEntityFetchWith(node)
		spec, ok := buildEntitySpec(fetch, fetch.Info)
		require.True(t, ok)
		assert.Same(t, node, spec.Representation)
		assert.Equal(t, resolve.CacheScopeEntity, spec.Scope)
		assert.Equal(t, "Product", spec.TypeName)
	})

	t.Run("determinism: two builds of the same fetch produce equal specs", func(t *testing.T) {
		fetch := entityFetchWith(productRepresentation(t, "upc", "price"))
		first, ok := buildEntitySpec(fetch, fetch.Info)
		require.True(t, ok)
		second, ok := buildEntitySpec(fetch, fetch.Info)
		require.True(t, ok)
		assert.True(t, first.Equals(second))
		assert.Equal(t, first, second)
	})

	noSpecRows := []struct {
		name  string
		fetch resolve.Fetch
		info  *resolve.FetchInfo
	}{
		{
			name:  "a non-entity fetch sends no representation",
			fetch: &resolve.SingleFetch{Info: productEntityInfo()},
			info:  productEntityInfo(),
		},
		{
			name:  "an entity fetch with an empty input template",
			fetch: &resolve.EntityFetch{Info: productEntityInfo()},
			info:  productEntityInfo(),
		},
		{
			name:  "a batch entity fetch with no items",
			fetch: &resolve.BatchEntityFetch{Info: productEntityInfo()},
			info:  productEntityInfo(),
		},
		{
			name:  "nil info",
			fetch: entityFetchWith(&resolve.Object{}),
			info:  nil,
		},
		{
			name:  "info without root fields",
			fetch: entityFetchWith(&resolve.Object{}),
			info:  &resolve.FetchInfo{DataSourceID: "products"},
		},
		{
			name:  "nil fetch",
			fetch: nil,
			info:  productEntityInfo(),
		},
	}
	for _, row := range noSpecRows {
		t.Run(row.name+" means no spec", func(t *testing.T) {
			spec, ok := buildEntitySpec(row.fetch, row.info)
			assert.False(t, ok)
			assert.Equal(t, resolve.CacheKeySpec{}, spec)
		})
	}
}

func TestCacheKeyBuilderRootFieldSpec(t *testing.T) {
	t.Run("scope plus the first root-field coordinate", func(t *testing.T) {
		spec, ok := buildRootFieldSpec(&resolve.FetchInfo{
			DataSourceID: "products",
			RootFields:   []resolve.GraphCoordinate{{TypeName: "Query", FieldName: "products"}},
		})
		require.True(t, ok)
		assert.Equal(t, resolve.CacheKeySpec{
			Scope:     resolve.CacheScopeRootField,
			TypeName:  "Query",
			FieldName: "products",
		}, spec)
	})

	t.Run("nil info and missing root fields mean no spec", func(t *testing.T) {
		_, ok := buildRootFieldSpec(nil)
		assert.False(t, ok)
		_, ok = buildRootFieldSpec(&resolve.FetchInfo{DataSourceID: "products"})
		assert.False(t, ok)
	})
}
