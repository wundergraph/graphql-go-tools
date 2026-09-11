package resolve

import (
	"bytes"
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// spyCache records the keys every lookup asked for.
type spyCache struct {
	*testCache
	mu      sync.Mutex
	lookups [][]string
}

func newSpyCache() *spyCache { return &spyCache{testCache: newTestCache()} }

func (s *spyCache) GetMany(ctx context.Context, keys []string) (map[string]caching.Item, error) {
	s.mu.Lock()
	s.lookups = append(s.lookups, append([]string(nil), keys...))
	s.mu.Unlock()
	return s.testCache.GetMany(ctx, keys)
}

func privateLoader(t *testing.T, store caching.Cache, cacheControl, body, privateID string) (*Loader, *result) {
	t.Helper()

	ctx := NewContext(context.Background())
	ctx.SetResponseCache(ResponseCacheOptions{
		Store:        store,
		DefaultTTL:   time.Minute,
		Invalidation: DefaultResponseCacheTagIndexOptions(),
		PrivateID:    privateID,
	})

	res := &result{
		out:        []byte(body),
		statusCode: http.StatusOK,
		httpResponseContext: &httpclient.ResponseContext{
			Response: &http.Response{Header: http.Header{"Cache-Control": []string{cacheControl}}},
		},
	}
	res.init(PostProcessingConfiguration{
		SelectResponseDataPath:   []string{"data"},
		SelectResponseErrorsPath: []string{"errors"},
	}, &FetchInfo{DataSourceName: "accounts"})

	return &Loader{ctx: ctx}, res
}

func TestResponseCachePrivateKeys(t *testing.T) {
	t.Run("no id builds public keys only", func(t *testing.T) {
		loader, res := privateLoader(t, newTestCache(), "public", `{}`, "")
		prepared := &preparedFetch{res: res}
		loader.responseCacheSetKeys(prepared, 7, []uint64{1, 2})
		require.Equal(t, []string{caching.Key(1, 7), caching.Key(2, 7)}, prepared.responseCacheKeys)
		require.Nil(t, prepared.responseCachePrivateKeys)
		_, ok := loader.ctx.responseCachePrivateIDHash()
		require.False(t, ok)
	})

	t.Run("an id builds a positional private twin", func(t *testing.T) {
		loader, res := privateLoader(t, newTestCache(), "public", `{}`, "u1")
		prepared := &preparedFetch{res: res}
		loader.responseCacheSetKeys(prepared, 7, []uint64{1, 2})
		idHash, ok := loader.ctx.responseCachePrivateIDHash()
		require.True(t, ok)
		require.Equal(t, []string{caching.PrivateKey(1, 7, idHash), caching.PrivateKey(2, 7, idHash)}, prepared.responseCachePrivateKeys)
		require.NotEqual(t, prepared.responseCacheKeys[0], prepared.responseCachePrivateKeys[0])
	})

	t.Run("no cache means no id", func(t *testing.T) {
		ctx := NewContext(context.Background())
		hash, ok := ctx.responseCachePrivateIDHash()
		require.False(t, ok)
		require.Zero(t, hash)
	})
}

func TestResponseCachePrivateCollect(t *testing.T) {
	const body = `{"data":{"_entities":[{"__typename":"User","id":1},{"__typename":"User","id":2}]}}`
	public := []string{"pub-1", "pub-2"}
	private := []string{"prv-1", "prv-2"}

	t.Run("a private response without an id is not stored", func(t *testing.T) {
		loader, res := privateLoader(t, newTestCache(), "private, max-age=60", body, "")
		prepared := &preparedFetch{res: res, responseCacheKeys: public}
		require.NoError(t, loader.responseCacheCollect(prepared))
		require.Empty(t, prepared.responseCacheItems)
	})

	t.Run("a private response with an id is stored under the private keys", func(t *testing.T) {
		loader, res := privateLoader(t, newTestCache(), "private, max-age=60", body, "u1")
		prepared := &preparedFetch{res: res, responseCacheKeys: public, responseCachePrivateKeys: private}
		require.NoError(t, loader.responseCacheCollect(prepared))
		require.Len(t, prepared.responseCacheItems, 2)
		require.Equal(t, "prv-1", prepared.responseCacheItems[0].Key)
		require.Equal(t, "prv-2", prepared.responseCacheItems[1].Key)
		require.Equal(t, time.Minute, prepared.responseCacheItems[0].TTL)
		require.Equal(t, []string{"subgraph:accounts", "type:accounts:User"}, prepared.responseCacheItems[0].Tags, "still indexed for invalidation")
	})

	t.Run("a public response with an id is stored under the public keys", func(t *testing.T) {
		loader, res := privateLoader(t, newTestCache(), "public, max-age=60", body, "u1")
		prepared := &preparedFetch{res: res, responseCacheKeys: public, responseCachePrivateKeys: private}
		require.NoError(t, loader.responseCacheCollect(prepared))
		require.Len(t, prepared.responseCacheItems, 2)
		require.Equal(t, "pub-1", prepared.responseCacheItems[0].Key)
		require.Equal(t, "pub-2", prepared.responseCacheItems[1].Key)
	})

	t.Run("a null entity keeps the private keys aligned", func(t *testing.T) {
		nullBody := `{"data":{"_entities":[null,{"__typename":"User","id":2}]}}`
		loader, res := privateLoader(t, newTestCache(), "private, max-age=60", nullBody, "u1")
		prepared := &preparedFetch{res: res, responseCacheKeys: public, responseCachePrivateKeys: private}
		require.NoError(t, loader.responseCacheCollect(prepared))
		require.Len(t, prepared.responseCacheItems, 1)
		require.Equal(t, "prv-2", prepared.responseCacheItems[0].Key)
	})
}

func TestResponseCachePrivateLookup(t *testing.T) {
	seed := func(t *testing.T, store caching.Cache, items ...caching.Item) {
		t.Helper()
		require.NoError(t, store.SetMany(context.Background(), items))
	}
	item := func(key, value string, ttl time.Duration) caching.Item {
		return caching.Item{Key: key, Value: []byte(value), TTL: ttl}
	}
	public := []string{"pub-1", "pub-2"}
	private := []string{"prv-1", "prv-2"}

	t.Run("without private keys only the public keys are asked for", func(t *testing.T) {
		store := newSpyCache()
		seed(t, store, item("pub-1", `{"id":1}`, time.Minute), item("pub-2", `{"id":2}`, time.Minute))
		loader, res := privateLoader(t, store, "", "", "")
		prepared := &preparedFetch{res: res, responseCacheKeys: public}
		require.True(t, loader.responseCacheLookup(prepared))
		require.Equal(t, [][]string{public}, store.lookups)
		require.False(t, res.responseCachePrivate)
		require.JSONEq(t, `{"data":{"_entities":[{"id":1},{"id":2}]}}`, string(res.out))
	})

	t.Run("both key sets go out in one call", func(t *testing.T) {
		store := newSpyCache()
		loader, res := privateLoader(t, store, "", "", "u1")
		prepared := &preparedFetch{res: res, responseCacheKeys: public, responseCachePrivateKeys: private}
		require.False(t, loader.responseCacheLookup(prepared))
		require.Equal(t, [][]string{{"pub-1", "pub-2", "prv-1", "prv-2"}}, store.lookups)
	})

	t.Run("the private entry wins over the public one", func(t *testing.T) {
		store := newTestCache()
		seed(t, store,
			item("pub-1", `{"id":1,"who":"everyone"}`, time.Minute),
			item("prv-1", `{"id":1,"who":"u1"}`, 30*time.Second),
			item("pub-2", `{"id":2}`, time.Minute),
		)
		loader, res := privateLoader(t, store, "", "", "u1")
		prepared := &preparedFetch{res: res, responseCacheKeys: public, responseCachePrivateKeys: private}
		require.True(t, loader.responseCacheLookup(prepared))
		require.JSONEq(t, `{"data":{"_entities":[{"id":1,"who":"u1"},{"id":2}]}}`, string(res.out))
		require.True(t, res.responseCachePrivate, "a mixed hit is private")
		require.Equal(t, 30*time.Second, res.responseCacheTTL, "min over the chosen entries")
	})

	t.Run("a private entry is invisible without private keys", func(t *testing.T) {
		store := newTestCache()
		seed(t, store, item("prv-1", `{"id":1}`, time.Minute), item("prv-2", `{"id":2}`, time.Minute))
		loader, res := privateLoader(t, store, "", "", "")
		prepared := &preparedFetch{res: res, responseCacheKeys: public}
		require.False(t, loader.responseCacheLookup(prepared))
	})

	t.Run("a missing position is a miss for the whole fetch", func(t *testing.T) {
		store := newTestCache()
		seed(t, store, item("prv-1", `{"id":1}`, time.Minute))
		loader, res := privateLoader(t, store, "", "", "u1")
		prepared := &preparedFetch{res: res, responseCacheKeys: public, responseCachePrivateKeys: private}
		require.False(t, loader.responseCacheLookup(prepared))
	})

	t.Run("a root fetch served privately is rebuilt as data", func(t *testing.T) {
		store := newTestCache()
		seed(t, store, item("prv-1", `{"me":{"id":"u1"}}`, time.Minute))
		loader, res := privateLoader(t, store, "", "", "u1")
		prepared := &preparedFetch{res: res, responseCacheKeys: public[:1], responseCachePrivateKeys: private[:1], isRootFetchCache: true}
		require.True(t, loader.responseCacheLookup(prepared))
		require.JSONEq(t, `{"data":{"me":{"id":"u1"}}}`, string(res.out))
		require.True(t, res.responseCachePrivate)
	})
}

// userHeaders forwards one user header to every subgraph.
type userHeaders struct{ user string }

func (h userHeaders) HeadersForSubgraph(string) (http.Header, uint64) {
	if h.user == "" {
		return nil, 0
	}
	return http.Header{"X-User-Id": []string{h.user}}, xxhash.Sum64String(h.user)
}

func (h userHeaders) HashAll() uint64 { return xxhash.Sum64String(h.user) }

// privateDataSource answers with the user it was asked for, as a subgraph that
// reads the identity from a forwarded header would, and counts its calls.
type privateDataSource struct {
	cacheControl string
	calls        *atomic.Int32
}

func (d privateDataSource) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
	d.calls.Add(1)
	if rc := httpclient.GetResponseContext(ctx); rc != nil {
		rc.StatusCode = http.StatusOK
		rc.Response = &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Cache-Control": []string{d.cacheControl}},
		}
	}
	user := headers.Get("X-User-Id")
	if user == "" {
		user = "anonymous"
	}
	return []byte(`{"data":{"me":"` + user + `"}}`), nil
}

func (d privateDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(ctx, headers, input)
}

func TestResponseCachePrivateResolve(t *testing.T) {
	newResponse := func(ds DataSource) *GraphQLResponse {
		return &GraphQLResponse{
			Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Single(&SingleFetch{
				FetchConfiguration: FetchConfiguration{
					DataSource: ds,
					Input:      `{"method":"POST","url":"http://accounts","body":{"query":"{me}"}}`,
					PostProcessing: PostProcessingConfiguration{
						SelectResponseDataPath:   []string{"data"},
						SelectResponseErrorsPath: []string{"errors"},
					},
				},
				InputTemplate: InputTemplate{Segments: []TemplateSegment{{
					Data:        []byte(`{"method":"POST","url":"http://accounts","body":{"query":"{me}"}}`),
					SegmentType: StaticSegmentType,
				}}},
				DataSourceIdentifier: graphqlDataSourceIdentifier,
				Info: &FetchInfo{
					OperationType:  ast.OperationTypeQuery,
					DataSourceID:   "accounts",
					DataSourceName: "accounts",
				},
			}),
			Data: &Object{Fields: []*Field{{Name: []byte("me"), Value: &String{Path: []string{"me"}}}}},
		}
	}

	resolveAs := func(t *testing.T, resolver *Resolver, store caching.Cache, response *GraphQLResponse, user string) (string, *ResponseInfo) {
		t.Helper()
		ctx := NewContext(context.Background())
		if user != "" {
			ctx.SubgraphHeadersBuilder = userHeaders{user: user}
		}
		ctx.SetResponseCache(ResponseCacheOptions{Store: store, DefaultTTL: time.Minute, PrivateID: user})
		var infos []*ResponseInfo
		ctx.LoaderHooks = &spyLoaderHooks{onFinished: func(_ context.Context, _ DataSourceInfo, info *ResponseInfo) {
			infos = append(infos, info)
		}}
		buf := &bytes.Buffer{}
		_, err := resolver.ResolveGraphQLResponse(ctx, response, nil, buf)
		require.NoError(t, err)
		require.Len(t, infos, 1)
		return buf.String(), infos[0]
	}

	t.Run("each user is served their own entry", func(t *testing.T) {
		calls := &atomic.Int32{}
		store := newTestCache()
		resolver := newTestResolver(t, baseResolverOpts())
		response := newResponse(privateDataSource{cacheControl: "private, max-age=60", calls: calls})

		out, info := resolveAs(t, resolver, store, response, "u1")
		require.Equal(t, `{"data":{"me":"u1"}}`, out)
		require.False(t, info.ResponseCacheHit)
		require.Equal(t, int32(1), calls.Load())
		require.Len(t, store.items, 1)

		out, info = resolveAs(t, resolver, store, response, "u1")
		require.Equal(t, `{"data":{"me":"u1"}}`, out)
		require.True(t, info.ResponseCacheHit)
		require.True(t, info.ResponseCachePrivate)
		require.Equal(t, int32(1), calls.Load(), "served from the cache")

		out, info = resolveAs(t, resolver, store, response, "u2")
		require.Equal(t, `{"data":{"me":"u2"}}`, out, "never another user's body")
		require.False(t, info.ResponseCacheHit)
		require.Equal(t, int32(2), calls.Load())
		require.Len(t, store.items, 2)

		out, info = resolveAs(t, resolver, store, response, "")
		require.Equal(t, `{"data":{"me":"anonymous"}}`, out)
		require.False(t, info.ResponseCacheHit)
		require.Equal(t, int32(3), calls.Load())
		require.Len(t, store.items, 2, "a private response without an id is not stored")

		for key := range store.items {
			require.Len(t, key, len(caching.PrivateKey(0, 0, 0)), "only private keys in the store: %s", key)
		}
	})

	t.Run("a public response is shared across users", func(t *testing.T) {
		calls := &atomic.Int32{}
		store := newTestCache()
		resolver := newTestResolver(t, baseResolverOpts())
		response := newResponse(privateDataSource{cacheControl: "public, max-age=60", calls: calls})

		out, _ := resolveAs(t, resolver, store, response, "u1")
		require.Equal(t, `{"data":{"me":"u1"}}`, out)

		out, info := resolveAs(t, resolver, store, response, "u2")
		require.Equal(t, `{"data":{"me":"u1"}}`, out, "the subgraph declared it shareable")
		require.True(t, info.ResponseCacheHit)
		require.False(t, info.ResponseCachePrivate)

		out, info = resolveAs(t, resolver, store, response, "")
		require.Equal(t, `{"data":{"me":"u1"}}`, out)
		require.True(t, info.ResponseCacheHit)
		require.Equal(t, int32(1), calls.Load())
		require.Len(t, store.items, 1)
	})
}

func TestInboundSingleFlight_PrivateID(t *testing.T) {
	response := &GraphQLResponse{Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery}}
	store := newTestCache()

	newCtx := func(user string) *Context {
		ctx := NewContext(context.Background())
		ctx.Request.ID = 1
		ctx.SetResponseCache(ResponseCacheOptions{Store: store, DefaultTTL: time.Minute, PrivateID: user})
		return ctx
	}

	t.Run("different users do not share a resolution", func(t *testing.T) {
		sf := NewRequestSingleFlight(1)
		leader, err := sf.GetOrCreate(newCtx("u1"), response)
		require.NoError(t, err)
		other, err := sf.GetOrCreate(newCtx("u2"), response)
		require.NoError(t, err)
		require.NotEqual(t, leader.ID, other.ID)
		sf.FinishOk(leader, nil)
		sf.FinishOk(other, nil)
	})

	t.Run("the same user does", func(t *testing.T) {
		sf := NewRequestSingleFlight(1)
		leader, err := sf.GetOrCreate(newCtx("u1"), response)
		require.NoError(t, err)
		done := make(chan *InflightRequest, 1)
		go func() {
			follower, err := sf.GetOrCreate(newCtx("u1"), response)
			require.NoError(t, err)
			done <- follower
		}()
		require.Eventually(t, leader.HasFollowers, time.Second, time.Millisecond)
		sf.FinishOk(leader, []byte("ok"))
		require.Equal(t, leader.ID, (<-done).ID)
	})
}
