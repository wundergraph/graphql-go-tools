package resolve

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastjsonext"
)

// ---------------------------------------------------------------------------
// Schema building blocks for User entity tests
// ---------------------------------------------------------------------------

// newUserCacheKeyTemplate returns a cache key template for User entities with @key(fields: "id").
func newUserCacheKeyTemplate() *EntityQueryCacheKeyTemplate {
	return &EntityQueryCacheKeyTemplate{
		Keys: NewResolvableObjectVariable(&Object{
			Fields: []*Field{
				{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
				{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
			},
		}),
	}
}

// newUserProvidesData describes the fields provided by a User entity fetch.
func newUserProvidesData() *Object {
	return &Object{
		Fields: []*Field{
			{Name: []byte("id"), Value: &Scalar{Path: []string{"id"}, Nullable: false}},
			{Name: []byte("username"), Value: &Scalar{Path: []string{"username"}, Nullable: false}},
		},
	}
}

// newUserEntityFetchSegments returns the input template segments for a User _entities fetch.
func newUserEntityFetchSegments() []TemplateSegment {
	return []TemplateSegment{
		{
			Data:        []byte(`{"method":"POST","url":"http://accounts.service","body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename ... on User {id username}}}","variables":{"representations":[`),
			SegmentType: StaticSegmentType,
		},
		{
			SegmentType:  VariableSegmentType,
			VariableKind: ResolvableObjectVariableKind,
			Renderer: NewGraphQLVariableResolveRenderer(&Object{
				Fields: []*Field{
					{Name: []byte("__typename"), Value: &String{Path: []string{"__typename"}}},
					{Name: []byte("id"), Value: &String{Path: []string{"id"}}},
				},
			}),
		},
		{
			Data:        []byte(`]}}}`),
			SegmentType: StaticSegmentType,
		},
	}
}

// ---------------------------------------------------------------------------
// extInvOption — functional options for extInvEnv configuration
// ---------------------------------------------------------------------------

type extInvOption func(*extInvConfig)

type extInvConfig struct {
	enableHeaderPrefix bool
	headerHash         uint64
	l2KeyInterceptor   func(context.Context, string, L2CacheKeyInterceptorInfo) string
	disableL2          bool
}

// withExtInvHeaderPrefix enables IncludeSubgraphHeaderPrefix on the entity cache config
// and fetch configuration, and sets up a mockSubgraphHeadersBuilder with the given hash.
func withExtInvHeaderPrefix(hash uint64) extInvOption {
	return func(c *extInvConfig) {
		c.enableHeaderPrefix = true
		c.headerHash = hash
	}
}

// withExtInvInterceptor sets an L2CacheKeyInterceptor on the caching options.
func withExtInvInterceptor(fn func(context.Context, string, L2CacheKeyInterceptorInfo) string) extInvOption {
	return func(c *extInvConfig) {
		c.l2KeyInterceptor = fn
	}
}

// withExtInvL2Disabled disables L2 caching.
func withExtInvL2Disabled() extInvOption {
	return func(c *extInvConfig) {
		c.disableL2 = true
	}
}

// ---------------------------------------------------------------------------
// extInvEnv — test environment for extensions cache invalidation unit tests
// ---------------------------------------------------------------------------

// extInvEnv encapsulates all test infrastructure for a single invalidation test.
// Tests only need to specify the entity response (with/without extensions) and
// any configuration options — all boilerplate is handled here.
type extInvEnv struct {
	t        *testing.T
	loader   *Loader
	ctx      *Context
	response *GraphQLResponse
	cache    *FakeLoaderCache
}

// newExtInvEnv creates a standard test environment: one root fetch returning
// User:1, one entity fetch returning the given entityResponse.
func newExtInvEnv(t *testing.T, entityResponse string, opts ...extInvOption) *extInvEnv {
	t.Helper()

	var cfg extInvConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	cache := NewFakeLoaderCache()

	rootDS := NewMockDataSource(ctrl)
	rootDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ any, _ []byte) ([]byte, error) {
			return []byte(`{"data":{"user":{"__typename":"User","id":"1"}}}`), nil
		}).Times(1)

	entityDS := NewMockDataSource(ctrl)
	entityDS.EXPECT().
		Load(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ any, _ []byte) ([]byte, error) {
			return []byte(entityResponse), nil
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
						Enabled:                    true,
						CacheName:                  "default",
						TTL:                        30 * time.Second,
						CacheKeyTemplate:           newUserCacheKeyTemplate(),
						UseL1Cache:                 true,
						IncludeSubgraphHeaderPrefix: cfg.enableHeaderPrefix,
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

	loader := &Loader{
		caches: map[string]LoaderCache{"default": cache},
		entityCacheConfigs: map[string]map[string]*EntityCacheInvalidationConfig{
			"accounts": {
				"User": {CacheName: "default", IncludeSubgraphHeaderPrefix: cfg.enableHeaderPrefix},
			},
		},
	}

	ctx := NewContext(t.Context())
	ctx.ExecutionOptions.DisableSubgraphRequestDeduplication = true
	ctx.ExecutionOptions.Caching.EnableL1Cache = true
	ctx.ExecutionOptions.Caching.EnableL2Cache = !cfg.disableL2

	if cfg.enableHeaderPrefix {
		ctx.SubgraphHeadersBuilder = &mockSubgraphHeadersBuilder{
			hashes: map[string]uint64{"accounts": cfg.headerHash},
		}
	}
	if cfg.l2KeyInterceptor != nil {
		ctx.ExecutionOptions.Caching.L2CacheKeyInterceptor = cfg.l2KeyInterceptor
	}

	return &extInvEnv{
		t:        t,
		loader:   loader,
		ctx:      ctx,
		response: response,
		cache:    cache,
	}
}

// run executes the loader and returns the GraphQL response string.
func (e *extInvEnv) run() string {
	e.t.Helper()
	ar := arena.NewMonotonicArena(arena.WithMinBufferSize(1024))
	resolvable := NewResolvable(ar, ResolvableOptions{})
	err := resolvable.Init(e.ctx, nil, ast.OperationTypeQuery)
	require.NoError(e.t, err)

	err = e.loader.LoadGraphQLResponseData(e.ctx, e.response, resolvable)
	require.NoError(e.t, err)

	return fastjsonext.PrintGraphQLResponse(resolvable.data, resolvable.errors)
}

// deleteKeys returns all keys that were passed to cache.Delete() calls.
func (e *extInvEnv) deleteKeys() []string {
	var keys []string
	for _, entry := range e.cache.GetLog() {
		if entry.Operation == "delete" {
			keys = append(keys, entry.Keys...)
		}
	}
	return keys
}

// hasDeletes returns true if any cache.Delete() calls were recorded.
func (e *extInvEnv) hasDeletes() bool {
	for _, entry := range e.cache.GetLog() {
		if entry.Operation == "delete" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// mockSubgraphHeadersBuilder — test mock for SubgraphHeadersBuilder
// ---------------------------------------------------------------------------

type mockSubgraphHeadersBuilder struct {
	hashes map[string]uint64
}

func (m *mockSubgraphHeadersBuilder) HeadersForSubgraph(subgraphName string) (http.Header, uint64) {
	return nil, m.hashes[subgraphName]
}

func (m *mockSubgraphHeadersBuilder) HashAll() uint64 {
	return 0
}

var _ SubgraphHeadersBuilder = (*mockSubgraphHeadersBuilder)(nil)
