package resolve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

func TestParseKeyFields(t *testing.T) {
	assert.Equal(t,
		[]KeyField{
			{
				Name: "id",
			},
			{
				Name: "address",
				Children: []KeyField{
					{
						Name: "city",
					},
				},
			},
			{
				Name: "sku",
			},
		},
		ParseKeyFields("id address { city } sku"),
	)
}

func TestEntityQueryCacheKeyTemplateKeyFields(t *testing.T) {
	template := &EntityQueryCacheKeyTemplate{
		TypeName: "User",
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{
					Name:  []byte("__typename"),
					Value: &String{},
				},
				{
					Name:  []byte("id"),
					Value: &String{},
				},
				{
					Name: []byte("organization"),
					Value: &Object{
						Fields: []*Field{
							{
								Name:  []byte("slug"),
								Value: &String{},
							},
						},
					},
				},
			},
		}),
	}

	assert.Equal(t,
		[]KeyField{
			{
				Name: "id",
			},
			{
				Name: "organization",
				Children: []KeyField{
					{
						Name: "slug",
					},
				},
			},
		},
		template.KeyFields(),
	)
}

func TestEntityQueryCacheKeyTemplateRenderCacheKeys(t *testing.T) {
	tests := []struct {
		name         string
		template     *EntityQueryCacheKeyTemplate
		prefix       string
		itemJSON     string
		expectedKeys []*CacheKey
	}{
		{
			name: "single key",
			template: &EntityQueryCacheKeyTemplate{
				TypeName: "User",
				Keys: NewResolvableObjectVariable(&Object{
					Fields: []*Field{
						{
							Name:  []byte("id"),
							Value: &String{},
						},
					},
				}),
			},
			itemJSON: `{"__typename":"User","id":"123","name":"Ada"}`,
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"User","key":{"id":"123"}}`,
					},
				},
			},
		},
		{
			name: "composite key",
			template: &EntityQueryCacheKeyTemplate{
				TypeName: "Product",
				Keys: NewResolvableObjectVariable(&Object{
					Fields: []*Field{
						{
							Name:  []byte("sku"),
							Value: &String{},
						},
						{
							Name:  []byte("upc"),
							Value: &String{},
						},
					},
				}),
			},
			itemJSON: `{"__typename":"Product","sku":"ABC123","upc":"DEF456","name":"Hat"}`,
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Product","key":{"sku":"ABC123","upc":"DEF456"}}`,
					},
				},
			},
		},
		{
			name: "nested key",
			template: &EntityQueryCacheKeyTemplate{
				TypeName: "InventoryItem",
				Keys: NewResolvableObjectVariable(&Object{
					Fields: []*Field{
						{
							Name: []byte("store"),
							Value: &Object{
								Fields: []*Field{
									{
										Name:  []byte("id"),
										Value: &String{},
									},
								},
							},
						},
					},
				}),
			},
			itemJSON: `{"__typename":"InventoryItem","store":{"id":123,"name":"Berlin"},"quantity":4}`,
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"InventoryItem","key":{"store":{"id":"123"}}}`,
					},
				},
			},
		},
		{
			name: "array key",
			template: &EntityQueryCacheKeyTemplate{
				TypeName: "Product",
				Keys: NewResolvableObjectVariable(&Object{
					Fields: []*Field{
						{
							Name: []byte("tags"),
							Value: &Array{
								Item: &String{},
							},
						},
					},
				}),
			},
			itemJSON: `{"__typename":"Product","tags":["electronics","sale"],"name":"Camera"}`,
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Product","key":{"tags":["electronics","sale"]}}`,
					},
				},
			},
		},
		{
			name: "typename fallback",
			template: &EntityQueryCacheKeyTemplate{
				TypeName: "User",
				Keys: NewResolvableObjectVariable(&Object{
					Fields: []*Field{
						{
							Name:  []byte("id"),
							Value: &String{},
						},
					},
				}),
			},
			itemJSON: `{"id":"123","name":"Ada"}`,
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"User","key":{"id":"123"}}`,
					},
				},
			},
		},
		{
			name: "with prefix",
			template: &EntityQueryCacheKeyTemplate{
				TypeName: "User",
				Keys: NewResolvableObjectVariable(&Object{
					Fields: []*Field{
						{
							Name:  []byte("id"),
							Value: &String{},
						},
					},
				}),
			},
			prefix:   "tenant-a",
			itemJSON: `{"__typename":"User","id":"123"}`,
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`tenant-a:{"__typename":"User","key":{"id":"123"}}`,
					},
				},
			},
		},
		{
			name: "missing key field produces zero keys",
			template: &EntityQueryCacheKeyTemplate{
				TypeName: "User",
				Keys: NewResolvableObjectVariable(&Object{
					Fields: []*Field{
						{
							Name:  []byte("id"),
							Value: &String{},
						},
					},
				}),
			},
			itemJSON:     `{"__typename":"User","name":"Ada"}`,
			expectedKeys: []*CacheKey{},
		},
		{
			name: "null key field produces zero keys",
			template: &EntityQueryCacheKeyTemplate{
				TypeName: "User",
				Keys: NewResolvableObjectVariable(&Object{
					Fields: []*Field{
						{
							Name:  []byte("id"),
							Value: &String{},
						},
					},
				}),
			},
			itemJSON:     `{"__typename":"User","id":null}`,
			expectedKeys: []*CacheKey{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := acquireCacheKeyTestArena(t)
			item := parseCacheKeyTestValue(t, a, tt.itemJSON)

			actual, err := tt.template.RenderCacheKeys(a, NewContext(context.Background()), []*astjson.Value{item}, tt.prefix)
			require.NoError(t, err)

			for i := range tt.expectedKeys {
				tt.expectedKeys[i].Item = item
			}
			assert.Equal(t, tt.expectedKeys, actual)
			for i := range actual {
				assert.Same(t, item, actual[i].Item)
			}
		})
	}
}

func TestEntityQueryCacheKeyTemplateNumberCoercionParity(t *testing.T) {
	a := acquireCacheKeyTestArena(t)
	intItem := parseCacheKeyTestValue(t, a, `{"__typename":"User","id":1}`)
	stringItem := parseCacheKeyTestValue(t, a, `{"__typename":"User","id":"1"}`)
	template := &EntityQueryCacheKeyTemplate{
		TypeName: "User",
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{
					Name:  []byte("id"),
					Value: &String{},
				},
			},
		}),
	}

	intKeys, err := template.RenderCacheKeys(a, NewContext(context.Background()), []*astjson.Value{intItem}, "")
	require.NoError(t, err)
	stringKeys, err := template.RenderCacheKeys(a, NewContext(context.Background()), []*astjson.Value{stringItem}, "")
	require.NoError(t, err)

	assert.Equal(t,
		[]*CacheKey{
			{
				Item: intItem,
				Keys: []string{
					`{"__typename":"User","key":{"id":"1"}}`,
				},
			},
		},
		intKeys,
	)
	assert.Equal(t,
		[]*CacheKey{
			{
				Item: stringItem,
				Keys: []string{
					`{"__typename":"User","key":{"id":"1"}}`,
				},
			},
		},
		stringKeys,
	)
	assert.Equal(t, intKeys[0].Keys, stringKeys[0].Keys)
	assert.Same(t, intItem, intKeys[0].Item)
	assert.Same(t, stringItem, stringKeys[0].Item)
}

func TestRootQueryCacheKeyTemplateRenderCacheKeys(t *testing.T) {
	tests := []struct {
		name         string
		field        RootField
		expectedKeys []*CacheKey
	}{
		{
			name: "no args",
			field: RootField{
				TypeName:  "Query",
				FieldName: "topProducts",
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Query","field":"topProducts"}`,
					},
				},
			},
		},
		{
			name: "single string arg",
			field: RootField{
				TypeName:  "Query",
				FieldName: "user",
				Arguments: []RootFieldArgument{
					{
						Name:  "id",
						Value: parseCacheKeyTestHeapValue(t, `"123"`),
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Query","field":"user","args":{"id":"123"}}`,
					},
				},
			},
		},
		{
			name: "multiple args sorted alphabetically",
			field: RootField{
				TypeName:  "Query",
				FieldName: "search",
				Arguments: []RootFieldArgument{
					{
						Name:  "z",
						Value: parseCacheKeyTestHeapValue(t, `3`),
					},
					{
						Name:  "a",
						Value: parseCacheKeyTestHeapValue(t, `"first"`),
					},
					{
						Name:  "m",
						Value: parseCacheKeyTestHeapValue(t, `true`),
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Query","field":"search","args":{"a":"first","m":true,"z":3}}`,
					},
				},
			},
		},
		{
			name: "bool arg",
			field: RootField{
				TypeName:  "Query",
				FieldName: "products",
				Arguments: []RootFieldArgument{
					{
						Name:  "available",
						Value: parseCacheKeyTestHeapValue(t, `false`),
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Query","field":"products","args":{"available":false}}`,
					},
				},
			},
		},
		{
			name: "number arg",
			field: RootField{
				TypeName:  "Query",
				FieldName: "topProducts",
				Arguments: []RootFieldArgument{
					{
						Name:  "first",
						Value: parseCacheKeyTestHeapValue(t, `2`),
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Query","field":"topProducts","args":{"first":2}}`,
					},
				},
			},
		},
		{
			name: "array arg",
			field: RootField{
				TypeName:  "Query",
				FieldName: "products",
				Arguments: []RootFieldArgument{
					{
						Name:  "upcs",
						Value: parseCacheKeyTestHeapValue(t, `["top-1","top-2"]`),
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Query","field":"products","args":{"upcs":["top-1","top-2"]}}`,
					},
				},
			},
		},
		{
			name: "object arg sorted recursively",
			field: RootField{
				TypeName:  "Query",
				FieldName: "search",
				Arguments: []RootFieldArgument{
					{
						Name:  "input",
						Value: parseCacheKeyTestHeapValue(t, `{"term":"hat","filters":{"z":2,"a":1}}`),
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Query","field":"search","args":{"input":{"filters":{"a":1,"z":2},"term":"hat"}}}`,
					},
				},
			},
		},
		{
			name: "null arg",
			field: RootField{
				TypeName:  "Query",
				FieldName: "user",
				Arguments: []RootFieldArgument{
					{
						Name:  "id",
						Value: parseCacheKeyTestHeapValue(t, `null`),
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Query","field":"user","args":{"id":null}}`,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := acquireCacheKeyTestArena(t)
			item := parseCacheKeyTestValue(t, a, `{"root":true}`)
			template := NewRootQueryCacheKeyTemplate(
				[]RootField{
					tt.field,
				},
				nil,
			)

			actual, err := template.RenderCacheKeys(a, NewContext(context.Background()), []*astjson.Value{item}, "")
			require.NoError(t, err)

			for i := range tt.expectedKeys {
				tt.expectedKeys[i].Item = item
			}
			assert.Equal(t, tt.expectedKeys, actual)
			for i := range actual {
				assert.Same(t, item, actual[i].Item)
			}
		})
	}
}

func TestRootQueryCacheKeyTemplateEntityKeyMappings(t *testing.T) {
	tests := []struct {
		name         string
		variables    string
		mappings     []EntityKeyMappingConfig
		expectedKeys []*CacheKey
	}{
		{
			name:      "simple id",
			variables: `{"id":"123"}`,
			mappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{
							EntityKeyField: "id",
							ArgumentPath:   []string{"id"},
						},
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"User","key":{"id":"123"}}`,
					},
				},
			},
		},
		{
			name:      "int to string",
			variables: `{"id":123}`,
			mappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{
							EntityKeyField: "id",
							ArgumentPath:   []string{"id"},
						},
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"User","key":{"id":"123"}}`,
					},
				},
			},
		},
		{
			name:      "nested path",
			variables: `{"input":{"store":{"id":123}}}`,
			mappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "InventoryItem",
					FieldMappings: []EntityFieldMappingConfig{
						{
							EntityKeyField: "store.id",
							ArgumentPath:   []string{"input", "store", "id"},
						},
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"InventoryItem","key":{"store":{"id":"123"}}}`,
					},
				},
			},
		},
		{
			name:      "array index",
			variables: `{"ids":["top-1","top-2"]}`,
			mappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "Product",
					FieldMappings: []EntityFieldMappingConfig{
						{
							EntityKeyField: "upc",
							ArgumentPath:   []string{"ids", "1"},
						},
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"Product","key":{"upc":"top-2"}}`,
					},
				},
			},
		},
		{
			name:      "multiple mappings",
			variables: `{"id":"123","email":"ada@example.com"}`,
			mappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{
							EntityKeyField: "id",
							ArgumentPath:   []string{"id"},
						},
					},
				},
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{
							EntityKeyField: "email",
							ArgumentPath:   []string{"email"},
						},
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"User","key":{"id":"123"}}`,
						`{"__typename":"User","key":{"email":"ada@example.com"}}`,
					},
				},
			},
		},
		{
			name:      "partial missing skips only that key",
			variables: `{"id":"123"}`,
			mappings: []EntityKeyMappingConfig{
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{
							EntityKeyField: "id",
							ArgumentPath:   []string{"id"},
						},
					},
				},
				{
					EntityTypeName: "User",
					FieldMappings: []EntityFieldMappingConfig{
						{
							EntityKeyField: "email",
							ArgumentPath:   []string{"email"},
						},
					},
				},
			},
			expectedKeys: []*CacheKey{
				{
					Keys: []string{
						`{"__typename":"User","key":{"id":"123"}}`,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := acquireCacheKeyTestArena(t)
			item := parseCacheKeyTestValue(t, a, `{"root":true}`)
			ctx := NewContext(context.Background())
			ctx.Variables = parseCacheKeyTestValue(t, a, tt.variables)
			template := NewRootQueryCacheKeyTemplate(
				[]RootField{
					{
						TypeName:    "Query",
						FieldName:   "user",
						ResponseKey: "user",
					},
				},
				tt.mappings,
			)

			actual, err := template.RenderCacheKeys(a, ctx, []*astjson.Value{item}, "")
			require.NoError(t, err)

			for i := range tt.expectedKeys {
				tt.expectedKeys[i].Item = item
			}
			assert.Equal(t, tt.expectedKeys, actual)
			for i := range actual {
				assert.Same(t, item, actual[i].Item)
			}
		})
	}
}

func TestRootQueryCacheKeyTemplateBatchEntityKeyMappings(t *testing.T) {
	a := acquireCacheKeyTestArena(t)
	item := parseCacheKeyTestValue(t, a, `{"root":true}`)
	ctx := NewContext(context.Background())
	ctx.Variables = parseCacheKeyTestValue(t, a, `{"upcs":["top-1","top-2","top-3"]}`)
	template := NewRootQueryCacheKeyTemplate(
		[]RootField{
			{
				TypeName:    "Query",
				FieldName:   "products",
				ResponseKey: "products",
			},
		},
		[]EntityKeyMappingConfig{
			{
				EntityTypeName: "Product",
				FieldMappings: []EntityFieldMappingConfig{
					{
						EntityKeyField:      "upc",
						ArgumentPath:        []string{"upcs"},
						ArgumentIsEntityKey: true,
					},
				},
			},
		},
	)

	actual, err := template.RenderCacheKeys(a, ctx, []*astjson.Value{item}, "")
	require.NoError(t, err)

	assert.Equal(t,
		[]*CacheKey{
			{
				Item:       item,
				BatchIndex: 0,
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
				},
			},
			{
				Item:       item,
				BatchIndex: 1,
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-2"}}`,
				},
			},
			{
				Item:       item,
				BatchIndex: 2,
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-3"}}`,
				},
			},
		},
		actual,
	)
	for i := range actual {
		assert.Same(t, item, actual[i].Item)
	}
	assert.Equal(t, []string{"upcs"}, template.BatchEntityKeyArgumentPath())
	assert.True(t, template.IsEntityFetch())
	assert.Equal(t, []string{"products"}, template.EntityMergePath(PostProcessingConfiguration{}))
	assert.Equal(t, []string{"data", "products"}, template.EntityMergePath(PostProcessingConfiguration{
		MergePath: []string{"data", "products"},
	}))
}

func TestRootQueryCacheKeyTemplateBatchEntityKeyMappingsSkipMissingElement(t *testing.T) {
	a := acquireCacheKeyTestArena(t)
	item := parseCacheKeyTestValue(t, a, `{"root":true}`)
	ctx := NewContext(context.Background())
	ctx.Variables = parseCacheKeyTestValue(t, a, `{"items":[{"upc":"top-1"},{"name":"missing"},{"upc":"top-3"}]}`)
	template := NewRootQueryCacheKeyTemplate(
		[]RootField{
			{
				TypeName:    "Query",
				FieldName:   "products",
				ResponseKey: "products",
			},
		},
		[]EntityKeyMappingConfig{
			{
				EntityTypeName: "Product",
				FieldMappings: []EntityFieldMappingConfig{
					{
						EntityKeyField:      "upc",
						ArgumentPath:        []string{"items", "upc"},
						ArgumentIsEntityKey: true,
					},
				},
			},
		},
	)

	actual, err := template.RenderCacheKeys(a, ctx, []*astjson.Value{item}, "")
	require.NoError(t, err)

	assert.Equal(t,
		[]*CacheKey{
			{
				Item:       item,
				BatchIndex: 0,
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-1"}}`,
				},
			},
			{
				Item:       item,
				BatchIndex: 2,
				Keys: []string{
					`{"__typename":"Product","key":{"upc":"top-3"}}`,
				},
			},
		},
		actual,
	)
	for i := range actual {
		assert.Same(t, item, actual[i].Item)
	}
}

func acquireCacheKeyTestArena(t *testing.T) arena.Arena {
	t.Helper()

	pool := arena.NewArenaPool()
	item := pool.Acquire(0)
	t.Cleanup(func() {
		pool.Release(item)
	})
	return item.Arena
}

func parseCacheKeyTestValue(t *testing.T, a arena.Arena, data string) *astjson.Value {
	t.Helper()

	value, err := astjson.ParseBytesWithArena(a, []byte(data))
	require.NoError(t, err)
	return value
}

func parseCacheKeyTestHeapValue(t *testing.T, data string) *astjson.Value {
	t.Helper()

	value, err := astjson.ParseBytes([]byte(data))
	require.NoError(t, err)
	return value
}
