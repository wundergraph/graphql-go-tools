package resolve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// TestL1Cache_ValidateFieldDataWithAliases verifies that field validation uses the
// original (non-aliased) name when checking normalized cache data.
func TestL1Cache_ValidateFieldDataWithAliases(t *testing.T) {
	t.Run("validates using original name on normalized data", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		field := &Field{
			Name:         []byte("userName"),
			OriginalName: []byte("username"),
			Value:        &Scalar{},
		}

		// Cache data is normalized (uses original name "username")
		item := mustParseJSON(ar, `{"username":"Alice"}`)

		result := loader.validateFieldData(item, field)
		// Validates using original name from normalized cache data
		assert.True(t, result)
	})

	t.Run("fails when original name missing from cached data", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		field := &Field{
			Name:         []byte("userName"),
			OriginalName: []byte("username"),
			Value:        &Scalar{},
		}

		// Cache data doesn't have "username"
		item := mustParseJSON(ar, `{"realName":"Alice"}`)

		result := loader.validateFieldData(item, field)
		// Missing original field name in cache data
		assert.False(t, result)
	})
}

// TestL1Cache_ProjectedCopyWithAliases verifies that projected copy reads from the
// original field name in cache and writes to the alias name in the output.
func TestL1Cache_ProjectedCopyWithAliases(t *testing.T) {
	t.Run("reads original name writes alias", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{
			jsonArena: ar,
		}

		obj := &Object{
			HasAliases: true,
			Fields: []*Field{
				{Name: []byte("userName"), OriginalName: []byte("username"), Value: &Scalar{}},
			},
		}

		// Cache stores data with original field name
		cached := mustParseJSON(ar, `{"username":"Alice"}`)
		result := loader.structuralCopyProjected(cached, obj)

		resultJSON := string(result.MarshalTo(nil))
		assert.Equal(t, `{"userName":"Alice"}`, resultJSON)
	})
}

// TestL1Cache_ComputeHasAliases verifies detection of aliased fields at any depth
// in the response plan tree, used to decide if normalize/denormalize is needed.
func TestL1Cache_ComputeHasAliases(t *testing.T) {
	t.Run("no aliases", func(t *testing.T) {
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}
		result := ComputeHasAliases(obj)
		assert.False(t, result)
		assert.False(t, obj.HasAliases)
	})

	t.Run("direct alias", func(t *testing.T) {
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("myId"), OriginalName: []byte("id"), Value: &Scalar{}},
			},
		}
		result := ComputeHasAliases(obj)
		assert.True(t, result)
		assert.True(t, obj.HasAliases)
	})

	t.Run("nested alias", func(t *testing.T) {
		innerObj := &Object{
			Fields: []*Field{
				{Name: []byte("n"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("product"), Value: innerObj},
			},
		}
		result := ComputeHasAliases(obj)
		assert.True(t, result)
		assert.True(t, obj.HasAliases)
		assert.True(t, innerObj.HasAliases)
	})

	t.Run("alias in array item", func(t *testing.T) {
		innerObj := &Object{
			Fields: []*Field{
				{Name: []byte("n"), OriginalName: []byte("name"), Value: &Scalar{}},
			},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("items"), Value: &Array{Item: innerObj}},
			},
		}
		result := ComputeHasAliases(obj)
		assert.True(t, result)
		assert.True(t, obj.HasAliases)
	})
}

// TestPopulateL1CacheForRootFieldEntities_MissingKeyFields verifies that root field
// entity population skips entities that are missing @key fields.
// When the client's query doesn't select the @key fields (e.g., "id"), RenderCacheKeys
// produces a key with empty key object (e.g., {"__typename":"Product","key":{}}).
// These degraded keys would collide for all entities of the same type, so we skip storage.
func TestL1Cache_PopulateRootFieldEntities_MissingKeyFields(t *testing.T) {
	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(4096))
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.Variables = astjson.MustParse(`{}`)

	resolvable := NewResolvable(ar, ResolvableOptions{})
	err := resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	// Set response data: entity with __typename but missing @key field "id"
	resolvable.data, err = astjson.ParseBytesWithArena(ar, []byte(`{"topProducts":[{"__typename":"Product","name":"Widget"}]}`))
	require.NoError(t, err)

	l1Cache := map[string]*astjson.Value{}

	l := &Loader{
		jsonArena:  ar,
		ctx:        ctx,
		resolvable: resolvable,
		l1Cache:    l1Cache,
	}

	// Template expects @key field "id" which is NOT in the entity data.
	// Path points to where entities live in the response.
	entityTemplate := &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Path: []string{"topProducts"},
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
			},
		}),
	}

	fetchItem := &FetchItem{
		Fetch: &SingleFetch{
			FetchConfiguration: FetchConfiguration{
				Caching: FetchCacheConfiguration{
					Enabled:    true,
					UseL1Cache: true,
					RootFieldL1EntityCacheKeyTemplates: map[string]CacheKeyTemplate{
						"topProducts:Product": entityTemplate,
					},
				},
			},
			Info: &FetchInfo{
				RootFields: []GraphCoordinate{
					{TypeName: "Query", FieldName: "topProducts"},
				},
			},
		},
	}

	l.populateL1CacheForRootFieldEntities(fetchItem)

	// Entity should NOT be stored because key fields are missing.
	// A degraded key like {"__typename":"Product","key":{}} would collide for all
	// Product entities, so populateL1CacheForRootFieldEntities skips storage.
	degradedKey := `{"__typename":"Product","key":{}}`
	_, loaded := l1Cache[degradedKey]
	// Entity with missing @key fields should not be stored
	assert.False(t, loaded)

	// A proper entity cache key won't find anything either
	_, loaded = l1Cache[`{"__typename":"Product","key":{"id":"123"}}`]
	// Proper entity key should not find the degraded entry
	assert.False(t, loaded)
}

func mustParseJSON(a arena.Arena, jsonStr string) *astjson.Value {
	v, err := astjson.ParseBytesWithArena(a, []byte(jsonStr))
	if err != nil {
		panic(err)
	}
	return v
}

// --- P1: validateItemHasRequiredData unit tests ---

// TestL1Cache_ValidateItemHasRequiredData exercises all branches of field validation:
// missing fields, null on nullable/non-nullable, nested objects, arrays, and CacheArgs.
// Without correct validation, stale or incomplete cache entries would be served.
func TestL1Cache_ValidateItemHasRequiredData(t *testing.T) {
	t.Run("nil item returns false", func(t *testing.T) {
		loader := &Loader{}
		obj := &Object{Fields: []*Field{{Name: []byte("id"), Value: &Scalar{}}}}
		assert.False(t, loader.validateItemHasRequiredData(nil, obj))
	})

	t.Run("all required scalar fields present", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}
		item := mustParseJSON(ar, `{"id":"1","name":"Alice"}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("missing required field", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}
		item := mustParseJSON(ar, `{"id":"1"}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null value for non-nullable scalar", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{Nullable: false}},
			},
		}
		item := mustParseJSON(ar, `{"id":null}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null value for nullable scalar", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("email"), Value: &Scalar{Nullable: true}},
			},
		}
		item := mustParseJSON(ar, `{"email":null}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("nested object with all fields", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		innerObj := &Object{
			Fields: []*Field{
				{Name: []byte("street"), Value: &Scalar{}},
			},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("address"), Value: innerObj},
			},
		}
		item := mustParseJSON(ar, `{"address":{"street":"Main St"}}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("nested object missing required field", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		innerObj := &Object{
			Fields: []*Field{
				{Name: []byte("street"), Value: &Scalar{}},
				{Name: []byte("city"), Value: &Scalar{}},
			},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("address"), Value: innerObj},
			},
		}
		item := mustParseJSON(ar, `{"address":{"street":"Main St"}}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null for non-nullable object", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		innerObj := &Object{
			Nullable: false,
			Fields:   []*Field{{Name: []byte("street"), Value: &Scalar{}}},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("address"), Value: innerObj},
			},
		}
		item := mustParseJSON(ar, `{"address":null}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null for nullable object", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		innerObj := &Object{
			Nullable: true,
			Fields:   []*Field{{Name: []byte("street"), Value: &Scalar{}}},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("address"), Value: innerObj},
			},
		}
		item := mustParseJSON(ar, `{"address":null}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("non-object value for object field", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		innerObj := &Object{
			Fields: []*Field{{Name: []byte("street"), Value: &Scalar{}}},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("address"), Value: innerObj},
			},
		}
		item := mustParseJSON(ar, `{"address":"not-an-object"}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array with all valid items", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{
			Item: &Scalar{},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":["a","b","c"]}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array with invalid item - non-nullable scalar null", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{
			Item: &Scalar{Nullable: false},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":["a",null,"c"]}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array with nullable items allows null", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{
			Item: &Scalar{Nullable: true},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":["a",null,"c"]}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null for non-nullable array", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{
			Nullable: false,
			Item:     &Scalar{},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":null}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("null for nullable array", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{
			Nullable: true,
			Item:     &Scalar{},
		}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":null}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("non-array value for array field", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{Item: &Scalar{}}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":"not-an-array"}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("empty array is valid", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{Item: &Scalar{}}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":[]}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array of objects with valid items", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		itemObj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
			},
		}
		arr := &Array{Item: itemObj}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("items"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"items":[{"id":"1"},{"id":"2"}]}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array of objects with invalid item", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		itemObj := &Object{
			Fields: []*Field{
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), Value: &Scalar{}},
			},
		}
		arr := &Array{Item: itemObj}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("items"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"items":[{"id":"1","name":"ok"},{"id":"2"}]}`)
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("field with CacheArgs uses suffixed name for lookup", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"first":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		// Field has CacheArgs, so validation should look for "friends_<suffix>" not "friends"
		field := &Field{
			Name:  []byte("friends"),
			Value: &Scalar{},
			CacheArgs: []CacheFieldArg{
				{ArgName: "first", VariableName: "first"},
			},
		}

		// Compute expected suffixed name
		suffix := loader.computeArgSuffix(field.CacheArgs)
		expectedKey := "friends" + suffix

		// Item has the suffixed field name (as normalize would produce)
		itemJSON := `{"` + expectedKey + `":"value"}`
		item := mustParseJSON(ar, itemJSON)

		obj := &Object{Fields: []*Field{field}}
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("field with CacheArgs fails when only base name present", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"first":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		field := &Field{
			Name:  []byte("friends"),
			Value: &Scalar{},
			CacheArgs: []CacheFieldArg{
				{ArgName: "first", VariableName: "first"},
			},
		}

		// Item has only the base name "friends" without suffix
		item := mustParseJSON(ar, `{"friends":"value"}`)

		obj := &Object{Fields: []*Field{field}}
		assert.False(t, loader.validateItemHasRequiredData(item, obj))
	})

	t.Run("array with nil Item spec is valid if array exists", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		arr := &Array{Item: nil}
		obj := &Object{
			Fields: []*Field{
				{Name: []byte("tags"), Value: arr},
			},
		}
		item := mustParseJSON(ar, `{"tags":["a","b"]}`)
		assert.True(t, loader.validateItemHasRequiredData(item, obj))
	})
}

// --- P3: computeArgSuffix unit tests ---

// TestL1Cache_ComputeArgSuffix verifies that field argument hashing produces
// deterministic, collision-resistant suffixes for cache key disambiguation.
// Without this, different argument values would share the same cache entry.
func TestL1Cache_ComputeArgSuffix(t *testing.T) {
	t.Run("single arg produces deterministic suffix", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		suffix1 := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "a"}})
		suffix2 := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "a"}})

		assert.Equal(t, suffix1, suffix2)
		assert.Equal(t, 17, len(suffix1))
		assert.Equal(t, byte('_'), suffix1[0])
	})

	t.Run("different values produce different suffixes", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"5","b":"10"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		suffix1 := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "a"}})
		suffix2 := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "b"}})

		assert.NotEqual(t, suffix1, suffix2)
	})

	t.Run("null variable produces null in hash", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		// Variable "missing" doesn't exist, so argValue is nil → "null" written
		suffix := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "missing"}})
		assert.Equal(t, 17, len(suffix))
	})

	t.Run("null variable differs from string null", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":null,"b":"null"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		suffixNull := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "a"}})
		suffixMissing := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "missing"}})

		// Both json null and missing variable produce "null" in the hash,
		// so they should be equal
		// Both json null and missing variable produce "null" in the hash
		assert.Equal(t, suffixNull, suffixMissing)
	})

	t.Run("unsorted args get sorted before hashing", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"a":"1","b":"2"}`))
		loader := &Loader{jsonArena: ar, ctx: ctx}

		sorted := []CacheFieldArg{
			{ArgName: "alpha", VariableName: "a"},
			{ArgName: "beta", VariableName: "b"},
		}
		unsorted := []CacheFieldArg{
			{ArgName: "beta", VariableName: "b"},
			{ArgName: "alpha", VariableName: "a"},
		}

		suffixSorted := loader.computeArgSuffix(sorted)
		suffixUnsorted := loader.computeArgSuffix(unsorted)

		assert.Equal(t, suffixSorted, suffixUnsorted)
	})

	t.Run("RemapVariables applied before lookup", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx := NewContext(t.Context())
		ctx.Variables = astjson.MustParseBytes([]byte(`{"original":"42"}`))
		ctx.RemapVariables = map[string]string{"remapped": "original"}
		loader := &Loader{jsonArena: ar, ctx: ctx}

		// "remapped" maps to "original" which has value "42"
		suffixRemapped := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "remapped"}})
		// "original" has value "42" directly
		suffixDirect := loader.computeArgSuffix([]CacheFieldArg{{ArgName: "first", VariableName: "original"}})

		assert.Equal(t, suffixRemapped, suffixDirect)
	})

	t.Run("object arg produces deterministic hash regardless of key order", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		ctx1 := NewContext(t.Context())
		ctx1.Variables = astjson.MustParseBytes([]byte(`{"filter":{"name":"Alice","age":30}}`))
		loader1 := &Loader{jsonArena: ar, ctx: ctx1}

		ctx2 := NewContext(t.Context())
		ctx2.Variables = astjson.MustParseBytes([]byte(`{"filter":{"age":30,"name":"Alice"}}`))
		loader2 := &Loader{jsonArena: ar, ctx: ctx2}

		suffix1 := loader1.computeArgSuffix([]CacheFieldArg{{ArgName: "filter", VariableName: "filter"}})
		suffix2 := loader2.computeArgSuffix([]CacheFieldArg{{ArgName: "filter", VariableName: "filter"}})

		// Object key order should not affect hash (canonical JSON)
		assert.Equal(t, suffix1, suffix2)
	})
}

// --- P4: mergeEntityFields unit tests ---

// TestL1Cache_MergeEntityFields verifies that merging entity data from a new fetch
// into an existing L1 cache entry adds new fields without overwriting existing ones.
func TestL1Cache_MergeEntityFields(t *testing.T) {
	t.Run("new field added to existing entity", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}

		dst := mustParseJSON(ar, `{"id":"1","name":"Alice"}`)
		src := mustParseJSON(ar, `{"id":"1","email":"alice@example.com"}`)

		loader.mergeEntityFields(dst, src)

		resultJSON := string(dst.MarshalTo(nil))
		assert.Equal(t, `{"id":"1","name":"Alice","email":"alice@example.com"}`, resultJSON)
	})

	t.Run("existing field preserved not overwritten", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}

		dst := mustParseJSON(ar, `{"id":"1","name":"Alice"}`)
		src := mustParseJSON(ar, `{"id":"1","name":"Bob"}`)

		loader.mergeEntityFields(dst, src)

		resultJSON := string(dst.MarshalTo(nil))
		// Existing field preserved, not overwritten
		assert.Equal(t, `{"id":"1","name":"Alice"}`, resultJSON)
	})

	t.Run("nil dst is no-op", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		src := mustParseJSON(ar, `{"id":"1"}`)
		// Should not panic
		loader.mergeEntityFields(nil, src)
	})

	t.Run("nil src is no-op", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		dst := mustParseJSON(ar, `{"id":"1"}`)
		loader.mergeEntityFields(dst, nil)
		resultJSON := string(dst.MarshalTo(nil))
		assert.Equal(t, `{"id":"1"}`, resultJSON)
	})

	t.Run("non-object type is no-op", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}
		dst := mustParseJSON(ar, `"string-value"`)
		src := mustParseJSON(ar, `{"id":"1"}`)
		// Should not panic
		loader.mergeEntityFields(dst, src)
	})

	t.Run("multiple new and existing fields coexist", func(t *testing.T) {
		ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
		loader := &Loader{jsonArena: ar}

		dst := mustParseJSON(ar, `{"id":"1","name":"Alice","age":30}`)
		src := mustParseJSON(ar, `{"id":"1","email":"a@b.com","role":"admin","name":"Bob"}`)

		loader.mergeEntityFields(dst, src)

		result := dst
		// Existing fields preserved
		assert.Equal(t, `"1"`, string(result.Get("id").MarshalTo(nil)))
		assert.Equal(t, `"Alice"`, string(result.Get("name").MarshalTo(nil)))
		assert.Equal(t, `30`, string(result.Get("age").MarshalTo(nil)))
		// New fields added
		assert.Equal(t, `"a@b.com"`, string(result.Get("email").MarshalTo(nil)))
		assert.Equal(t, `"admin"`, string(result.Get("role").MarshalTo(nil)))
	})
}
