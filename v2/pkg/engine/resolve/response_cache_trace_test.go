package resolve

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
)

func TestResponseCacheTrace(t *testing.T) {
	newFetch := func(ds DataSource, identifier []byte) *SingleFetch {
		input := `{"method":"POST","url":"http://accounts","body":{"query":"{me}"}}`
		return &SingleFetch{
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
			},
		}
	}

	resolve := func(t *testing.T, fetch *SingleFetch, opts ResponseCacheOptions, tracing TraceOptions) *ResponseCacheTrace {
		t.Helper()
		ctx := NewContext(context.Background())
		ctx.SetResponseCache(opts)
		ctx.TracingOptions = tracing
		response := &GraphQLResponse{
			Info:    &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Single(fetch),
			Data:    &Object{Fields: []*Field{{Name: []byte("me"), Value: &String{Path: []string{"me"}}}}},
		}
		_, err := newTestResolver(t, baseResolverOpts()).ResolveGraphQLResponse(ctx, response, nil, &bytes.Buffer{})
		require.NoError(t, err)
		if fetch.Trace == nil {
			return nil
		}
		return fetch.Trace.ResponseCache
	}

	enabled := TraceOptions{Enable: true, EnablePredictableDebugTimings: true}
	cacheable := func(cacheControl string) *SingleFetch {
		return newFetch(privateDataSource{cacheControl: cacheControl, calls: &atomic.Int32{}}, graphqlDataSourceIdentifier)
	}

	t.Run("a miss that is written, then a hit", func(t *testing.T) {
		opts := ResponseCacheOptions{Store: newTestCache(), DefaultTTL: time.Minute}

		miss := resolve(t, cacheable("public, max-age=60"), opts, enabled)
		require.NotNil(t, miss)
		require.Equal(t, "miss", miss.Status)
		require.Equal(t, "stored", miss.StoreDecision)
		require.Equal(t, "public, max-age=60", miss.CacheControl)
		require.Len(t, miss.Entries, 1)
		written := miss.Entries[0]
		require.False(t, written.Found)
		require.False(t, written.Served)
		require.True(t, written.Written)
		require.Equal(t, written.Key, written.WrittenKey)
		require.Equal(t, time.Minute.Nanoseconds(), written.WriteTTLNano)
		require.Equal(t, []string{caching.SubgraphSurrogateKey("accounts")}, written.SurrogateKeys)
		require.Empty(t, written.DeclaredTags)
		require.JSONEq(t, `{"me":"anonymous"}`, string(written.Value))

		hit := resolve(t, cacheable("public, max-age=60"), opts, enabled)
		require.Equal(t, "hit", hit.Status)
		require.Empty(t, hit.StoreDecision)
		require.Empty(t, hit.CacheControl, "nothing was asked of the subgraph")
		require.Len(t, hit.Entries, 1)
		served := hit.Entries[0]
		require.Equal(t, written.Key, served.Key)
		require.True(t, served.Found)
		require.True(t, served.Served)
		require.False(t, served.Written)
		require.Positive(t, served.RemainingTTLNano)
		require.Equal(t, written.SurrogateKeys, served.SurrogateKeys)
		require.JSONEq(t, `{"me":"anonymous"}`, string(served.Value))
	})

	t.Run("a response that is not stored has no write", func(t *testing.T) {
		trace := resolve(t, cacheable("no-store"), ResponseCacheOptions{Store: newTestCache(), DefaultTTL: time.Minute}, enabled)
		require.Equal(t, "no_store", trace.StoreDecision)
		require.Len(t, trace.Entries, 1)
		require.False(t, trace.Entries[0].Written)
		require.Zero(t, trace.Entries[0].WriteTTLNano)
	})

	t.Run("a fetch the cache is never asked about is not cacheable", func(t *testing.T) {
		fetch := newFetch(privateDataSource{cacheControl: "public, max-age=60", calls: &atomic.Int32{}}, []byte("other"))
		trace := resolve(t, fetch, ResponseCacheOptions{Store: newTestCache(), DefaultTTL: time.Minute}, enabled)
		require.Equal(t, &ResponseCacheTrace{Status: "not_cacheable", CacheControl: "public, max-age=60"}, trace)
	})

	t.Run("a failed write is not written", func(t *testing.T) {
		opts := ResponseCacheOptions{
			Store:      &failingCache{testCache: newTestCache(), setErr: errors.New("write failed")},
			DefaultTTL: time.Minute,
		}
		trace := resolve(t, cacheable("public, max-age=60"), opts, enabled)
		require.Contains(t, trace.WriteError, "write failed")
		require.False(t, trace.Entries[0].Written)
		require.Equal(t, time.Minute.Nanoseconds(), trace.Entries[0].WriteTTLNano, "what was attempted")
	})

	t.Run("excluded output leaves the value out", func(t *testing.T) {
		tracing := enabled
		tracing.ExcludeOutput = true
		trace := resolve(t, cacheable("public, max-age=60"), ResponseCacheOptions{Store: newTestCache(), DefaultTTL: time.Minute}, tracing)
		require.True(t, trace.Entries[0].Written)
		require.Nil(t, trace.Entries[0].Value)
	})

	t.Run("the request carries its surrogate keys and the caller's details", func(t *testing.T) {
		ctx := NewContext(context.Background())
		ctx.SetResponseCache(ResponseCacheOptions{
			Store:      newTestCache(),
			DefaultTTL: time.Minute,
			TraceDetails: func(surrogateKeys []string) any {
				return map[string]int{"count": len(surrogateKeys)}
			},
		})
		ctx.TracingOptions = TraceOptions{Enable: true, EnablePredictableDebugTimings: true, IncludeTraceOutputInResponseExtensions: true}
		response := &GraphQLResponse{
			Info:    &GraphQLResponseInfo{OperationType: ast.OperationTypeQuery},
			Fetches: Single(cacheable("public, max-age=60")),
			Data:    &Object{Fields: []*Field{{Name: []byte("me"), Value: &String{Path: []string{"me"}}}}},
		}
		out := &bytes.Buffer{}
		_, err := newTestResolver(t, baseResolverOpts()).ResolveGraphQLResponse(ctx, response, nil, out)
		require.NoError(t, err)

		parsed := astjson.MustParseBytes(out.Bytes())
		requestTrace := parsed.Get("extensions", "trace", "response_cache")
		require.NotNil(t, requestTrace, out.String())
		require.JSONEq(t, `{"surrogate_keys":["subgraph-accounts"],"details":{"count":1}}`, string(requestTrace.MarshalTo(nil)))
	})

	t.Run("excluded or off, there is no trace", func(t *testing.T) {
		tracing := enabled
		tracing.ExcludeResponseCache = true
		require.Nil(t, resolve(t, cacheable("public, max-age=60"), ResponseCacheOptions{Store: newTestCache(), DefaultTTL: time.Minute}, tracing))
		require.Nil(t, resolve(t, cacheable("public, max-age=60"), ResponseCacheOptions{Store: newTestCache(), DefaultTTL: time.Minute}, TraceOptions{}))
	})
}

func TestResponseCacheTrace_MultiEntity(t *testing.T) {
	load := func(t *testing.T, cache *testCache, multiDS *recordingDataSource) *ResponseCacheTrace {
		t.Helper()
		ctx := multiEntityContext(t)
		ctx.TracingOptions = TraceOptions{Enable: true, EnablePredictableDebugTimings: true}
		ctx.SetResponseCache(ResponseCacheOptions{Store: cache, DefaultTTL: time.Minute})
		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		response := multiEntityMergedTree(&recordingDataSource{response: []byte(multiEntityRootResponse)}, multiDS)
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, response))
		multi := response.Fetches.ChildNodes[1].Item.Fetch.(*MultiEntityFetch)
		require.NotNil(t, multi.Trace)
		return multi.Trace.ResponseCache
	}

	cache := newTestCache()
	cold := load(t, cache, &recordingDataSource{response: []byte(multiEntityMergedResponse), responseHeaders: cacheableHeaders()})
	require.Equal(t, "miss", cold.Status)
	require.Len(t, cold.Entries, 3, "two employees and one employee")
	aliases := map[string]int{}
	for _, entry := range cold.Entries {
		aliases[entry.Alias+" "+entry.Path]++
		require.True(t, entry.Written)
	}
	require.Equal(t, map[string]int{"f1 employees": 2, "f2 employee": 1}, aliases, "each entry names its response path")

	warm := load(t, cache, &recordingDataSource{err: errors.New("subgraph must not be called")})
	require.Equal(t, "hit", warm.Status)
	for _, entry := range warm.Entries {
		require.True(t, entry.Found)
		require.True(t, entry.Served)
		require.NotEmpty(t, entry.Value)
	}
}
