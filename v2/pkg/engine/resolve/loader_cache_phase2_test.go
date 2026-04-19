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

// TestL1Cache_RootFieldPromotionWithAliases verifies that root-field L1
// promotion stores entity values using SCHEMA field names, not response
// (aliased) names. Without the normalize-on-write fix, an aliased root query
// would silently corrupt entity L1 reads for subsequent entity fetches.
func TestL1Cache_RootFieldPromotionWithAliases(t *testing.T) {
	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL1Cache = true

	loader := &Loader{
		jsonArena: ar,
		l1Cache:   map[string]*astjson.Value{},
		ctx:       ctx,
		resolvable: &Resolvable{
			// Response uses aliased field names ("identifier" for "id",
			// "fullName" for "name") — this is what the subgraph returned
			// after alias rewriting.
			data: mustParseArena(t, ar, `{"users":[{"identifier":"42","fullName":"Alice","__typename":"User"}]}`),
		},
	}

	// Entity Object describing the schema-name shape (id, name).
	providesData := &Object{
		Fields: []*Field{
			{Name: []byte("users"), Value: &Array{Item: &Object{
				HasAliases: true,
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &Scalar{}},
					{Name: []byte("identifier"), OriginalName: []byte("id"), Value: &Scalar{}},
					{Name: []byte("fullName"), OriginalName: []byte("name"), Value: &Scalar{}},
				},
			}}},
		},
	}

	entityTemplate := &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Path: []string{"users"},
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				// Template reads the aliased field name from the response.
				{Name: []byte("id"), Value: &String{Path: []string{"identifier"}}},
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
						"users:User": entityTemplate,
					},
				},
			},
			Info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData:  providesData,
			},
		},
	}

	loader.populateL1CacheForRootFieldEntities(fetchItem)

	cacheKey := `{"__typename":"User","key":{"id":"42"}}`
	cached, ok := loader.l1Cache[cacheKey]
	require.True(t, ok, "entity promoted to L1 cache")

	// Stored value must use SCHEMA field names (id, name), not response
	// names (identifier, fullName). This is the bug fix: without the
	// normalize-on-write step, the cached value would carry alias names
	// and later entity fetches using validateItemHasRequiredData against
	// schema names would silently miss.
	assert.Equal(t,
		`{"__typename":"User","id":"42","name":"Alice"}`,
		string(cached.MarshalTo(nil)))

	// Verify a subsequent entity fetch for User{id:"42"} can L1-hit.
	entityCacheKey := &CacheKey{
		Keys: []string{cacheKey},
	}
	entityInfo := &FetchInfo{
		OperationType:  ast.OperationTypeQuery,
		DataSourceName: "accounts",
		RootFields: []GraphCoordinate{
			{TypeName: "User", FieldName: "_entities"},
		},
		ProvidesData: &Object{
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &Scalar{}},
				{Name: []byte("id"), Value: &Scalar{}},
				{Name: []byte("name"), Value: &Scalar{}},
			},
		},
	}
	res := &result{}
	allComplete := loader.tryL1CacheLoad(entityInfo, []*CacheKey{entityCacheKey}, res)
	assert.True(t, allComplete, "entity L1 read should succeed with schema-shape cached value")
}

// TestL2WritePreservesFieldsOutsideSelection verifies that when a fetch
// writes back to L2 cache, fields that were cached from previous queries but
// not in the current query's selection are preserved via the mergeValues
// writeback.
func TestL2WritePreservesFieldsOutsideSelection(t *testing.T) {
	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

	// Simulate a previous L2 entry with {id, name}.
	prior := mustParseArena(t, ar, `{"__typename":"User","id":"1","name":"Alice"}`)
	// Fresh fetch writeback only contains {id, email} (current query selection).
	fresh := mustParseArena(t, ar, `{"__typename":"User","id":"1","email":"alice@example.com"}`)

	merged := mergeCachedValueForWrite(ar, prior, fresh)
	require.NotNil(t, merged)

	// The merged value must contain all three fields — name from prior,
	// email from fresh. Fresh wins on overlapping fields (id).
	assert.Equal(t,
		`{"__typename":"User","id":"1","name":"Alice","email":"alice@example.com"}`,
		string(merged.MarshalTo(nil)))
}

// TestExportRequestScopedFields_MergeWorkingCopyOnFailure verifies that when
// MergeValues fails for a request-scoped L1 merge (e.g., differing array
// lengths), the live cache entry is NOT mutated — the working-copy-and-swap
// pattern isolates the failure.
func TestExportRequestScopedFields_MergeWorkingCopyOnFailure(t *testing.T) {
	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))

	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.Caching.EnableL1Cache = true

	l := &Loader{
		jsonArena:       ar,
		requestScopedL1: map[string]*astjson.Value{},
		ctx:             ctx,
	}

	// Store an initial cached entry with an array of length 2.
	initialBytes := []byte(`{"tags":["a","b"]}`)
	initial := mustParseArena(t, ar, string(initialBytes))
	l.requestScopedL1["myKey"] = initial

	// Try to export a value with a conflicting nested shape — an array of
	// length 3 vs the existing length 2. astjson.MergeValues returns an
	// ErrMergeDifferingArrayLengths error in that case.
	sources := []*astjson.Value{
		mustParseArena(t, ar, `{"viewer":{"tags":["x","y","z"]}}`),
	}

	cfg := FetchCacheConfiguration{
		RequestScopedFields: []RequestScopedField{
			{
				FieldName: "tags",
				FieldPath: []string{"viewer", "tags"},
				L1Key:     "myKey",
				// No ProvidesData → DeepCopy without Transform → no widening check.
			},
		},
	}

	// Drop ProvidesData to use the no-Transform path. The export will
	// attempt to merge the fresh value ["x","y","z"] into a working copy
	// of the existing {"tags":["a","b"]}. Merging a bare array into an
	// object of different shape will fail safely.
	// Note: FieldPath navigates to the "tags" array, and the new value is
	// a 3-element array vs existing entry being an object with "tags":[2].
	l.exportRequestScopedFields(&result{}, cfg, sources)

	// Verify the live cache entry is unchanged.
	cached, ok := l.requestScopedL1["myKey"]
	require.True(t, ok)

	// The existing entry must be byte-identical to initialBytes (with no
	// partial mutation). Accept either the original untouched state or a
	// successful merge that preserves the original shape.
	stored := string(cached.MarshalTo(nil))
	// The key invariant: the stored value is byte-identical to the original —
	// merging an array into an object fails with a type mismatch, so the
	// working-copy-and-swap leaves the live entry untouched (never partially corrupted).
	assert.Equal(t, `{"tags":["a","b"]}`, stored)
}
