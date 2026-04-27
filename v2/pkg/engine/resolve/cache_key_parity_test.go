package resolve

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// TestCacheKeyParityRegression_ReadWriteInvalidation is a cross-cutting parity
// regression test: the same logical entity must produce an identical L2 cache key
// for args-derived reads, response-derived writes, and extension-driven deletes
// when GlobalCacheKeyPrefix and IncludeSubgraphHeaderPrefix are both enabled.
// This fills the gap between narrower AC-linked tests for AC-L2-04, AC-KEY-03,
// AC-KEY-07, AC-EXT-02, and AC-EXT-03.
func TestCacheKeyParityRegression_ReadWriteInvalidation(t *testing.T) {
	// schema-v42 = GlobalCacheKeyPrefix.
	// 33333 = subgraph header hash for "accounts".
	// JSON object = canonical User entity key with id derived from user(id: 42).
	const expectedKey = `schema-v42:33333:{"__typename":"User","key":{"id":"42"}}`

	// SETUP: enable L2 with both prefix layers and use one fake cache so each
	// phase can observe the exact key passed to Get, Set, or Delete.
	cache := NewFakeLoaderCache()
	ctx := NewContext(t.Context())
	// Operation variables; id=42 feeds the args-derived read key and matches
	// the response entity used for writeback.
	ctx.Variables = astjson.MustParse(`{"id":42}`)
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = false
	ctx.ExecutionOptions.Caching.EnableL2Cache = true
	ctx.ExecutionOptions.Caching.GlobalCacheKeyPrefix = "schema-v42"
	ctx.SubgraphHeadersBuilder = &mockSubgraphHeadersBuilder{
		hashes: map[string]uint64{"accounts": 33333},
	}

	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	loader := &Loader{
		ctx:       ctx,
		jsonArena: ar,
		caches:    map[string]LoaderCache{"default": cache},
	}

	rootInfo := &FetchInfo{
		DataSourceName: "accounts",
	}
	// EntityKeyMappings maps query argument id -> entity key field id, so the
	// read-side root template renders the same entity key as writeback.
	rootCfg := FetchCacheConfiguration{
		Enabled:                     true,
		CacheName:                   "default",
		TTL:                         30 * time.Second,
		UseL1Cache:                  true,
		IncludeSubgraphHeaderPrefix: true,
		CacheKeyTemplate: &RootQueryCacheKeyTemplate{
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
		},
	}
	rootRes := &result{}

	// PHASE 1 — READ KEY: prepareCacheKeys builds the L2 lookup key before any
	// fetch happens; tryL2CacheLoad records that key in the fake cache log.
	_, err := loader.prepareCacheKeys(rootInfo, rootCfg, []*astjson.Value{astjson.MustParse(`{}`)}, rootRes)
	require.NoError(t, err)

	readKeys := loader.extractCacheKeysStrings(ar, rootRes.l2CacheKeys)
	assert.Equal(t, []string{expectedKey}, readKeys)

	skipFetch, err := loader.tryL2CacheLoad(ctx.ctx, rootInfo, rootRes)
	require.NoError(t, err)
	assert.False(t, skipFetch)
	assert.Equal(t, []CacheLogEntry{
		{
			Operation: "get",
			Items:     []CacheLogItem{{Key: expectedKey, Hit: false}},
		},
	}, cache.GetLog())
	cache.ClearLog()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ any, _ []byte) ([]byte, error) {
			// Root fetch returns only the entity stub needed for entity discovery.
			return []byte(`{"data":{"user":{"__typename":"User","id":"42"}}}`), nil
		}).Times(1)

	entityDS := NewMockDataSource(ctrl)
	entityDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ any, _ []byte) ([]byte, error) {
			// Entity fetch returns the full payload that L2 writeback stores.
			return []byte(`{"data":{"_entities":[{"__typename":"User","id":"42","username":"Ada"}]}}`), nil
		}).Times(1)

	response := &GraphQLResponse{
		Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
		Fetches: Sequence(
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: rootDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data"},
					},
				},
				InputTemplate: InputTemplate{
					Segments: []TemplateSegment{
						{Data: []byte(`{"method":"POST","url":"http://root.service","body":{"query":"{user {__typename id}}"}}`), SegmentType: StaticSegmentType},
					},
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "query"),
			SingleWithPath(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: entityDS,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath: []string{"data", "_entities", "0"},
					},
					Caching: FetchCacheConfiguration{
						Enabled:                     true,
						CacheName:                   "default",
						TTL:                         30 * time.Second,
						CacheKeyTemplate:            newUserCacheKeyTemplate(),
						UseL1Cache:                  true,
						IncludeSubgraphHeaderPrefix: true,
					},
				},
				InputTemplate: InputTemplate{Segments: newUserEntityFetchSegments()},
				Info: &FetchInfo{
					DataSourceID:   "accounts",
					DataSourceName: "accounts",
					OperationType:  ast.OperationTypeQuery,
					ProvidesData:   newUserProvidesData(),
				},
				DataSourceIdentifier: []byte("graphql_datasource.Source"),
			}, "query.user", ObjectPath("user")),
		),
		Data: &Object{
			Fields: []*Field{
				{
					Name: []byte("user"),
					Value: &Object{
						Path: []string{"user"},
						Fields: []*Field{
							{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
							{Name: []byte("username"), Value: &String{Path: []string{"username"}}},
						},
					},
				},
			},
		},
	}

	resolvable := NewResolvable(ar, ResolvableOptions{})
	err = resolvable.Init(ctx, nil, ast.OperationTypeQuery)
	require.NoError(t, err)

	// PHASE 2 — WRITE KEY: run the real loader path; the cache log Set entry is
	// the key used to store the fetched entity response.
	err = loader.LoadGraphQLResponseData(ctx, response, resolvable)
	require.NoError(t, err)

	// Two entries are expected: the entity fetch L2 miss, then the entity
	// writeback Set using the response-derived key.
	assert.Equal(t, []CacheLogEntry{
		{
			Operation: "get",
			Items:     []CacheLogItem{{Key: expectedKey, Hit: false}},
		},
		{
			Operation: "set",
			Items:     []CacheLogItem{{Key: expectedKey, TTL: 30 * time.Second}},
		},
	}, cache.GetLog())

	// PHASE 3 — INVALIDATION KEY: use a separate execution because
	// processExtensionsCacheInvalidation skips deleting a key that the active
	// fetch is about to write. This independent env exposes the Delete key.
	env := newExtInvEnv(t,
		// extensions.cacheInvalidation.keys[0] is the subgraph contract for
		// telling the loader which entity key to invalidate.
		`{"data":{"_entities":[{"__typename":"User","id":"1","username":"Alice"}]},"extensions":{"cacheInvalidation":{"keys":[{"typename":"User","key":{"id":"42"}}]}}}`,
		withExtInvHeaderPrefix(33333),
	)
	env.ctx.ExecutionOptions.Caching.GlobalCacheKeyPrefix = "schema-v42"
	env.run()

	invalidationKeys := env.deleteKeys()
	assert.Equal(t, []string{expectedKey}, invalidationKeys)

	// PARITY: read == write == invalidation is the cache-key contract.
	writeKeys := []string{cache.GetLog()[1].Items[0].Key}
	assert.Equal(t, readKeys, writeKeys)
	assert.Equal(t, readKeys, invalidationKeys)
}
