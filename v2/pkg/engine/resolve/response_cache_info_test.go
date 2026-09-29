package resolve

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
)

// failingCache fails the calls it is told to.
type failingCache struct {
	*testCache

	getErr, setErr error
}

func (c *failingCache) GetMany(ctx context.Context, keys []string) (map[string]caching.Item, error) {
	if c.getErr != nil {
		return nil, c.getErr
	}
	return c.testCache.GetMany(ctx, keys)
}

func (c *failingCache) SetMany(ctx context.Context, items []caching.Item) error {
	if c.setErr != nil {
		return c.setErr
	}
	return c.testCache.SetMany(ctx, items)
}

// TestResponseCacheInfo pins what the engine loader hooks are told the response
// cache did for a fetch.
func TestResponseCacheInfo(t *testing.T) {
	rootFields := []GraphCoordinate{{TypeName: "Query", FieldName: "me"}}

	newResponse := func(ds DataSource, identifier []byte) *GraphQLResponse {
		input := `{"method":"POST","url":"http://accounts","body":{"query":"{me}"}}`
		return &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: ds,
					Input:      input,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{{
					Data:        []byte(input),
					SegmentType: StaticSegmentType,
				}}},
				DataSourceIdentifier: identifier,
				Info: &FetchInfo{
					OperationType:  ast.OperationTypeQuery,
					DataSourceID:   "accounts",
					DataSourceName: "accounts",
					RootFields:     rootFields,
				},
			}),
			Data: &Object{Fields: []*Field{{Name: []byte("me"), Value: &String{Path: []string{"me"}}}}},
		}
	}

	resolveWith := func(t *testing.T, response *GraphQLResponse, opts ResponseCacheOptions) *ResponseInfo {
		t.Helper()
		ctx := NewContext(context.Background())
		ctx.SetResponseCache(opts)
		var infos []*ResponseInfo
		ctx.LoaderHooks = &spyLoaderHooks{onFinished: func(_ context.Context, _ DataSourceInfo, info *ResponseInfo) {
			infos = append(infos, info)
		}}
		_, err := newTestResolver(t, baseResolverOpts()).ResolveGraphQLResponse(ctx, response, nil, &bytes.Buffer{})
		require.NoError(t, err)
		require.Len(t, infos, 1)
		return infos[0]
	}

	cacheable := func(cacheControl string) *GraphQLResponse {
		return newResponse(privateDataSource{cacheControl: cacheControl, calls: &atomic.Int32{}}, graphqlDataSourceIdentifier)
	}

	t.Run("a miss that is stored, then a hit", func(t *testing.T) {
		opts := ResponseCacheOptions{Store: newTestCache(), DefaultTTL: time.Minute}
		response := cacheable("public, max-age=60")

		miss := resolveWith(t, response, opts)
		require.Equal(t, ResponseCacheStatusMiss, miss.ResponseCache.Status)
		require.Equal(t, caching.StoreDecisionStored, miss.ResponseCache.StoreDecision)
		require.Equal(t, 1, miss.ResponseCache.KeysRequested)
		require.Zero(t, miss.ResponseCache.KeysFound)
		require.Equal(t, rootFields, miss.RootFields)

		hit := resolveWith(t, response, opts)
		require.Equal(t, ResponseCacheStatusHit, hit.ResponseCache.Status)
		require.Equal(t, caching.StoreDecisionNone, hit.ResponseCache.StoreDecision, "nothing is decided for a hit")
		require.Equal(t, 1, hit.ResponseCache.KeysRequested)
		require.Equal(t, 1, hit.ResponseCache.KeysFound)
		require.Equal(t, rootFields, hit.RootFields)
	})

	t.Run("a response that is not stored tells why", func(t *testing.T) {
		tests := []struct {
			name         string
			cacheControl string
			defaultTTL   time.Duration
			want         caching.StoreDecision
		}{
			{name: "no-store", cacheControl: "no-store", defaultTTL: time.Minute, want: caching.StoreDecisionNoStore},
			{name: "no-cache", cacheControl: "no-cache", defaultTTL: time.Minute, want: caching.StoreDecisionNoCache},
			{name: "no lifetime", cacheControl: "public, max-age=0", defaultTTL: time.Minute, want: caching.StoreDecisionNoLifetime},
			{name: "no directive", cacheControl: "", defaultTTL: time.Minute, want: caching.StoreDecisionNoDirective},
			{name: "private without a user id", cacheControl: "private, max-age=60", defaultTTL: time.Minute, want: caching.StoreDecisionPrivateWithoutID},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				store := newTestCache()
				info := resolveWith(t, cacheable(tt.cacheControl), ResponseCacheOptions{Store: store, DefaultTTL: tt.defaultTTL})

				require.Equal(t, ResponseCacheStatusMiss, info.ResponseCache.Status)
				require.Equal(t, tt.want, info.ResponseCache.StoreDecision)
				require.Empty(t, store.items)
			})
		}
	})

	t.Run("a fetch the cache is never asked about is not cacheable", func(t *testing.T) {
		store := newTestCache()
		response := newResponse(privateDataSource{cacheControl: "public, max-age=60", calls: &atomic.Int32{}}, []byte("other"))

		info := resolveWith(t, response, ResponseCacheOptions{Store: store, DefaultTTL: time.Minute})
		require.Equal(t, ResponseCacheStatusNotCacheable, info.ResponseCache.Status)
		require.Equal(t, caching.StoreDecisionNone, info.ResponseCache.StoreDecision)
		require.Zero(t, info.ResponseCache.KeysRequested)
	})

	t.Run("a failure of the cache is reported with what failed and for which subgraph", func(t *testing.T) {
		lookupFailure, writeFailure := errors.New("lookup failed"), errors.New("write failed")
		var reported []error
		opts := ResponseCacheOptions{
			Store:      &failingCache{testCache: newTestCache(), getErr: lookupFailure, setErr: writeFailure},
			DefaultTTL: time.Minute,
			OnError:    func(err error) { reported = append(reported, err) },
		}

		info := resolveWith(t, cacheable("public, max-age=60"), opts)
		require.Equal(t, ResponseCacheStatusMiss, info.ResponseCache.Status)
		require.Equal(t, caching.StoreDecisionStored, info.ResponseCache.StoreDecision, "decided before the write was tried")

		require.Len(t, reported, 2)

		var lookup *ResponseCacheError
		require.ErrorAs(t, reported[0], &lookup)
		require.Equal(t, ResponseCacheOperationLookup, lookup.Operation)
		require.Equal(t, "accounts", lookup.Subgraph)
		require.ErrorIs(t, reported[0], lookupFailure)

		var write *ResponseCacheError
		require.ErrorAs(t, reported[1], &write)
		require.Equal(t, ResponseCacheOperationWrite, write.Operation)
		require.Equal(t, "accounts", write.Subgraph)
		require.ErrorIs(t, reported[1], writeFailure)
	})
}

// TestResponseCacheInfo_MultiEntity pins the status of a merged fetch, which
// is the only one that can be answered in part.
func TestResponseCacheInfo_MultiEntity(t *testing.T) {
	load := func(t *testing.T, cache *testCache, multiDS *recordingDataSource) ResponseCacheInfo {
		t.Helper()
		ctx := multiEntityContext(t)
		ctx.SetResponseCache(ResponseCacheOptions{
			Store:      cache,
			DefaultTTL: 60 * time.Second,
			OnError:    func(err error) { t.Errorf("response cache error: %v", err) },
		})
		var infos []*ResponseInfo
		ctx.SetEngineLoaderHooks(&spyLoaderHooks{
			onFinished: func(_ context.Context, _ DataSourceInfo, info *ResponseInfo) {
				infos = append(infos, info)
			},
		})
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		response := multiEntityMergedTree(
			&recordingDataSource{response: []byte(multiEntityRootResponse)},
			multiDS,
		)
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, response))
		require.Len(t, infos, 2)
		return infos[1].ResponseCache
	}

	cache := newTestCache()

	cold := load(t, cache, &recordingDataSource{
		response:        []byte(multiEntityMergedResponse),
		responseHeaders: cacheableHeaders(),
	})
	require.Equal(t, ResponseCacheStatusMiss, cold.Status)
	require.Equal(t, caching.StoreDecisionStored, cold.StoreDecision)
	require.Positive(t, cold.KeysRequested)
	require.Zero(t, cold.KeysFound)

	warm := load(t, cache, &recordingDataSource{err: errors.New("subgraph must not be called")})
	require.Equal(t, ResponseCacheStatusHit, warm.Status)
	require.Equal(t, caching.StoreDecisionNone, warm.StoreDecision)
	require.Equal(t, warm.KeysRequested, warm.KeysFound)

	// Evict f2 and leave f1 warm with no life left, which the TTL alone cannot
	// tell from a miss.
	cache.mu.Lock()
	for key, item := range cache.items {
		if bytes.Contains(item.Value, []byte("notes")) {
			delete(cache.items, key)
			continue
		}
		item.TTL = 0
		cache.items[key] = item
	}
	cache.mu.Unlock()

	partial := load(t, cache, &recordingDataSource{
		response:        []byte(`{"data":{"f2":[{"notes":"n"}]}}`),
		responseHeaders: http.Header{"Cache-Control": []string{"public, max-age=120"}},
	})
	require.Equal(t, ResponseCacheStatusPartialHit, partial.Status)
	require.Equal(t, caching.StoreDecisionStored, partial.StoreDecision)
}
