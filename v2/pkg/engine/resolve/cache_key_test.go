package resolve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// TestCachingRenderRootQueryCacheKeyTemplate verifies root field cache key
// rendering with various argument types (none, single, multiple, boolean,
// string, prefix). Incorrect keys would cause cache misses or cross-query
// collisions.
func TestCachingRenderRootQueryCacheKeyTemplate(t *testing.T) {
	t.Run("single field no arguments", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "users"},
					ResponseKey: "users",
					Args:        []FieldArgument{},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Query","field":"users"}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single field single argument", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "droid"},
					ResponseKey: "droid",
					Args: []FieldArgument{
						{
							Name: "id",
							Variable: &ContextVariable{
								Path:     []string{"id"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":1}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Query","field":"droid","args":{"id":1}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single field single string argument", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []FieldArgument{
						{
							Name: "name",
							Variable: &ContextVariable{
								Path:     []string{"name"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"name":"john"}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Query","field":"user","args":{"name":"john"}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single field multiple arguments", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "search"},
					ResponseKey: "search",
					Args: []FieldArgument{
						{
							Name: "term",
							Variable: &ContextVariable{
								Path:     []string{"term"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
						{
							Name: "max",
							Variable: &ContextVariable{
								Path:     []string{"max"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"term":"C3PO","max":10}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Query","field":"search","args":{"term":"C3PO","max":10}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single field multiple arguments with boolean", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "products"},
					ResponseKey: "products",
					Args: []FieldArgument{
						{
							Name: "includeDeleted",
							Variable: &ContextVariable{
								Path:     []string{"includeDeleted"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
						{
							Name: "limit",
							Variable: &ContextVariable{
								Path:     []string{"limit"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"includeDeleted":true,"limit":20}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Query","field":"products","args":{"includeDeleted":true,"limit":20}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("multiple fields single argument each", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "droid"},
					ResponseKey: "droid",
					Args: []FieldArgument{
						{
							Name: "id",
							Variable: &ContextVariable{
								Path:     []string{"id"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []FieldArgument{
						{
							Name: "name",
							Variable: &ContextVariable{
								Path:     []string{"name"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":1,"name":"john"}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)

		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{
					`{"__typename":"Query","field":"droid","args":{"id":1}}`,
					`{"__typename":"Query","field":"user","args":{"name":"john"}}`,
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("multiple fields with mixed arguments", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "product"},
					ResponseKey: "product",
					Args: []FieldArgument{
						{
							Name: "id",
							Variable: &ContextVariable{
								Path:     []string{"id"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
						{
							Name: "includeReviews",
							Variable: &ContextVariable{
								Path:     []string{"includeReviews"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "hero"},
					ResponseKey: "hero",
					Args:        []FieldArgument{},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":"123","includeReviews":true}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)

		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{
					`{"__typename":"Query","field":"product","args":{"id":"123","includeReviews":true}}`,
					`{"__typename":"Query","field":"hero"}`,
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("field with object variable argument", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "search"},
					ResponseKey: "search",
					Args: []FieldArgument{
						{
							Name: "filter",
							Variable: &ObjectVariable{
								Path:     []string{"filter"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{"filter":{"category":"electronics","price":100}}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Query","field":"search","args":{"filter":{"category":"electronics","price":100}}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("field with null argument", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []FieldArgument{
						{
							Name: "id",
							Variable: &ContextVariable{
								Path:     []string{"id"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":null}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Query","field":"user","args":{"id":null}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("field with missing argument", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []FieldArgument{
						{
							Name: "id",
							Variable: &ContextVariable{
								Path:     []string{"id"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Query","field":"user","args":{"id":null}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("field with array argument", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "products"},
					ResponseKey: "products",
					Args: []FieldArgument{
						{
							Name: "ids",
							Variable: &ContextVariable{
								Path:     []string{"ids"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"ids":[1,2,3]}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Query","field":"products","args":{"ids":[1,2,3]}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("non-Query type", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Subscription", FieldName: "messageAdded"},
					ResponseKey: "messageAdded",
					Args: []FieldArgument{
						{
							Name: "roomId",
							Variable: &ContextVariable{
								Path:     []string{"roomId"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"roomId":"123"}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Subscription","field":"messageAdded","args":{"roomId":"123"}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single field with arena", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []FieldArgument{
						{
							Name: "name",
							Variable: &ContextVariable{
								Path:     []string{"name"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := &Context{
			Variables: astjson.MustParse(`{"name":"john"}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(ar, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Query","field":"user","args":{"name":"john"}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single field with prefix", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []FieldArgument{
						{
							Name: "id",
							Variable: &ContextVariable{
								Path:     []string{"id"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":1}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "prefix")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`prefix:{"__typename":"Query","field":"user","args":{"id":1}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("multiple fields with prefix", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "droid"},
					ResponseKey: "droid",
					Args: []FieldArgument{
						{
							Name: "id",
							Variable: &ContextVariable{
								Path:     []string{"id"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []FieldArgument{
						{
							Name: "name",
							Variable: &ContextVariable{
								Path:     []string{"name"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":1,"name":"john"}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "my-prefix")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{
					`my-prefix:{"__typename":"Query","field":"droid","args":{"id":1}}`,
					`my-prefix:{"__typename":"Query","field":"user","args":{"name":"john"}}`,
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})
}

// TestCachingRenderEntityQueryCacheKeyTemplate verifies entity cache key
// rendering from __typename + @key fields. Covers single entities, batches,
// composite keys, and nested key fields.
func TestCachingRenderEntityQueryCacheKeyTemplate(t *testing.T) {
	t.Run("single entity with typename and id", func(t *testing.T) {
		tmpl := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{
						Name: []byte("__typename"),
						Value: &String{
							Path: []string{"__typename"},
						},
					},
					{
						Name: []byte("id"),
						Value: &String{
							Path: []string{"id"},
						},
					},
				},
			}),
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{"__typename":"Product","id":"123"}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Product","key":{"id":"123"}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single entity with multiple keys", func(t *testing.T) {
		tmpl := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{
						Name: []byte("__typename"),
						Value: &String{
							Path: []string{"__typename"},
						},
					},
					{
						Name: []byte("sku"),
						Value: &String{
							Path: []string{"sku"},
						},
					},
					{
						Name: []byte("upc"),
						Value: &String{
							Path: []string{"upc"},
						},
					},
				},
			}),
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{"__typename":"Product","sku":"ABC123","upc":"DEF456","name":"Trilby"}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Product","key":{"sku":"ABC123","upc":"DEF456"}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single entity with prefix", func(t *testing.T) {
		tmpl := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{
						Name: []byte("__typename"),
						Value: &String{
							Path: []string{"__typename"},
						},
					},
					{
						Name: []byte("id"),
						Value: &String{
							Path: []string{"id"},
						},
					},
				},
			}),
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{"__typename":"Product","id":"123"}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "entity-prefix")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`entity-prefix:{"__typename":"Product","key":{"id":"123"}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("entity with multiple keys and prefix", func(t *testing.T) {
		tmpl := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{
						Name: []byte("__typename"),
						Value: &String{
							Path: []string{"__typename"},
						},
					},
					{
						Name: []byte("sku"),
						Value: &String{
							Path: []string{"sku"},
						},
					},
					{
						Name: []byte("upc"),
						Value: &String{
							Path: []string{"upc"},
						},
					},
				},
			}),
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{"__typename":"Product","sku":"ABC123","upc":"DEF456","name":"Trilby"}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "cache")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`cache:{"__typename":"Product","key":{"sku":"ABC123","upc":"DEF456"}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("entity with array key field", func(t *testing.T) {
		// Test that arrays in entity keys are properly resolved
		tmpl := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{
						Name: []byte("__typename"),
						Value: &String{
							Path: []string{"__typename"},
						},
					},
					{
						Name: []byte("tags"),
						Value: &Array{
							Path: []string{"tags"},
							Item: &String{},
						},
					},
				},
			}),
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{"__typename":"Product","tags":["electronics","sale"]}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []string{`{"__typename":"Product","key":{"tags":["electronics","sale"]}}`},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})
}

// TestDerivedEntityCacheKey verifies EntityKeyMappings-based cache key
// derivation for root field queries. These keys allow L2 cache lookups
// by entity identity (e.g., User by id) for root field responses.
func TestDerivedEntityCacheKey(t *testing.T) {
	t.Run("simple string ID", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []FieldArgument{
						{Name: "id", Variable: &ContextVariable{Path: []string{"id"}, Renderer: NewCacheKeyVariableRenderer()}},
					},
				},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"id":"123"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{`{"__typename":"User","key":{"id":"123"}}`}, cacheKeys[0].Keys)
	})

	t.Run("integer argument", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []FieldArgument{
						{Name: "id", Variable: &ContextVariable{Path: []string{"id"}, Renderer: NewCacheKeyVariableRenderer()}},
					},
				},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"id":42}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		// Numbers are coerced to strings in entity cache keys for consistent matching
		// between read path (request args) and write path (response entity data)
		assert.Equal(t, []string{`{"__typename":"User","key":{"id":"42"}}`}, cacheKeys[0].Keys)
	})

	t.Run("number to string coercion in entity cache keys", func(t *testing.T) {
		makeTmpl := func() *RootQueryCacheKeyTemplate {
			return &RootQueryCacheKeyTemplate{
				RootFields: []QueryField{
					{
						Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
						ResponseKey: "user",
						Args: []FieldArgument{
							{Name: "id", Variable: &ContextVariable{Path: []string{"id"}, Renderer: NewCacheKeyVariableRenderer()}},
						},
					},
				},
				EntityKeyMappings: []EntityKeyMappingConfig{
					{
						EntityTypeName: "User",
						FieldMappings: []EntityFieldMappingConfig{
							{EntityKeyField: "id", ArgumentPath: []string{"id"}},
						},
					},
				},
			}
		}

		tests := []struct {
			name      string
			variables string
			wantKey   string
		}{
			{
				name:      "integer coerced to string",
				variables: `{"id":1}`,
				wantKey:   `{"__typename":"User","key":{"id":"1"}}`,
			},
			{
				name:      "float with decimal coerced to string",
				variables: `{"id":1.5}`,
				wantKey:   `{"__typename":"User","key":{"id":"1.5"}}`,
			},
			{
				name:      "float whole number coerced to string",
				variables: `{"id":1.0}`,
				wantKey:   `{"__typename":"User","key":{"id":"1.0"}}`,
			},
			{
				name:      "large integer coerced to string",
				variables: `{"id":9999999}`,
				wantKey:   `{"__typename":"User","key":{"id":"9999999"}}`,
			},
			{
				name:      "string stays string",
				variables: `{"id":"1"}`,
				wantKey:   `{"__typename":"User","key":{"id":"1"}}`,
			},
			{
				name:      "integer and string produce same key",
				variables: `{"id":42}`,
				wantKey:   `{"__typename":"User","key":{"id":"42"}}`,
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tmpl := makeTmpl()
				ctx := &Context{Variables: astjson.MustParse(tt.variables), ctx: context.Background()}
				data := astjson.MustParse(`{}`)
				cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
				assert.NoError(t, err)
				assert.Equal(t, 1, len(cacheKeys))
				assert.Equal(t, []string{tt.wantKey}, cacheKeys[0].Keys)
			})
		}

		// Verify integer and string inputs produce identical cache keys
		t.Run("integer and string inputs match", func(t *testing.T) {
			tmpl1 := makeTmpl()
			ctx1 := &Context{Variables: astjson.MustParse(`{"id":1}`), ctx: context.Background()}
			keys1, err := tmpl1.RenderCacheKeys(nil, ctx1, []*astjson.Value{astjson.MustParse(`{}`)}, "")
			assert.NoError(t, err)

			tmpl2 := makeTmpl()
			ctx2 := &Context{Variables: astjson.MustParse(`{"id":"1"}`), ctx: context.Background()}
			keys2, err := tmpl2.RenderCacheKeys(nil, ctx2, []*astjson.Value{astjson.MustParse(`{}`)}, "")
			assert.NoError(t, err)

			assert.Equal(t, keys1[0].Keys, keys2[0].Keys)
		})
	})

	t.Run("nested object path", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "user"},
					ResponseKey: "user",
					Args: []FieldArgument{
						{Name: "input", Variable: &ContextVariable{Path: []string{"input"}, Renderer: NewCacheKeyVariableRenderer()}},
					},
				},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"input", "userId"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"input":{"userId":"456"}}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{`{"__typename":"User","key":{"id":"456"}}`}, cacheKeys[0].Keys)
	})

	t.Run("deep nested path", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "thing"}, ResponseKey: "thing"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "X",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"a", "b", "c"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"a":{"b":{"c":"deep"}}}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{`{"__typename":"X","key":{"id":"deep"}}`}, cacheKeys[0].Keys)
	})

	t.Run("array index path", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"}, ResponseKey: "user"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"ids", "0"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"ids":["first","second"]}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{`{"__typename":"User","key":{"id":"first"}}`}, cacheKeys[0].Keys)
	})

	t.Run("array index path - empty array", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"}, ResponseKey: "user"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"ids", "0"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"ids":[]}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		// Empty array has no index 0 → skip caching
		assert.Equal(t, 0, len(cacheKeys[0].Keys))
	})

	t.Run("array index path - null variable", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"}, ResponseKey: "user"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"ids", "0"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"ids":null}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		// Null variable → skip caching
		assert.Equal(t, 0, len(cacheKeys[0].Keys))
	})

	t.Run("multiple key fields", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "orgUser"}, ResponseKey: "orgUser"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "OrgUser",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "orgId", ArgumentPath: []string{"orgId"}},
						{EntityKeyField: "userId", ArgumentPath: []string{"userId"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"orgId":"org1","userId":"u1"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{`{"__typename":"OrgUser","key":{"orgId":"org1","userId":"u1"}}`}, cacheKeys[0].Keys)
	})

	t.Run("with prefix", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"}, ResponseKey: "user"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"id":"123"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "12345")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{`12345:{"__typename":"User","key":{"id":"123"}}`}, cacheKeys[0].Keys)
	})

	t.Run("missing variable - skip caching", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"}, ResponseKey: "user"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"nonexistent"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		// No keys generated (empty) because variable is missing
		assert.Equal(t, 0, len(cacheKeys[0].Keys))
	})

	t.Run("null variable - skip caching", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"}, ResponseKey: "user"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"id":null}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		// No keys generated because variable is null
		assert.Equal(t, 0, len(cacheKeys[0].Keys))
	})

	t.Run("variable remapping", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"}, ResponseKey: "user"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
			},
		}

		ctx := &Context{
			Variables:      astjson.MustParse(`{"userId":"123"}`),
			RemapVariables: map[string]string{"id": "userId"},
			ctx:            context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{`{"__typename":"User","key":{"id":"123"}}`}, cacheKeys[0].Keys)
	})

	t.Run("dot-notation entity key field", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productByStore"}, ResponseKey: "productByStore"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "store.id", ArgumentPath: []string{"storeId"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"storeId":"123"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{`{"__typename":"Product","key":{"store":{"id":"123"}}}`}, cacheKeys[0].Keys)
	})

	t.Run("deeply nested dot-notation entity key field", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "thing"}, ResponseKey: "thing"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Thing",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "owner.company.id", ArgumentPath: []string{"companyId"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"companyId":"abc"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{`{"__typename":"Thing","key":{"owner":{"company":{"id":"abc"}}}}`}, cacheKeys[0].Keys)
	})

	t.Run("dot-notation shared prefix merges into same object", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "product"}, ResponseKey: "product"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "store.id", ArgumentPath: []string{"storeId"}},
						{EntityKeyField: "store.region", ArgumentPath: []string{"region"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"storeId":"s1","region":"us"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		// Both store.id and store.region must appear under the same "store" object
		assert.Equal(t, []string{`{"__typename":"Product","key":{"store":{"id":"s1","region":"us"}}}`}, cacheKeys[0].Keys)
	})

	t.Run("multiple entity key mappings - multi-key lookup", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "product"}, ResponseKey: "product"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "sku", ArgumentPath: []string{"sku"}},
						{EntityKeyField: "region", ArgumentPath: []string{"region"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"id":"123","sku":"abc","region":"us"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"id":"123"}}`,
			`{"__typename":"Product","key":{"sku":"abc","region":"us"}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("multiple entity key mappings - partial missing skips that key only", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "product"}, ResponseKey: "product"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "sku", ArgumentPath: []string{"sku"}},
						{EntityKeyField: "region", ArgumentPath: []string{"region"}},
					},
				},
			},
		}

		// Only id and sku provided, region missing → second mapping skipped
		ctx := &Context{Variables: astjson.MustParse(`{"id":"123","sku":"abc"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"id":"123"}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("flat key + composite key - all args present", func(t *testing.T) {
		// Flat @key(fields: "id") + composite @key(fields: "sku region").
		// All arguments provided → both mappings resolve → two cache keys.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productByAll"}, ResponseKey: "productByAll"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "sku", ArgumentPath: []string{"sku"}},
						{EntityKeyField: "region", ArgumentPath: []string{"region"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"id":"p1","sku":"ABC","region":"us-east"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"id":"p1"}}`,
			`{"__typename":"Product","key":{"sku":"ABC","region":"us-east"}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("flat key + composite key - only composite args present", func(t *testing.T) {
		// Flat @key(fields: "id") + composite @key(fields: "sku region").
		// Only sku and region provided, id missing → flat mapping skipped → one cache key.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productBySku"}, ResponseKey: "productBySku"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "sku", ArgumentPath: []string{"sku"}},
						{EntityKeyField: "region", ArgumentPath: []string{"region"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"sku":"ABC","region":"us-east"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"sku":"ABC","region":"us-east"}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("flat key + nested composite key - all args present", func(t *testing.T) {
		// Flat @key(fields: "id") + nested @key(fields: "store { id region }").
		// All arguments provided → both mappings resolve → two cache keys,
		// the second with nested JSON structure from dot-notation.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productByAll"}, ResponseKey: "productByAll"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "store.id", ArgumentPath: []string{"storeId"}},
						{EntityKeyField: "store.region", ArgumentPath: []string{"storeRegion"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"id":"p1","storeId":"s1","storeRegion":"us"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"id":"p1"}}`,
			`{"__typename":"Product","key":{"store":{"id":"s1","region":"us"}}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("flat key + nested composite key - only nested args present", func(t *testing.T) {
		// Flat @key(fields: "id") + nested @key(fields: "store { id region }").
		// Only storeId and storeRegion provided, id missing → flat mapping skipped.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productByStore"}, ResponseKey: "productByStore"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "store.id", ArgumentPath: []string{"storeId"}},
						{EntityKeyField: "store.region", ArgumentPath: []string{"storeRegion"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"storeId":"s1","storeRegion":"us"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"store":{"id":"s1","region":"us"}}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("nested composite key - structured argument input", func(t *testing.T) {
		// Nested @key(fields: "store { id region }") with a structured argument:
		// query productByStore(store: {id: "s1", region: "us"})
		// ArgumentPath ["store", "id"] navigates into the structured variable
		// to extract the value for entity key field "store.id".
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productByStore"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "store.id", ArgumentPath: []string{"store", "id"}},
						{EntityKeyField: "store.region", ArgumentPath: []string{"store", "region"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"store":{"id":"s1","region":"us"}}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"store":{"id":"s1","region":"us"}}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("flat key + nested composite key with structured arg - only nested resolves", func(t *testing.T) {
		// Flat @key(fields: "id") + nested @key(fields: "store { id region }").
		// Argument "store" is a structured input object, "id" is a flat argument.
		// Only "store" provided → flat mapping skipped → one nested cache key.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productByStore"}, ResponseKey: "productByStore"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "store.id", ArgumentPath: []string{"store", "id"}},
						{EntityKeyField: "store.region", ArgumentPath: []string{"store", "region"}},
					},
				},
			},
		}

		// Only structured store argument provided, no flat id
		ctx := &Context{Variables: astjson.MustParse(`{"store":{"id":"s1","region":"us"}}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"store":{"id":"s1","region":"us"}}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("two nested composite keys with structured args - both resolve", func(t *testing.T) {
		// Two nested keys: @key(fields: "store { id }") + @key(fields: "location { city country }").
		// Arguments are structured input objects: store: {id: "s1"}, location: {city: "Berlin", country: "DE"}.
		// Both resolve → two nested cache keys.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "warehouse"}, ResponseKey: "warehouse"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Warehouse",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "store.id", ArgumentPath: []string{"store", "id"}},
					},
				},
				{
					EntityTypeName: "Warehouse",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "location.city", ArgumentPath: []string{"location", "city"}},
						{EntityKeyField: "location.country", ArgumentPath: []string{"location", "country"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"store":{"id":"s1"},"location":{"city":"Berlin","country":"DE"}}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Warehouse","key":{"store":{"id":"s1"}}}`,
			`{"__typename":"Warehouse","key":{"location":{"city":"Berlin","country":"DE"}}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("two nested composite keys with structured args - only first resolves", func(t *testing.T) {
		// Two nested keys: @key(fields: "store { id }") + @key(fields: "location { city country }").
		// Arguments are structured: store: {id: "s1"}, but no location argument.
		// Only store resolves → location mapping skipped → one cache key.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "warehouse"}, ResponseKey: "warehouse"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Warehouse",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "store.id", ArgumentPath: []string{"store", "id"}},
					},
				},
				{
					EntityTypeName: "Warehouse",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "location.city", ArgumentPath: []string{"location", "city"}},
						{EntityKeyField: "location.country", ArgumentPath: []string{"location", "country"}},
					},
				},
			},
		}

		// Only store argument provided — location missing → second mapping skipped
		ctx := &Context{Variables: astjson.MustParse(`{"store":{"id":"s1"}}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Warehouse","key":{"store":{"id":"s1"}}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("remap variables - flat key forward lookup", func(t *testing.T) {
		// Production scenario: VariablesMapper renames $id → $a in the AST.
		// resolveArgumentPath resolves "id" → ContextVariable.Path ["a"].
		// RemapVariables maps newName → oldName: {"a": "id"}.
		// Variables JSON keeps the original name: {"id": "user-123"}.
		// Forward lookup: RemapVariables["a"] = "id" → Variables.Get("id") = "user-123".
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"}, ResponseKey: "user"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"a"}},
					},
				},
			},
		}

		ctx := &Context{
			Variables:      astjson.MustParse(`{"id":"user-123"}`),
			RemapVariables: map[string]string{"a": "id"},
			ctx:            context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"User","key":{"id":"user-123"}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("remap variables - multiple mappings forward lookup", func(t *testing.T) {
		// Two mappings: flat @key(fields: "id") + composite @key(fields: "sku region").
		// VariablesMapper renamed $id→$a, $sku→$b, $region→$c.
		// resolveArgumentPath resolved each to ["a"], ["b"], ["c"].
		// Variables JSON keeps original names: {"id", "sku", "region"}.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productByAll"}, ResponseKey: "productByAll"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"a"}},
					},
				},
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "sku", ArgumentPath: []string{"b"}},
						{EntityKeyField: "region", ArgumentPath: []string{"c"}},
					},
				},
			},
		}

		ctx := &Context{
			Variables:      astjson.MustParse(`{"id":"p1","sku":"ABC","region":"us-east"}`),
			RemapVariables: map[string]string{"a": "id", "b": "sku", "c": "region"},
			ctx:            context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"id":"p1"}}`,
			`{"__typename":"Product","key":{"sku":"ABC","region":"us-east"}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("remap variables - partial remap with multi-key", func(t *testing.T) {
		// Two entity key mappings: flat "id" (remapped $id→$a) + flat "username" (derived key, no argument).
		// ArgumentPath ["a"] resolved by planner; ArgumentPath ["username"] unresolved (derived key).
		// Only the "id" mapping resolves; "username" has no variable → skip that mapping.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"}, ResponseKey: "user"},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"a"}},
					},
				},
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "username", ArgumentPath: []string{"username"}},
					},
				},
			},
		}

		ctx := &Context{
			Variables:      astjson.MustParse(`{"id":"user-123"}`),
			RemapVariables: map[string]string{"a": "id"},
			ctx:            context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		// Only the "id" mapping resolves; "username" is a derived key with no variable
		assert.Equal(t, []string{
			`{"__typename":"User","key":{"id":"user-123"}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("remap variables - nested input object argument path", func(t *testing.T) {
		// Multi-element ArgumentPath ["a", "sellerId"] with RemapVariables {"a": "k"}
		// should remap the first element "a" → "k" and resolve from {"k": {"sellerId": "s1", "sku": "WIDGET-01"}}.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productBySeller"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "sellerId", ArgumentPath: []string{"a", "sellerId"}},
						{EntityKeyField: "sku", ArgumentPath: []string{"a", "sku"}},
					},
				},
			},
		}

		ctx := &Context{
			Variables:      astjson.MustParse(`{"k":{"sellerId":"s1","sku":"WIDGET-01"}}`),
			RemapVariables: map[string]string{"a": "k"},
			ctx:            context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Product","key":{"sellerId":"s1","sku":"WIDGET-01"}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("remap variables - deeply nested input object argument path", func(t *testing.T) {
		// 3-element ArgumentPath ["a", "address", "id"] with RemapVariables {"a": "v"}
		// should remap first element "a" → "v" and resolve from {"v": {"address": {"id": "v1"}}}.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "venue"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Venue",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "address.id", ArgumentPath: []string{"a", "address", "id"}},
					},
				},
			},
		}

		ctx := &Context{
			Variables:      astjson.MustParse(`{"v":{"address":{"id":"v1"}}}`),
			RemapVariables: map[string]string{"a": "v"},
			ctx:            context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{
			`{"__typename":"Venue","key":{"address":{"id":"v1"}}}`,
		}, cacheKeys[0].Keys)
	})

	t.Run("flat key + composite key - neither matches (skip cache)", func(t *testing.T) {
		// Flat @key(fields: "id") + composite @key(fields: "sku region").
		// No arguments provided → both mappings skip → empty keys → skip cache.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productByAll"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "sku", ArgumentPath: []string{"sku"}},
						{EntityKeyField: "region", ArgumentPath: []string{"region"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"unrelated":"value"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{}, cacheKeys[0].Keys)
	})

	t.Run("flat key + nested composite key - neither matches (skip cache)", func(t *testing.T) {
		// Flat @key(fields: "id") + nested @key(fields: "store { id region }").
		// No arguments provided → both mappings skip → empty keys → skip cache.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productByAll"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "store.id", ArgumentPath: []string{"storeId"}},
						{EntityKeyField: "store.region", ArgumentPath: []string{"storeRegion"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"unrelated":"value"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{}, cacheKeys[0].Keys)
	})

	t.Run("flat key + nested composite key with structured arg - neither matches (skip cache)", func(t *testing.T) {
		// Flat @key(fields: "id") + nested @key(fields: "store { id region }") with structured arg.
		// No arguments provided → both mappings skip → empty keys → skip cache.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "productByStore"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"id"}},
					},
				},
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "store.id", ArgumentPath: []string{"store", "id"}},
						{EntityKeyField: "store.region", ArgumentPath: []string{"store", "region"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"unrelated":"value"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{}, cacheKeys[0].Keys)
	})

	t.Run("two nested composite keys with structured args - neither matches (skip cache)", func(t *testing.T) {
		// Two nested keys: @key(fields: "store { id }") + @key(fields: "location { city country }").
		// No arguments provided → both mappings skip → empty keys → skip cache.
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "warehouse"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Warehouse",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "store.id", ArgumentPath: []string{"store", "id"}},
					},
				},
				{
					EntityTypeName: "Warehouse",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "location.city", ArgumentPath: []string{"location", "city"}},
						{EntityKeyField: "location.country", ArgumentPath: []string{"location", "country"}},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"unrelated":"value"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{}, cacheKeys[0].Keys)
	})

	t.Run("no entity key mapping - uses root field key", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "user"},
					Args: []FieldArgument{
						{Name: "id", Variable: &ContextVariable{Path: []string{"id"}, Renderer: NewCacheKeyVariableRenderer()}},
					},
				},
			},
			// No EntityKeyMappings - should use root field key format
		}

		ctx := &Context{Variables: astjson.MustParse(`{"id":"123"}`), ctx: context.Background()}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data}, "")
		assert.NoError(t, err)
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{`{"__typename":"Query","field":"user","args":{"id":"123"}}`}, cacheKeys[0].Keys)
	})
}

func BenchmarkRenderCacheKeys(b *testing.B) {
	a := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

	ctxRootQuery := &Context{
		Variables: astjson.MustParse(`{"id":1,"name":"john","term":"C3PO","max":10}`),
		ctx:       context.Background(),
	}

	ctxEntityQuery := &Context{
		Variables: astjson.MustParse(`{}`),
		ctx:       context.Background(),
	}

	b.Run("RootQuery/SingleField", func(b *testing.B) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate: GraphCoordinate{
						TypeName:  "Query",
						FieldName: "user",
					},
					Args: []FieldArgument{
						{
							Name: "id",
							Variable: &ContextVariable{
								Path:     []string{"id"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		data := astjson.MustParse(`{}`)
		items := []*astjson.Value{data}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			a.Reset()
			_, err := tmpl.RenderCacheKeys(a, ctxRootQuery, items, "")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("RootQuery/MultipleFields", func(b *testing.B) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate: GraphCoordinate{
						TypeName:  "Query",
						FieldName: "droid",
					},
					Args: []FieldArgument{
						{
							Name: "id",
							Variable: &ContextVariable{
								Path:     []string{"id"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
				{
					Coordinate: GraphCoordinate{
						TypeName:  "Query",
						FieldName: "user",
					},
					Args: []FieldArgument{
						{
							Name: "name",
							Variable: &ContextVariable{
								Path:     []string{"name"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
				{
					Coordinate: GraphCoordinate{
						TypeName:  "Query",
						FieldName: "search",
					},
					Args: []FieldArgument{
						{
							Name: "term",
							Variable: &ContextVariable{
								Path:     []string{"term"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
						{
							Name: "max",
							Variable: &ContextVariable{
								Path:     []string{"max"},
								Renderer: NewCacheKeyVariableRenderer(),
							},
						},
					},
				},
			},
		}

		data := astjson.MustParse(`{}`)
		items := []*astjson.Value{data}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			a.Reset()
			_, err := tmpl.RenderCacheKeys(a, ctxRootQuery, items, "")
			if err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("EntityQuery", func(b *testing.B) {
		tmpl := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{
						Name: []byte("__typename"),
						Value: &String{
							Path: []string{"__typename"},
						},
					},
					{
						Name: []byte("id"),
						Value: &String{
							Path: []string{"id"},
						},
					},
					{
						Name: []byte("sku"),
						Value: &String{
							Path: []string{"sku"},
						},
					},
					{
						Name: []byte("upc"),
						Value: &String{
							Path: []string{"upc"},
						},
					},
				},
			}),
		}

		data1 := astjson.MustParse(`{"__typename":"Product","id":"123","sku":"ABC123","upc":"DEF456","name":"Trilby"}`)
		data2 := astjson.MustParse(`{"__typename":"Product","id":"456","sku":"XYZ789","upc":"GHI012","name":"Fedora"}`)
		data3 := astjson.MustParse(`{"__typename":"Product","id":"789","sku":"JKL345","upc":"MNO678","name":"Boater"}`)
		items := []*astjson.Value{data1, data2, data3}

		b.ReportAllocs()
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			a.Reset()
			_, err := tmpl.RenderCacheKeys(a, ctxEntityQuery, items, "")
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}

// TestRenderCacheKeys_EntityKeyMappings_NotDuplicatedByRootFields verifies
// that EntityKeyMappings produce exactly one key per entity, not duplicated
// per root field in multi-field queries.
func TestRenderCacheKeys_EntityKeyMappings_NotDuplicatedByRootFields(t *testing.T) {
	a := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

	template := &RootQueryCacheKeyTemplate{
		RootFields: []QueryField{
			{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "field1"}},
			{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "field2"}},
		},
		EntityKeyMappings: []EntityKeyMappingConfig{
			{
				EntityTypeName: "Product",
				FieldMappings: []EntityFieldMappingConfig{
					{EntityKeyField: "upc", ArgumentPath: []string{"upc"}},
				},
			},
		},
	}

	ctx := NewContext(context.Background())
	ctx.Variables = astjson.MustParse(`{"upc":"top-1"}`)

	items := []*astjson.Value{astjson.NullValue}
	keys, err := template.RenderCacheKeys(a, ctx, items, "")
	require.NoError(t, err)
	require.Len(t, keys, 1, "one CacheKey per item")
	// Should have exactly 1 key string, not 2 (one per root field)
	require.Equal(t, []string{
		`{"__typename":"Product","key":{"upc":"top-1"}}`,
	}, keys[0].Keys, "EntityKeyMappings should produce one key, not duplicated per root field")
}

// TestResolveFieldValue verifies that resolveFieldValue extracts arena-allocated
// values from JSON data for each node type (String, Scalar, Integer, etc.).
func TestResolveFieldValue(t *testing.T) {
	a := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	template := &EntityQueryCacheKeyTemplate{}

	t.Run("String", func(t *testing.T) {
		data := astjson.MustParse(`{"name":"Alice"}`)
		result := template.resolveFieldValue(a, &String{Path: []string{"name"}}, data)
		require.NotNil(t, result)
		assert.Equal(t, `"Alice"`, string(result.MarshalTo(nil)))
	})

	t.Run("Scalar", func(t *testing.T) {
		data := astjson.MustParse(`{"id":"abc-123"}`)
		result := template.resolveFieldValue(a, &Scalar{Path: []string{"id"}}, data)
		require.NotNil(t, result)
		assert.Equal(t, `"abc-123"`, string(result.MarshalTo(nil)))
	})

	t.Run("Integer", func(t *testing.T) {
		data := astjson.MustParse(`{"age":42}`)
		result := template.resolveFieldValue(a, &Integer{Path: []string{"age"}}, data)
		require.NotNil(t, result)
		assert.Equal(t, `42`, string(result.MarshalTo(nil)))
	})

	t.Run("Float", func(t *testing.T) {
		data := astjson.MustParse(`{"price":19.99}`)
		result := template.resolveFieldValue(a, &Float{Path: []string{"price"}}, data)
		require.NotNil(t, result)
		assert.Equal(t, `19.99`, string(result.MarshalTo(nil)))
	})

	t.Run("Boolean", func(t *testing.T) {
		data := astjson.MustParse(`{"active":true}`)
		result := template.resolveFieldValue(a, &Boolean{Path: []string{"active"}}, data)
		require.NotNil(t, result)
		assert.Equal(t, `true`, string(result.MarshalTo(nil)))
	})

	t.Run("Enum", func(t *testing.T) {
		data := astjson.MustParse(`{"status":"ACTIVE"}`)
		result := template.resolveFieldValue(a, &Enum{Path: []string{"status"}}, data)
		require.NotNil(t, result)
		assert.Equal(t, `"ACTIVE"`, string(result.MarshalTo(nil)))
	})

	t.Run("BigInt", func(t *testing.T) {
		data := astjson.MustParse(`{"bigId":"9007199254740993"}`)
		result := template.resolveFieldValue(a, &BigInt{Path: []string{"bigId"}}, data)
		require.NotNil(t, result)
		assert.Equal(t, `"9007199254740993"`, string(result.MarshalTo(nil)))
	})

	t.Run("CustomNode", func(t *testing.T) {
		data := astjson.MustParse(`{"custom":"some-value"}`)
		result := template.resolveFieldValue(a, &CustomNode{Path: []string{"custom"}}, data)
		require.NotNil(t, result)
		assert.Equal(t, `"some-value"`, string(result.MarshalTo(nil)))
	})

	t.Run("Object", func(t *testing.T) {
		data := astjson.MustParse(`{"address":{"city":"Berlin","zip":"10115"}}`)
		node := &Object{
			Path: []string{"address"},
			Fields: []*Field{
				{Name: []byte("city"), Value: &String{Path: []string{"city"}}},
				{Name: []byte("zip"), Value: &String{Path: []string{"zip"}}},
			},
		}
		result := template.resolveFieldValue(a, node, data)
		require.NotNil(t, result)
		assert.Equal(t, `{"city":"Berlin","zip":"10115"}`, string(result.MarshalTo(nil)))
	})

	t.Run("Object skips __typename", func(t *testing.T) {
		data := astjson.MustParse(`{"address":{"__typename":"Address","city":"Berlin"}}`)
		node := &Object{
			Path: []string{"address"},
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				{Name: []byte("city"), Value: &String{Path: []string{"city"}}},
			},
		}
		result := template.resolveFieldValue(a, node, data)
		require.NotNil(t, result)
		assert.Equal(t, `{"city":"Berlin"}`, string(result.MarshalTo(nil)))
	})

	t.Run("Object returns nil for null data", func(t *testing.T) {
		data := astjson.MustParse(`{"address":null}`)
		node := &Object{
			Path: []string{"address"},
			Fields: []*Field{
				{Name: []byte("city"), Value: &String{Path: []string{"city"}}},
			},
		}
		result := template.resolveFieldValue(a, node, data)
		assert.Nil(t, result)
	})

	t.Run("Array", func(t *testing.T) {
		data := astjson.MustParse(`{"tags":["go","graphql"]}`)
		node := &Array{
			Path: []string{"tags"},
			Item: &String{},
		}
		result := template.resolveFieldValue(a, node, data)
		require.NotNil(t, result)
		assert.Equal(t, `["go","graphql"]`, string(result.MarshalTo(nil)))
	})

	t.Run("Array returns nil for missing path", func(t *testing.T) {
		data := astjson.MustParse(`{}`)
		node := &Array{
			Path: []string{"tags"},
			Item: &String{},
		}
		result := template.resolveFieldValue(a, node, data)
		assert.Nil(t, result)
	})

	t.Run("missing path returns nil", func(t *testing.T) {
		data := astjson.MustParse(`{}`)
		result := template.resolveFieldValue(a, &String{Path: []string{"missing"}}, data)
		assert.Nil(t, result)
	})

	t.Run("nested path", func(t *testing.T) {
		data := astjson.MustParse(`{"a":{"b":{"c":"deep"}}}`)
		result := template.resolveFieldValue(a, &String{Path: []string{"a", "b", "c"}}, data)
		require.NotNil(t, result)
		assert.Equal(t, `"deep"`, string(result.MarshalTo(nil)))
	})
}

// TestRenderCacheKeys_BatchEntityKey verifies that list arguments in
// EntityKeyMappings expand into multiple cache keys (one per list item),
// enabling per-entity L2 lookups for batch root field queries.
func TestRenderCacheKeys_BatchEntityKey(t *testing.T) {
	t.Run("list argument produces multiple cache keys", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "products"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"upcs":["p1","p2","p3"]}`), ctx: context.Background()}
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{nil}, "")
		assert.NoError(t, err)
		assert.Equal(t, []*CacheKey{
			{Keys: []string{`{"__typename":"Product","key":{"upc":"p1"}}`}, BatchIndex: 0},
			{Keys: []string{`{"__typename":"Product","key":{"upc":"p2"}}`}, BatchIndex: 1},
			{Keys: []string{`{"__typename":"Product","key":{"upc":"p3"}}`}, BatchIndex: 2},
		}, cacheKeys)
	})

	t.Run("empty list produces no cache keys", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "products"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"upcs":[]}`), ctx: context.Background()}
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{nil}, "")
		assert.NoError(t, err)
		assert.Equal(t, 0, len(cacheKeys))
	})

	t.Run("single-element list produces one cache key", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "products"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"upcs":["p1"]}`), ctx: context.Background()}
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{nil}, "")
		assert.NoError(t, err)
		assert.Equal(t, []*CacheKey{
			{Keys: []string{`{"__typename":"Product","key":{"upc":"p1"}}`}, BatchIndex: 0},
		}, cacheKeys)
	})

	t.Run("scalar argument with ArgumentIsEntityKey falls back to single key", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "product"},
					Args: []FieldArgument{
						{Name: "upc", Variable: &ContextVariable{Path: []string{"upc"}, Renderer: NewCacheKeyVariableRenderer()}},
					},
				},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "upc", ArgumentPath: []string{"upc"}, ArgumentIsEntityKey: true},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"upc":"p1"}`), ctx: context.Background()}
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{nil}, "")
		assert.NoError(t, err)
		// Falls back to non-batch path — uses renderDerivedEntityKey, same key format
		assert.Equal(t, 1, len(cacheKeys))
		assert.Equal(t, []string{`{"__typename":"Product","key":{"upc":"p1"}}`}, cacheKeys[0].Keys)
	})

	t.Run("batch key format matches scalar key format", func(t *testing.T) {
		// Scalar lookup
		scalarTmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "product"},
					Args: []FieldArgument{
						{Name: "upc", Variable: &ContextVariable{Path: []string{"upc"}, Renderer: NewCacheKeyVariableRenderer()}},
					},
				},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "upc", ArgumentPath: []string{"upc"}},
					},
				},
			},
		}

		scalarCtx := &Context{Variables: astjson.MustParse(`{"upc":"p1"}`), ctx: context.Background()}
		scalarKeys, err := scalarTmpl.RenderCacheKeys(nil, scalarCtx, []*astjson.Value{nil}, "")
		assert.NoError(t, err)

		// Batch lookup
		batchTmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "products"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
					},
				},
			},
		}

		batchCtx := &Context{Variables: astjson.MustParse(`{"upcs":["p1"]}`), ctx: context.Background()}
		batchKeys, err := batchTmpl.RenderCacheKeys(nil, batchCtx, []*astjson.Value{nil}, "")
		assert.NoError(t, err)

		// Same cache key format — enables cache sharing between scalar and batch lookups
		assert.Equal(t, scalarKeys[0].Keys[0], batchKeys[0].Keys[0])
	})

	t.Run("null argument produces empty cache keys", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "products"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"upcs":null}`), ctx: context.Background()}
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{nil}, "")
		assert.NoError(t, err)
		assert.Equal(t, 0, len(cacheKeys))
	})

	t.Run("list argument with prefix", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "products"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
					},
				},
			},
		}

		ctx := &Context{Variables: astjson.MustParse(`{"upcs":["p1","p2"]}`), ctx: context.Background()}
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{nil}, "12345")
		assert.NoError(t, err)
		assert.Equal(t, []*CacheKey{
			{Keys: []string{`12345:{"__typename":"Product","key":{"upc":"p1"}}`}, BatchIndex: 0},
			{Keys: []string{`12345:{"__typename":"Product","key":{"upc":"p2"}}`}, BatchIndex: 1},
		}, cacheKeys)
	})

	t.Run("list argument with RemapVariables", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "products"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						// ArgumentPath uses the remapped variable name "a"
						{EntityKeyField: "upc", ArgumentPath: []string{"a"}, ArgumentIsEntityKey: true},
					},
				},
			},
		}

		// Variables use original name "upcs", RemapVariables maps "a" → "upcs"
		ctx := &Context{
			Variables:      astjson.MustParse(`{"upcs":["p1","p2"]}`),
			RemapVariables: map[string]string{"a": "upcs"},
			ctx:            context.Background(),
		}
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{nil}, "")
		assert.NoError(t, err)
		assert.Equal(t, []*CacheKey{
			{Keys: []string{`{"__typename":"Product","key":{"upc":"p1"}}`}, BatchIndex: 0},
			{Keys: []string{`{"__typename":"Product","key":{"upc":"p2"}}`}, BatchIndex: 1},
		}, cacheKeys)
	})

	t.Run("constructor precomputes batch entity key metadata", func(t *testing.T) {
		tmpl := NewRootQueryCacheKeyTemplate(
			[]QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "products"}},
			},
			[]EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "upc", ArgumentPath: []string{"upcs"}, ArgumentIsEntityKey: true},
					},
				},
			},
		)

		assert.True(t, tmpl.batchEntityKeyPrecomputed)
		assert.True(t, tmpl.hasBatchEntityKey)
		assert.Equal(t, []string{"upcs"}, tmpl.batchEntityKeyArgumentPath)
		assert.True(t, tmpl.HasBatchEntityKey())
		assert.Equal(t, []string{"upcs"}, tmpl.BatchEntityKeyArgumentPath())
	})

	t.Run("batch entity key with RemapVariables produces per-element keys", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{Coordinate: GraphCoordinate{TypeName: "Query", FieldName: "articles"}},
			},
			EntityKeyMappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Article",
					FieldMappings: []EntityFieldMappingConfig{
						{EntityKeyField: "id", ArgumentPath: []string{"a"}, ArgumentIsEntityKey: true},
					},
				},
			},
		}

		// Variables use remapped name "a", original argument name is "ids"
		ctx := &Context{
			Variables:      astjson.MustParse(`{"ids":["1","2","3"]}`),
			RemapVariables: map[string]string{"a": "ids"},
			ctx:            context.Background(),
		}
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{nil}, "")
		assert.NoError(t, err)
		assert.Equal(t, 3, len(cacheKeys))
		assert.Equal(t, []*CacheKey{
			{Keys: []string{`{"__typename":"Article","key":{"id":"1"}}`}, BatchIndex: 0},
			{Keys: []string{`{"__typename":"Article","key":{"id":"2"}}`}, BatchIndex: 1},
			{Keys: []string{`{"__typename":"Article","key":{"id":"3"}}`}, BatchIndex: 2},
		}, cacheKeys)
	})
}

// TestEntityQueryCacheKeyTemplate_NumericKeyCoercion pins down the number→string
// coercion contract on the entity-data rendering path. The sibling paths
// (RootQueryCacheKeyTemplate.renderDerivedEntityKey /
// renderDerivedEntityKeyFromValue) coerce numeric @key values to strings via
// setNestedKey so that `{"id":1}` and `{"id":"1"}` share one cache entry.
// The entity-data path at caching.go:657 (EntityQueryCacheKeyTemplate.
// renderCacheKeys) must produce a byte-identical key for the same entity,
// otherwise the read path (derived key from args) and the write path
// (direct key from entity data) silently miss the cache.
func TestEntityQueryCacheKeyTemplate_NumericKeyCoercion(t *testing.T) {
	t.Parallel()

	t.Run("flat numeric @key field is coerced to string", func(t *testing.T) {
		tmpl := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("upc"), Value: &Scalar{Path: []string{"upc"}}},
				},
			}),
		}
		entity := astjson.MustParse(`{"__typename":"Product","upc":42,"name":"Widget"}`)

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		cacheKeys, err := tmpl.RenderCacheKeys(ar, nil, []*astjson.Value{entity}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t,
			`{"__typename":"Product","key":{"upc":"42"}}`,
			cacheKeys[0].Keys[0],
			"numeric @key values read from entity data must be coerced to strings, matching the derived-key path")
	})

	t.Run("float @key field is coerced to string", func(t *testing.T) {
		tmpl := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("price"), Value: &Scalar{Path: []string{"price"}}},
				},
			}),
		}
		entity := astjson.MustParse(`{"__typename":"Product","price":9.99}`)

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		cacheKeys, err := tmpl.RenderCacheKeys(ar, nil, []*astjson.Value{entity}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t,
			`{"__typename":"Product","key":{"price":"9.99"}}`,
			cacheKeys[0].Keys[0])
	})

	t.Run("nested composite numeric @key is coerced at all levels", func(t *testing.T) {
		// Composite @key: Store is keyed by location.id where location is a
		// nested Object node in the template and id is numeric in the response.
		tmpl := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{
						Name: []byte("location"),
						Value: &Object{
							Path: []string{"location"},
							Fields: []*Field{
								{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}}},
							},
						},
					},
				},
			}),
		}
		entity := astjson.MustParse(`{"__typename":"Store","location":{"id":7}}`)

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		cacheKeys, err := tmpl.RenderCacheKeys(ar, nil, []*astjson.Value{entity}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t,
			`{"__typename":"Store","key":{"location":{"id":"7"}}}`,
			cacheKeys[0].Keys[0],
			"numeric scalars inside nested composite @key Objects must also be coerced")
	})

	t.Run("string @key field is unchanged", func(t *testing.T) {
		// Regression guard: coercion must be a no-op for strings.
		tmpl := &EntityQueryCacheKeyTemplate{
			Keys: NewResolvableObjectVariable(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("upc"), Value: &String{Path: []string{"upc"}}},
				},
			}),
		}
		entity := astjson.MustParse(`{"__typename":"Product","upc":"42"}`)

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		cacheKeys, err := tmpl.RenderCacheKeys(ar, nil, []*astjson.Value{entity}, "")
		require.NoError(t, err)
		require.Equal(t, 1, len(cacheKeys))
		assert.Equal(t,
			`{"__typename":"Product","key":{"upc":"42"}}`,
			cacheKeys[0].Keys[0])
	})
}

// TestCacheKeyPathSymmetry_NumericKeys verifies that the read-path key (derived
// from request args via RootQueryCacheKeyTemplate) and the write-path key
// (derived from entity data via EntityQueryCacheKeyTemplate) are byte-identical
// when the @key values are numeric. Without coercion on both sides, these
// paths silently produce different keys for the same logical entity, causing
// every write to miss every subsequent read.
func TestCacheKeyPathSymmetry_NumericKeys(t *testing.T) {
	t.Parallel()

	// Read path: RootQueryCacheKeyTemplate reading args → derived entity key.
	readTmpl := &RootQueryCacheKeyTemplate{
		RootFields: []QueryField{
			{
				Coordinate:  GraphCoordinate{TypeName: "Query", FieldName: "product"},
				ResponseKey: "product",
				Args: []FieldArgument{
					{Name: "upc", Variable: &ContextVariable{Path: []string{"upc"}, Renderer: NewCacheKeyVariableRenderer()}},
				},
			},
		},
		EntityKeyMappings: []EntityKeyMappingConfig{
			{
				EntityTypeName: "Product",
				FieldMappings: []EntityFieldMappingConfig{
					{EntityKeyField: "upc", ArgumentPath: []string{"upc"}},
				},
			},
		},
	}

	// Write path: EntityQueryCacheKeyTemplate reading entity data → entity key.
	writeTmpl := &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				{Name: []byte("upc"), Value: &Scalar{Path: []string{"upc"}}},
			},
		}),
	}

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

	// Same logical entity: upc = 42 (number).
	ctx := &Context{Variables: astjson.MustParse(`{"upc":42}`), ctx: context.Background()}
	readKeys, err := readTmpl.RenderCacheKeys(ar, ctx, []*astjson.Value{astjson.MustParse(`{}`)}, "")
	require.NoError(t, err)
	require.Equal(t, 1, len(readKeys))

	entity := astjson.MustParse(`{"__typename":"Product","upc":42}`)
	writeKeys, err := writeTmpl.RenderCacheKeys(ar, nil, []*astjson.Value{entity}, "")
	require.NoError(t, err)
	require.Equal(t, 1, len(writeKeys))

	assert.Equal(t, readKeys[0].Keys[0], writeKeys[0].Keys[0],
		"read path (from args) and write path (from entity data) must produce identical keys for the same entity; otherwise reads silently miss writes")
}
