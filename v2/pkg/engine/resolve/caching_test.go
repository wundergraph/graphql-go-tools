package resolve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

func TestCachingRenderRootQueryCacheKeyTemplate(t *testing.T) {
	t.Run("single field no arguments", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate: GraphCoordinate{
						TypeName:  "Query",
						FieldName: "users",
					},
					Args: []FieldArgument{},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"users"}`,
						Path: "users",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single field single argument", func(t *testing.T) {
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
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":1}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"droid","args":{"id":1}}`,
						Path: "droid",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single field single string argument", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
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
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"name":"john"}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"user","args":{"name":"john"}}`,
						Path: "user",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single field multiple arguments", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
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

		ctx := &Context{
			Variables: astjson.MustParse(`{"term":"C3PO","max":10}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"search","args":{"term":"C3PO","max":10}}`,
						Path: "search",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single field multiple arguments with boolean", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate: GraphCoordinate{
						TypeName:  "Query",
						FieldName: "products",
					},
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
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"products","args":{"includeDeleted":true,"limit":20}}`,
						Path: "products",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("multiple fields single argument each", func(t *testing.T) {
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
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":1,"name":"john"}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)

		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"droid","args":{"id":1}}`,
						Path: "droid",
					},
					{
						Name: `{"__typename":"Query","field":"user","args":{"name":"john"}}`,
						Path: "user",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("multiple fields with mixed arguments", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate: GraphCoordinate{
						TypeName:  "Query",
						FieldName: "product",
					},
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
					Coordinate: GraphCoordinate{
						TypeName:  "Query",
						FieldName: "hero",
					},
					Args: []FieldArgument{},
				},
			},
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":"123","includeReviews":true}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)

		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"product","args":{"id":"123","includeReviews":true}}`,
						Path: "product",
					},
					{
						Name: `{"__typename":"Query","field":"hero"}`,
						Path: "hero",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("field with object variable argument", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate: GraphCoordinate{
						TypeName:  "Query",
						FieldName: "search",
					},
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
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"search","args":{"filter":{"category":"electronics","price":100}}}`,
						Path: "search",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("field with null argument", func(t *testing.T) {
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

		ctx := &Context{
			Variables: astjson.MustParse(`{"id":null}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"user","args":{"id":null}}`,
						Path: "user",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("field with missing argument", func(t *testing.T) {
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

		ctx := &Context{
			Variables: astjson.MustParse(`{}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"user","args":{"id":null}}`,
						Path: "user",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("field with array argument", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate: GraphCoordinate{
						TypeName:  "Query",
						FieldName: "products",
					},
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
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"products","args":{"ids":[1,2,3]}}`,
						Path: "products",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("non-Query type", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
				{
					Coordinate: GraphCoordinate{
						TypeName:  "Subscription",
						FieldName: "messageAdded",
					},
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
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Subscription","field":"messageAdded","args":{"roomId":"123"}}`,
						Path: "messageAdded",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("single field with arena", func(t *testing.T) {
		tmpl := &RootQueryCacheKeyTemplate{
			RootFields: []QueryField{
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
			},
		}

		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := &Context{
			Variables: astjson.MustParse(`{"name":"john"}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{}`)
		cacheKeys, err := tmpl.RenderCacheKeys(ar, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Query","field":"user","args":{"name":"john"}}`,
						Path: "user",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})
}

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
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Product","keys":{"id":"123"}}`,
					},
				},
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
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"Product","keys":{"sku":"ABC123","upc":"DEF456"}}`,
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
	})

	t.Run("entity with nested object key", func(t *testing.T) {
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
						Name: []byte("key"),
						Value: &Object{
							Fields: []*Field{
								{
									Name: []byte("id"),
									Value: &String{
										Path: []string{"key", "id"},
									},
								},
								{
									Name: []byte("version"),
									Value: &String{
										Path: []string{"key", "version"},
									},
								},
							},
						},
					},
				},
			}),
		}

		ctx := &Context{
			Variables: astjson.MustParse(`{}`),
			ctx:       context.Background(),
		}
		data := astjson.MustParse(`{"__typename":"VersionedEntity","key":{"id":"123","version":"1"}}`)
		cacheKeys, err := tmpl.RenderCacheKeys(nil, ctx, []*astjson.Value{data})
		assert.NoError(t, err)
		expected := []*CacheKey{
			{
				Item: data,
				Keys: []KeyEntry{
					{
						Name: `{"__typename":"VersionedEntity","keys":{"key":{"id":"123","version":"1"}}}`,
						Path: "",
					},
				},
			},
		}
		assert.Equal(t, expected, cacheKeys)
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
			_, err := tmpl.RenderCacheKeys(a, ctxRootQuery, items)
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
			_, err := tmpl.RenderCacheKeys(a, ctxRootQuery, items)
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
			_, err := tmpl.RenderCacheKeys(a, ctxEntityQuery, items)
			if err != nil {
				b.Fatal(err)
			}
		}
	})
}
