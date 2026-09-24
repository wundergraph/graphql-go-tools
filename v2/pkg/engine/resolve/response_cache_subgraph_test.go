package resolve

import (
	"bytes"
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// subgraphLoader is privateLoader with per-subgraph options and a named subgraph.
func subgraphLoader(t *testing.T, store caching.Cache, subgraph, cacheControl, body string, opts ResponseCacheOptions) (*Loader, *result) {
	t.Helper()
	opts.Store = store
	ctx := NewContext(context.Background())
	ctx.SetResponseCache(opts)
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
	}, &FetchInfo{DataSourceName: subgraph})
	return &Loader{ctx: ctx}, res
}

func TestResponseCacheSubgraphKeys(t *testing.T) {
	const body = `{"data":{"_entities":[{"__typename":"User","id":1}]}}`
	sel, e1 := caching.DigestString("sel"), caching.DigestString("e1")
	entities := []caching.Digest{e1}

	t.Run("a disabled subgraph gets no keys, another still does", func(t *testing.T) {
		opts := ResponseCacheOptions{DefaultTTL: time.Minute, Subgraphs: map[string]ResponseCacheSubgraphOptions{
			"accounts": {Disabled: true},
		}}
		loader, res := subgraphLoader(t, newTestCache(), "accounts", "public", body, opts)
		require.False(t, loader.responseCacheEnabledFor("accounts"))
		require.True(t, loader.responseCacheEnabledFor("products"))
		require.True(t, loader.responseCacheEnabledFor(""), "no name means the default")

		prepared := &preparedFetch{res: res}
		loader.responseCacheSetKeys(prepared, sel, entities)
		require.Nil(t, prepared.responseCacheKeys)
		require.Nil(t, prepared.responseCachePrivateKeys)

		_, other := subgraphLoader(t, newTestCache(), "products", "public", body, opts)
		prepared = &preparedFetch{res: other}
		loader.responseCacheSetKeys(prepared, sel, entities)
		require.Equal(t, []string{caching.Key(e1, sel)}, prepared.responseCacheKeys)
	})

	t.Run("a disabled default caches only the entries", func(t *testing.T) {
		opts := ResponseCacheOptions{DefaultTTL: time.Minute, PrivateID: "u1", DefaultDisabled: true, Subgraphs: map[string]ResponseCacheSubgraphOptions{
			"accounts": {DefaultTTL: time.Hour},
			"reviews":  {Disabled: true},
		}}
		loader, _ := subgraphLoader(t, newTestCache(), "accounts", "public", body, opts)
		require.True(t, loader.responseCacheEnabledFor("accounts"))
		require.False(t, loader.responseCacheEnabledFor("reviews"))
		require.False(t, loader.responseCacheEnabledFor("products"))
		require.False(t, loader.responseCacheEnabledFor(""))
		_, ok := loader.ctx.responseCacheSingleFlightID()
		require.False(t, ok, "the default's id keys nothing")
	})

	t.Run("an entry's private id replaces the default", func(t *testing.T) {
		opts := ResponseCacheOptions{DefaultTTL: time.Minute, PrivateID: "u1", Subgraphs: map[string]ResponseCacheSubgraphOptions{
			"accounts": {PrivateID: "acct-7"},
		}}
		loader, res := subgraphLoader(t, newTestCache(), "accounts", "public", body, opts)
		prepared := &preparedFetch{res: res}
		loader.responseCacheSetKeys(prepared, sel, entities)
		require.Equal(t, []string{caching.PrivateKey(e1, sel, caching.DigestString("acct-7"))}, prepared.responseCachePrivateKeys)

		_, other := subgraphLoader(t, newTestCache(), "products", "public", body, opts)
		prepared = &preparedFetch{res: other}
		loader.responseCacheSetKeys(prepared, sel, entities)
		require.Equal(t, []string{caching.PrivateKey(e1, sel, caching.DigestString("u1"))}, prepared.responseCachePrivateKeys)
	})

	t.Run("an entry without a private id has none, whatever the default", func(t *testing.T) {
		opts := ResponseCacheOptions{DefaultTTL: time.Minute, PrivateID: "u1", Subgraphs: map[string]ResponseCacheSubgraphOptions{
			"accounts": {DefaultTTL: time.Hour},
		}}
		loader, res := subgraphLoader(t, newTestCache(), "accounts", "public", body, opts)
		prepared := &preparedFetch{res: res}
		loader.responseCacheSetKeys(prepared, sel, entities)
		require.Equal(t, []string{caching.Key(e1, sel)}, prepared.responseCacheKeys)
		require.Nil(t, prepared.responseCachePrivateKeys)
	})

	t.Run("an entry with the default's id shares its digest", func(t *testing.T) {
		opts := ResponseCacheOptions{DefaultTTL: time.Minute, PrivateID: "u1", Subgraphs: map[string]ResponseCacheSubgraphOptions{
			"accounts": {PrivateID: "u1"},
		}}
		loader, _ := subgraphLoader(t, newTestCache(), "accounts", "public", body, opts)
		sub, ok := loader.responseCacheSubgraph("accounts")
		require.True(t, ok)
		require.Equal(t, caching.DigestString("u1"), sub.privateID)
	})
}

func TestResponseCacheSubgraphTTL(t *testing.T) {
	const body = `{"data":{"_entities":[{"__typename":"User","id":1}]}}`
	keys := []string{"k-1"}

	t.Run("an entry's fallback lifetime is used", func(t *testing.T) {
		opts := ResponseCacheOptions{DefaultTTL: time.Minute, Subgraphs: map[string]ResponseCacheSubgraphOptions{
			"accounts": {DefaultTTL: time.Hour},
		}}
		loader, res := subgraphLoader(t, newTestCache(), "accounts", "public", body, opts)
		prepared := &preparedFetch{res: res, responseCacheKeys: keys}
		require.NoError(t, loader.responseCacheCollect(prepared))
		require.Len(t, prepared.responseCacheItems, 1)
		require.Equal(t, time.Hour, prepared.responseCacheItems[0].TTL)
	})

	t.Run("an entry without one takes the default", func(t *testing.T) {
		opts := ResponseCacheOptions{DefaultTTL: time.Minute, Subgraphs: map[string]ResponseCacheSubgraphOptions{
			"accounts": {PrivateID: "u1"},
		}}
		loader, res := subgraphLoader(t, newTestCache(), "accounts", "public", body, opts)
		prepared := &preparedFetch{res: res, responseCacheKeys: keys}
		require.NoError(t, loader.responseCacheCollect(prepared))
		require.Len(t, prepared.responseCacheItems, 1)
		require.Equal(t, time.Minute, prepared.responseCacheItems[0].TTL)
	})

	t.Run("a max-age still wins over the entry", func(t *testing.T) {
		opts := ResponseCacheOptions{DefaultTTL: time.Minute, Subgraphs: map[string]ResponseCacheSubgraphOptions{
			"accounts": {DefaultTTL: time.Hour},
		}}
		loader, res := subgraphLoader(t, newTestCache(), "accounts", "public, max-age=5", body, opts)
		prepared := &preparedFetch{res: res, responseCacheKeys: keys}
		require.NoError(t, loader.responseCacheCollect(prepared))
		require.Len(t, prepared.responseCacheItems, 1)
		require.Equal(t, 5*time.Second, prepared.responseCacheItems[0].TTL)
	})
}

func TestResponseCacheSubgraphSingleFlightID(t *testing.T) {
	newCtx := func(opts ResponseCacheOptions) *Context {
		opts.Store = newTestCache()
		ctx := NewContext(context.Background())
		ctx.SetResponseCache(opts)
		return ctx
	}
	id := func(t *testing.T, opts ResponseCacheOptions) caching.Digest {
		t.Helper()
		digest, ok := newCtx(opts).responseCacheSingleFlightID()
		require.True(t, ok)
		return digest
	}

	t.Run("no id anywhere means none", func(t *testing.T) {
		_, ok := newCtx(ResponseCacheOptions{Subgraphs: map[string]ResponseCacheSubgraphOptions{
			"accounts": {DefaultTTL: time.Hour},
		}}).responseCacheSingleFlightID()
		require.False(t, ok)
	})

	t.Run("only the default id is the default's digest", func(t *testing.T) {
		require.Equal(t, caching.DigestString("u1"), id(t, ResponseCacheOptions{PrivateID: "u1"}))
	})

	t.Run("a differing entry id changes it", func(t *testing.T) {
		base := ResponseCacheOptions{PrivateID: "u1"}
		withEntry := ResponseCacheOptions{PrivateID: "u1", Subgraphs: map[string]ResponseCacheSubgraphOptions{"accounts": {PrivateID: "a"}}}
		otherEntry := ResponseCacheOptions{PrivateID: "u1", Subgraphs: map[string]ResponseCacheSubgraphOptions{"accounts": {PrivateID: "b"}}}
		require.NotEqual(t, id(t, base), id(t, withEntry))
		require.NotEqual(t, id(t, withEntry), id(t, otherEntry))
		require.Equal(t, id(t, withEntry), id(t, withEntry), "stable")
	})

	t.Run("an entry id without a default still counts", func(t *testing.T) {
		require.NotEqual(t,
			id(t, ResponseCacheOptions{Subgraphs: map[string]ResponseCacheSubgraphOptions{"accounts": {PrivateID: "a"}}}),
			id(t, ResponseCacheOptions{Subgraphs: map[string]ResponseCacheSubgraphOptions{"accounts": {PrivateID: "b"}}}),
		)
	})

	t.Run("a disabled entry's id is ignored", func(t *testing.T) {
		require.Equal(t,
			id(t, ResponseCacheOptions{PrivateID: "u1"}),
			id(t, ResponseCacheOptions{PrivateID: "u1", Subgraphs: map[string]ResponseCacheSubgraphOptions{"accounts": {Disabled: true, PrivateID: "a"}}}),
		)
	})

	t.Run("the single flight keeps users with different entry ids apart", func(t *testing.T) {
		response := &GraphQLResponse{Info: &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery}}
		sf := NewRequestSingleFlight(1)
		as := func(entryID string) *Context {
			ctx := newCtx(ResponseCacheOptions{PrivateID: "u1", Subgraphs: map[string]ResponseCacheSubgraphOptions{"accounts": {PrivateID: entryID}}})
			ctx.Request.ID = 1
			return ctx
		}
		leader, err := sf.GetOrCreate(as("a"), response)
		require.NoError(t, err)
		other, err := sf.GetOrCreate(as("b"), response)
		require.NoError(t, err)
		require.NotEqual(t, leader.ID, other.ID)
		sf.FinishOk(leader, nil)
		sf.FinishOk(other, nil)
	})
}

func TestResponseCacheSubgraphResolve(t *testing.T) {
	newResponse := func(subgraph string, ds DataSource) *GraphQLResponse {
		input := `{"method":"POST","url":"http://` + subgraph + `","body":{"query":"{me}"}}`
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
				DataSourceIdentifier: graphqlDataSourceIdentifier,
				Info: &FetchInfo{
					OperationType:  ast.OperationTypeQuery,
					DataSourceID:   subgraph,
					DataSourceName: subgraph,
				},
			}),
			Data: &Object{Fields: []*Field{{Name: []byte("me"), Value: &String{Path: []string{"me"}}}}},
		}
	}

	resolveWith := func(t *testing.T, resolver *Resolver, response *GraphQLResponse, opts ResponseCacheOptions) *ResponseInfo {
		t.Helper()
		ctx := NewContext(context.Background())
		ctx.SetResponseCache(opts)
		var infos []*ResponseInfo
		ctx.LoaderHooks = &spyLoaderHooks{onFinished: func(_ context.Context, _ DataSourceInfo, info *ResponseInfo) {
			infos = append(infos, info)
		}}
		_, err := resolver.ResolveGraphQLResponse(ctx, response, nil, &bytes.Buffer{})
		require.NoError(t, err)
		require.Len(t, infos, 1)
		return infos[0]
	}

	t.Run("a disabled subgraph is fetched every time while another is cached", func(t *testing.T) {
		store := newTestCache()
		resolver := newTestResolver(t, baseResolverOpts())
		opts := ResponseCacheOptions{Store: store, DefaultTTL: time.Minute, Subgraphs: map[string]ResponseCacheSubgraphOptions{
			"accounts": {Disabled: true},
		}}

		accountCalls := &atomic.Int32{}
		accounts := newResponse("accounts", privateDataSource{cacheControl: "public, max-age=60", calls: accountCalls})
		for range 2 {
			require.False(t, resolveWith(t, resolver, accounts, opts).ResponseCacheHit)
		}
		require.Equal(t, int32(2), accountCalls.Load())
		require.Empty(t, store.items)

		productCalls := &atomic.Int32{}
		products := newResponse("products", privateDataSource{cacheControl: "public, max-age=60", calls: productCalls})
		require.False(t, resolveWith(t, resolver, products, opts).ResponseCacheHit)
		require.True(t, resolveWith(t, resolver, products, opts).ResponseCacheHit)
		require.Equal(t, int32(1), productCalls.Load())
		require.Len(t, store.items, 1)
	})

	t.Run("a subgraph's own fallback lifetime reaches the client", func(t *testing.T) {
		store := newTestCache()
		resolver := newTestResolver(t, baseResolverOpts())
		opts := ResponseCacheOptions{Store: store, DefaultTTL: time.Minute, Subgraphs: map[string]ResponseCacheSubgraphOptions{
			"accounts": {DefaultTTL: time.Hour},
		}}
		accounts := newResponse("accounts", privateDataSource{cacheControl: "public", calls: &atomic.Int32{}})
		require.False(t, resolveWith(t, resolver, accounts, opts).ResponseCacheHit)
		hit := resolveWith(t, resolver, accounts, opts)
		require.True(t, hit.ResponseCacheHit)
		require.Equal(t, time.Hour, hit.ResponseCacheTTL, "the entry's lifetime, not the default minute")
	})
}

func TestResponseCacheSubgraphMultiEntity(t *testing.T) {
	run := func(t *testing.T, cache *testCache, multiDS *recordingDataSource, subgraphs map[string]ResponseCacheSubgraphOptions) {
		t.Helper()
		ctx := multiEntityContext(t)
		ctx.SetResponseCache(ResponseCacheOptions{
			Store:      cache,
			DefaultTTL: 60 * time.Second,
			OnError:    func(err error) { t.Errorf("response cache error: %v", err) },
			Subgraphs:  subgraphs,
		})
		response := multiEntityMergedTree(&recordingDataSource{response: []byte(multiEntityRootResponse)}, multiDS)
		response.Fetches.ChildNodes[1].Item.Fetch.(*MultiEntityFetch).Info.DataSourceName = "products"
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, response))
		assertMergedErrors(t, loader, "")
		require.JSONEq(t, multiEntityExpectedData, string(loader.dataBuffer.Get().MarshalTo(nil)))
	}

	t.Run("a disabled subgraph's merged fetch is neither stored nor served", func(t *testing.T) {
		cache := newTestCache()
		multiDS := &recordingDataSource{response: []byte(multiEntityMergedResponse), responseHeaders: cacheableHeaders()}
		disabled := map[string]ResponseCacheSubgraphOptions{"products": {Disabled: true}}
		run(t, cache, multiDS, disabled)
		run(t, cache, multiDS, disabled)
		require.Equal(t, 2, multiDS.calls)
		require.Empty(t, cache.items)
	})

	t.Run("another subgraph's entry leaves it cached", func(t *testing.T) {
		cache := newTestCache()
		multiDS := &recordingDataSource{response: []byte(multiEntityMergedResponse), responseHeaders: cacheableHeaders()}
		other := map[string]ResponseCacheSubgraphOptions{"reviews": {Disabled: true}}
		run(t, cache, multiDS, other)
		run(t, cache, multiDS, other)
		require.Equal(t, 1, multiDS.calls)
		require.NotEmpty(t, cache.items)
	})
}
