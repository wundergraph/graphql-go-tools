package resolve

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/caching"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
)

// sentHeaders forwards the same headers to every subgraph.
type sentHeaders http.Header

func (h sentHeaders) HeadersForSubgraph(string) (http.Header, uint64) {
	return http.Header(h), h.HashAll()
}

func (h sentHeaders) HashAll() uint64 { return xxhash.Sum64String(fmt.Sprint(http.Header(h))) }

func lang(value string) sentHeaders {
	if value == "" {
		return nil
	}
	return sentHeaders{"Accept-Language": []string{value}}
}

var acceptLanguage = []string{"accept-language"}

func langDigest(value string) caching.Digest {
	return caching.VaryDigest(acceptLanguage, http.Header(lang(value)))
}

// varyLoader is privateLoader with a Vary response header and a request that
// sends Accept-Language.
func varyLoader(t *testing.T, store caching.Cache, cacheControl, vary, body, privateID string, sent sentHeaders) (*Loader, *result) {
	t.Helper()
	loader, res := privateLoader(t, store, cacheControl, body, privateID)
	if vary != "" {
		res.httpResponseContext.Response.Header.Set("Vary", vary)
	}
	loader.ctx.SubgraphHeadersBuilder = sent
	res.sentHeaders = http.Header(sent)
	return loader, res
}

func TestResponseCacheVaryCollect(t *testing.T) {
	const body = `{"data":{"_entities":[{"__typename":"User","id":1},{"__typename":"User","id":2}]}}`
	public := []string{"pub-1", "pub-2"}
	private := []string{"prv-1", "prv-2"}

	collect := func(t *testing.T, loader *Loader, res *result, keys, privateKeys []string) []caching.Item {
		t.Helper()
		prepared := &preparedFetch{res: res, responseCacheKeys: keys, responseCachePrivateKeys: privateKeys}
		require.NoError(t, loader.responseCacheCollect(prepared))
		return prepared.responseCacheItems
	}

	t.Run("a varying body lands under its variant with a record at the base", func(t *testing.T) {
		loader, res := varyLoader(t, newTestCache(), "max-age=60", "Accept-Language", body, "", lang("de"))
		items := collect(t, loader, res, public, nil)
		require.Len(t, items, 4)

		for i, base := range public {
			record, variant := items[2*i], items[2*i+1]
			require.Equal(t, base, record.Key)
			require.Equal(t, [][]string{acceptLanguage}, record.Vary)
			require.Empty(t, record.Value)
			require.Empty(t, record.SurrogateKeys)
			require.Equal(t, caching.VariantKey(base, langDigest("de")), variant.Key)
			require.Empty(t, variant.Vary)
			require.JSONEq(t, fmt.Sprintf(`{"__typename":"User","id":%d}`, i+1), string(variant.Value))
			require.Equal(t, variant.Tags, record.Tags, "invalidation drops both")
			require.Equal(t, variant.TTL, record.TTL)
		}
	})

	t.Run("a header the router does not send digests as empty", func(t *testing.T) {
		loader, res := varyLoader(t, newTestCache(), "max-age=60", "X-Region", body, "", lang("de"))
		items := collect(t, loader, res, public, nil)
		require.Len(t, items, 4)
		require.Equal(t, caching.VariantKey("pub-1", caching.VaryDigest([]string{"x-region"}, nil)), items[1].Key)
	})

	t.Run("the digest is of the headers the request went out with", func(t *testing.T) {
		loader, res := varyLoader(t, newTestCache(), "max-age=60", "Accept-Language", body, "", lang("de"))
		loader.ctx.SubgraphHeadersBuilder = lang("fr")
		items := collect(t, loader, res, public, nil)
		require.Len(t, items, 4)
		require.Equal(t, caching.VariantKey("pub-1", langDigest("de")), items[1].Key, "not what the builder says now")
	})

	t.Run("without Vary nothing changes", func(t *testing.T) {
		loader, res := varyLoader(t, newTestCache(), "max-age=60", "", body, "", lang("de"))
		items := collect(t, loader, res, public, nil)
		require.Len(t, items, 2)
		require.Equal(t, public, []string{items[0].Key, items[1].Key})
	})

	t.Run("Vary: * is not stored", func(t *testing.T) {
		loader, res := varyLoader(t, newTestCache(), "max-age=60", "Accept-Language, *", body, "", lang("de"))
		require.Empty(t, collect(t, loader, res, public, nil))
	})

	t.Run("too many names is not stored", func(t *testing.T) {
		names := make([]string, caching.MaxVaryHeaders+1)
		for i := range names {
			names[i] = fmt.Sprintf("x-%d", i)
		}
		loader, res := varyLoader(t, newTestCache(), "max-age=60", strings.Join(names, ", "), body, "", lang("de"))
		require.Empty(t, collect(t, loader, res, public, nil))

		loader, res = varyLoader(t, newTestCache(), "max-age=60", strings.Join(names[1:], ", "), body, "", lang("de"))
		require.Len(t, collect(t, loader, res, public, nil), 4, "the cap itself is fine")
	})

	t.Run("a private response varies under the user's key", func(t *testing.T) {
		loader, res := varyLoader(t, newTestCache(), "private, max-age=60", "Accept-Language", body, "u1", lang("de"))
		items := collect(t, loader, res, public, private)
		require.Len(t, items, 4)
		require.Equal(t, "prv-2", items[2].Key)
		require.Equal(t, caching.VariantKey("prv-2", langDigest("de")), items[3].Key)
	})
}

func TestResponseCacheVaryLookup(t *testing.T) {
	seed := func(t *testing.T, store caching.Cache, items ...caching.Item) {
		t.Helper()
		require.NoError(t, store.SetMany(context.Background(), items))
	}
	body := func(key, value string) caching.Item {
		return caching.Item{Key: key, Value: []byte(value), TTL: time.Minute}
	}
	record := func(key string) caching.Item {
		return caching.Item{Key: key, Vary: [][]string{acceptLanguage}, TTL: time.Minute}
	}
	variant := func(base, value, body string) caching.Item {
		return caching.Item{Key: caching.VariantKey(base, langDigest(value)), Value: []byte(body), TTL: 30 * time.Second}
	}
	public := []string{"pub-1", "pub-2"}
	private := []string{"prv-1", "prv-2"}

	lookup := func(t *testing.T, store caching.Cache, sent sentHeaders, privateID string, keys, privateKeys []string) (bool, *result) {
		t.Helper()
		loader, res := varyLoader(t, store, "", "", "", privateID, sent)
		prepared := &preparedFetch{res: res, responseCacheKeys: keys, responseCachePrivateKeys: privateKeys}
		return loader.responseCacheLookup(prepared), res
	}

	t.Run("a record sends a second lookup for the variant", func(t *testing.T) {
		store := newSpyCache()
		seed(t, store, record("pub-1"), variant("pub-1", "de", `{"id":1,"lang":"de"}`), variant("pub-1", "fr", `{"id":1,"lang":"fr"}`))

		hit, res := lookup(t, store, lang("de"), "", public[:1], nil)
		require.True(t, hit)
		require.JSONEq(t, `{"data":{"_entities":[{"id":1,"lang":"de"}]}}`, string(res.out))
		require.Equal(t, 30*time.Second, res.responseCacheTTL, "the variant's life, not the record's")
		require.Equal(t, [][]string{{"pub-1"}, {caching.VariantKey("pub-1", langDigest("de"))}}, store.lookups)

		hit, res = lookup(t, store, lang("fr"), "", public[:1], nil)
		require.True(t, hit)
		require.JSONEq(t, `{"data":{"_entities":[{"id":1,"lang":"fr"}]}}`, string(res.out))
	})

	t.Run("a value never stored is a miss after the second lookup", func(t *testing.T) {
		store := newSpyCache()
		seed(t, store, record("pub-1"), variant("pub-1", "de", `{"id":1}`))
		hit, _ := lookup(t, store, lang("en"), "", public[:1], nil)
		require.False(t, hit)
		require.Len(t, store.lookups, 2)

		hit, _ = lookup(t, store, nil, "", public[:1], nil)
		require.False(t, hit, "nothing sent is its own variant")
	})

	t.Run("a body needs no second lookup", func(t *testing.T) {
		store := newSpyCache()
		seed(t, store, body("pub-1", `{"id":1}`), body("pub-2", `{"id":2}`))
		hit, _ := lookup(t, store, lang("de"), "", public, nil)
		require.True(t, hit)
		require.Len(t, store.lookups, 1)
	})

	t.Run("a batch mixes bodies and records", func(t *testing.T) {
		store := newSpyCache()
		seed(t, store, body("pub-1", `{"id":1}`), record("pub-2"), variant("pub-2", "de", `{"id":2,"lang":"de"}`))
		hit, res := lookup(t, store, lang("de"), "", public, nil)
		require.True(t, hit)
		require.JSONEq(t, `{"data":{"_entities":[{"id":1},{"id":2,"lang":"de"}]}}`, string(res.out))
		require.Equal(t, [][]string{public, {caching.VariantKey("pub-2", langDigest("de"))}}, store.lookups)
	})

	t.Run("a record without its variant is a miss for the whole fetch", func(t *testing.T) {
		store := newTestCache()
		seed(t, store, body("pub-1", `{"id":1}`), record("pub-2"))
		hit, _ := lookup(t, store, lang("de"), "", public, nil)
		require.False(t, hit)
	})

	t.Run("a record where a body should be is a miss", func(t *testing.T) {
		store := newTestCache()
		seed(t, store, record("pub-1"), caching.Item{Key: caching.VariantKey("pub-1", langDigest("de")), Vary: [][]string{acceptLanguage}, TTL: time.Minute})
		hit, _ := lookup(t, store, lang("de"), "", public[:1], nil)
		require.False(t, hit)
	})

	t.Run("a private record wins over a public body", func(t *testing.T) {
		store := newSpyCache()
		seed(t, store, body("pub-1", `{"id":1,"who":"everyone"}`), record("prv-1"), variant("prv-1", "de", `{"id":1,"who":"u1","lang":"de"}`))

		hit, res := lookup(t, store, lang("de"), "u1", public[:1], private[:1])
		require.True(t, hit)
		require.True(t, res.responseCachePrivate)
		require.JSONEq(t, `{"data":{"_entities":[{"id":1,"who":"u1","lang":"de"}]}}`, string(res.out))
		require.Equal(t, [][]string{{"pub-1", "prv-1"}, {caching.VariantKey("prv-1", langDigest("de"))}}, store.lookups)

		hit, _ = lookup(t, store, lang("fr"), "u1", public[:1], private[:1])
		require.False(t, hit, "the user's record is followed, not the shared body")

		hit, res = lookup(t, store, lang("fr"), "", public[:1], nil)
		require.True(t, hit, "without an id the shared body is what there is")
		require.False(t, res.responseCachePrivate)
	})
}

// varyDataSource answers in the language it was asked for and says so with
// Vary, as a localising subgraph would, and counts its calls.
type varyDataSource struct {
	vary  string
	calls *atomic.Int32
}

func (d varyDataSource) Load(ctx context.Context, headers http.Header, input []byte) ([]byte, error) {
	d.calls.Add(1)
	if rc := httpclient.GetResponseContext(ctx); rc != nil {
		rc.StatusCode = http.StatusOK
		rc.Response = &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Cache-Control": []string{"max-age=60"}, "Vary": []string{d.vary}},
		}
	}
	language := headers.Get("Accept-Language")
	if language == "" {
		language = "none"
	}
	return []byte(`{"data":{"greeting":"` + language + `"}}`), nil
}

func (d varyDataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) ([]byte, error) {
	return d.Load(ctx, headers, input)
}

func TestResponseCacheVaryResolve(t *testing.T) {
	newResponse := func(ds DataSource) *GraphQLResponse {
		input := `{"method":"POST","url":"http://greetings","body":{"query":"{greeting}"}}`
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
				InputTemplate:        InputTemplate{Segments: []TemplateSegment{{Data: []byte(input), SegmentType: StaticSegmentType}}},
				DataSourceIdentifier: graphqlDataSourceIdentifier,
				Info: &FetchInfo{
					OperationType:  ast.OperationTypeQuery,
					DataSourceID:   "greetings",
					DataSourceName: "greetings",
				},
			}),
			Data: &Object{Fields: []*Field{{Name: []byte("greeting"), Value: &String{Path: []string{"greeting"}}}}},
		}
	}

	resolveIn := func(t *testing.T, resolver *Resolver, store caching.Cache, response *GraphQLResponse, language string) (string, *ResponseInfo) {
		t.Helper()
		ctx := NewContext(context.Background())
		ctx.SubgraphHeadersBuilder = lang(language)
		ctx.SetResponseCache(ResponseCacheOptions{Store: store, DefaultTTL: time.Minute})
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

	t.Run("each language is served its own variant", func(t *testing.T) {
		calls := &atomic.Int32{}
		store := newTestCache()
		resolver := newTestResolver(t, baseResolverOpts())
		response := newResponse(varyDataSource{vary: "Accept-Language", calls: calls})

		out, info := resolveIn(t, resolver, store, response, "de")
		require.Equal(t, `{"data":{"greeting":"de"}}`, out)
		require.False(t, info.ResponseCacheHit)
		require.Len(t, store.items, 2, "record and variant")

		out, info = resolveIn(t, resolver, store, response, "de")
		require.Equal(t, `{"data":{"greeting":"de"}}`, out)
		require.True(t, info.ResponseCacheHit)
		require.Equal(t, int32(1), calls.Load())

		out, info = resolveIn(t, resolver, store, response, "fr")
		require.Equal(t, `{"data":{"greeting":"fr"}}`, out, "never another language's body")
		require.False(t, info.ResponseCacheHit)
		require.Equal(t, int32(2), calls.Load())
		require.Len(t, store.items, 3, "one record, two variants")

		out, info = resolveIn(t, resolver, store, response, "fr")
		require.Equal(t, `{"data":{"greeting":"fr"}}`, out)
		require.True(t, info.ResponseCacheHit)

		out, info = resolveIn(t, resolver, store, response, "de")
		require.Equal(t, `{"data":{"greeting":"de"}}`, out, "the first variant is still there")
		require.True(t, info.ResponseCacheHit)
		require.Equal(t, int32(2), calls.Load())

		records := 0
		for key, item := range store.items {
			if len(item.Vary) > 0 {
				records++
				require.NotContains(t, key, "+", "a record sits at the base key")
			} else {
				require.Contains(t, key, "+", "a body sits under a variant key")
			}
		}
		require.Equal(t, 1, records)
	})

	t.Run("Vary: * is never cached", func(t *testing.T) {
		calls := &atomic.Int32{}
		store := newTestCache()
		resolver := newTestResolver(t, baseResolverOpts())
		response := newResponse(varyDataSource{vary: "*", calls: calls})

		resolveIn(t, resolver, store, response, "de")
		_, info := resolveIn(t, resolver, store, response, "de")
		require.False(t, info.ResponseCacheHit)
		require.Equal(t, int32(2), calls.Load())
		require.Empty(t, store.items)
	})

	t.Run("a header the subgraph varies on but never receives shares one variant", func(t *testing.T) {
		calls := &atomic.Int32{}
		store := newTestCache()
		resolver := newTestResolver(t, baseResolverOpts())
		response := newResponse(varyDataSource{vary: "X-Region", calls: calls})

		resolveIn(t, resolver, store, response, "de")
		out, info := resolveIn(t, resolver, store, response, "fr")
		require.Equal(t, `{"data":{"greeting":"de"}}`, out, "the subgraph did not claim to vary on the language")
		require.True(t, info.ResponseCacheHit)
		require.Equal(t, int32(1), calls.Load())
	})
}

// A merged fetch stores and serves variants like the single fetches it replaces:
// one Vary for the whole answer, a record per entity, a body per language.
func TestResponseCacheVaryMultiEntity(t *testing.T) {
	const (
		deResponse = `{"data":{` +
			`"f1":[{"__typename":"Employee","products":["a"]},{"__typename":"Employee","products":["b"]}],` +
			`"f2":[{"__typename":"Employee","notes":"n"}]}}`
		enResponse = `{"data":{` +
			`"f1":[{"__typename":"Employee","products":["x"]},{"__typename":"Employee","products":["y"]}],` +
			`"f2":[{"__typename":"Employee","notes":"m"}]}}`
	)

	varyHeaders := func(vary string) http.Header {
		headers := cacheableHeaders()
		headers.Set("Vary", vary)
		return headers
	}

	run := func(t *testing.T, cache *testCache, multiDS *recordingDataSource, sent sentHeaders) (*Context, string) {
		t.Helper()
		ctx := multiEntityContext(t)
		ctx.SubgraphHeadersBuilder = sent
		ctx.SetResponseCache(ResponseCacheOptions{
			Store:      cache,
			DefaultTTL: 60 * time.Second,
			OnError:    func(err error) { t.Errorf("response cache error: %v", err) },
		})
		multi := twoEntryMultiFetch(multiEntityFirstVar())
		multi.FetchID = 1
		multi.DependsOnFetchIDs = []int{0}
		multi.DataSource = multiDS
		multi.Info.DataSourceName = "products"

		loader := &Loader{dataBuffer: &DataBuffer{data: astjson.ObjectValue(nil)}}
		require.NoError(t, loader.LoadGraphQLResponseData(ctx, &GraphQLResponse{Fetches: Sequence(
			Single(multiEntityRootFetch(&recordingDataSource{response: []byte(multiEntityRootResponse)})),
			Single(multi),
		)}))
		assertMergedErrors(t, loader, "")
		return ctx, string(loader.dataBuffer.data.MarshalTo(nil))
	}

	// records and variants split what the cache holds: a record has no body.
	records := func(cache *testCache) (records, variants []string) {
		for key, item := range cache.items {
			if len(item.Vary) > 0 {
				records = append(records, key)
			} else {
				variants = append(variants, key)
			}
		}
		return records, variants
	}

	t.Run("a miss stores a record per entity and its body under the language's variant", func(t *testing.T) {
		cache := newTestCache()
		run(t, cache, &recordingDataSource{response: []byte(deResponse), responseHeaders: varyHeaders("Accept-Language")}, lang("de"))

		bases, variants := records(cache)
		require.Len(t, bases, 3)
		require.Len(t, variants, 3)
		for _, base := range bases {
			require.Equal(t, [][]string{acceptLanguage}, cache.items[base].Vary)
			variant, ok := cache.items[caching.VariantKey(base, langDigest("de"))]
			require.True(t, ok, "variant of %s", base)
			require.NotEmpty(t, variant.Value)
		}
	})

	t.Run("the same language is served whole from the cache", func(t *testing.T) {
		cache := newTestCache()
		run(t, cache, &recordingDataSource{response: []byte(deResponse), responseHeaders: varyHeaders("Accept-Language")}, lang("de"))

		warm := &recordingDataSource{err: fmt.Errorf("subgraph must not be called")}
		ctx, out := run(t, cache, warm, lang("de"))
		require.Equal(t, 0, warm.calls)
		require.Contains(t, out, `"products":["a"]`)
		require.Contains(t, out, `"notes":"n"`)
		require.ElementsMatch(t, []string{caching.SubgraphSurrogateKey("products"), caching.TypeSurrogateKey("products", "Employee")}, ctx.ResponseCacheSurrogateKeys())
	})

	t.Run("another language is fetched and kept beside the first", func(t *testing.T) {
		cache := newTestCache()
		run(t, cache, &recordingDataSource{response: []byte(deResponse), responseHeaders: varyHeaders("Accept-Language")}, lang("de"))

		en := &recordingDataSource{response: []byte(enResponse), responseHeaders: varyHeaders("Accept-Language")}
		_, out := run(t, cache, en, lang("en"))
		require.Equal(t, 1, en.calls)
		require.Contains(t, out, `"products":["x"]`)

		bases, variants := records(cache)
		require.Len(t, bases, 3, "one record per entity, rewritten not doubled")
		require.Len(t, variants, 6, "a body per language per entity")

		warm := &recordingDataSource{err: fmt.Errorf("subgraph must not be called")}
		_, out = run(t, cache, warm, lang("de"))
		require.Contains(t, out, `"products":["a"]`)
		_, out = run(t, cache, warm, lang("en"))
		require.Contains(t, out, `"products":["x"]`)
		require.Equal(t, 0, warm.calls)
	})

	t.Run("Vary: * is never stored", func(t *testing.T) {
		cache := newTestCache()
		ctx, _ := run(t, cache, &recordingDataSource{response: []byte(deResponse), responseHeaders: varyHeaders("*")}, lang("de"))
		require.Empty(t, cache.items)
		require.Nil(t, ctx.ResponseCacheSurrogateKeys())
	})

	t.Run("a record whose variant is gone sends its entry to the origin, the rest stay warm", func(t *testing.T) {
		cache := newTestCache()
		run(t, cache, &recordingDataSource{response: []byte(deResponse), responseHeaders: varyHeaders("Accept-Language")}, lang("de"))

		// Evict f2's body, leaving its record pointing at nothing.
		for key, item := range cache.items {
			if bytes.Contains(item.Value, []byte("notes")) {
				delete(cache.items, key)
			}
		}
		bases, variants := records(cache)
		require.Len(t, bases, 3)
		require.Len(t, variants, 2)

		partial := &recordingDataSource{
			response:        []byte(`{"data":{"f2":[{"__typename":"Employee","notes":"again"}]}}`),
			responseHeaders: varyHeaders("Accept-Language"),
		}
		_, out := run(t, cache, partial, lang("de"))
		require.Equal(t, 1, partial.calls)
		require.NotContains(t, string(partial.lastInput), `"id":1`, "f1 was warm and not asked for")
		require.Contains(t, string(partial.lastInput), `"id":9`)
		require.Contains(t, out, `"products":["a"]`)
		require.Contains(t, out, `"notes":"again"`)

		_, variants = records(cache)
		require.Len(t, variants, 3, "the origin's answer is stored back under its variant")
	})

	t.Run("a narrower Vary later keeps the wider set's variants reachable", func(t *testing.T) {
		cache := newTestCache()
		deEU := sentHeaders{"Accept-Language": []string{"de"}, "X-Region": []string{"eu"}}
		run(t, cache, &recordingDataSource{response: []byte(deResponse), responseHeaders: varyHeaders("Accept-Language, X-Region")}, deEU)

		// Another language, no region: a miss, and this time the origin says
		// it varies on the language alone.
		narrow := &recordingDataSource{response: []byte(enResponse), responseHeaders: varyHeaders("Accept-Language")}
		run(t, cache, narrow, lang("en"))
		require.Equal(t, 1, narrow.calls)

		bases, variants := records(cache)
		require.Len(t, bases, 3)
		require.Len(t, variants, 6)
		for _, base := range bases {
			require.Equal(t, [][]string{acceptLanguage, {"accept-language", "x-region"}}, cache.items[base].Vary)
		}

		warm := &recordingDataSource{err: fmt.Errorf("subgraph must not be called")}
		_, out := run(t, cache, warm, deEU)
		require.Contains(t, out, `"products":["a"]`, "the wide set's variant, under the narrow set there is none for de")
		_, out = run(t, cache, warm, lang("en"))
		require.Contains(t, out, `"products":["x"]`, "the narrow set's variant")
		_, out = run(t, cache, warm, sentHeaders{"Accept-Language": []string{"en"}, "X-Region": []string{"us"}})
		require.Contains(t, out, `"products":["x"]`, "the region is nothing to the narrow set")
		require.Equal(t, 0, warm.calls)
	})

	t.Run("a header the router never sends shares one variant", func(t *testing.T) {
		cache := newTestCache()
		run(t, cache, &recordingDataSource{response: []byte(deResponse), responseHeaders: varyHeaders("Accept-Language")}, nil)

		warm := &recordingDataSource{err: fmt.Errorf("subgraph must not be called")}
		_, out := run(t, cache, warm, nil)
		require.Equal(t, 0, warm.calls)
		require.Contains(t, out, `"products":["a"]`)
	})
}

// A record holds every name set responses at its key have varied on, and a
// request is served from whichever set's variant is there.
func TestResponseCacheVarySets(t *testing.T) {
	langRegion := []string{"accept-language", "x-region"}
	deEU := sentHeaders{"Accept-Language": []string{"de"}, "X-Region": []string{"eu"}}
	digest := func(names []string, sent sentHeaders) caching.Digest {
		return caching.VaryDigest(names, http.Header(sent))
	}
	variant := func(names []string, sent sentHeaders, body string) caching.Item {
		return caching.Item{Key: caching.VariantKey("pub-1", digest(names, sent)), Value: []byte(body), TTL: time.Minute}
	}
	record := caching.Item{Key: "pub-1", Vary: [][]string{acceptLanguage, langRegion}, TTL: time.Minute}

	seed := func(t *testing.T, store caching.Cache, items ...caching.Item) {
		t.Helper()
		require.NoError(t, store.SetMany(context.Background(), items))
	}
	lookup := func(t *testing.T, store caching.Cache, sent sentHeaders) (bool, *result) {
		t.Helper()
		loader, res := varyLoader(t, store, "", "", "", "", sent)
		prepared := &preparedFetch{res: res, responseCacheKeys: []string{"pub-1"}}
		return loader.responseCacheLookup(prepared), res
	}

	t.Run("one candidate per set, asked for in one round", func(t *testing.T) {
		store := newSpyCache()
		seed(t, store, record, variant(langRegion, deEU, `{"set":"lang-region"}`))

		hit, res := lookup(t, store, deEU)
		require.True(t, hit)
		require.Contains(t, string(res.out), `"set":"lang-region"`)
		require.Len(t, store.lookups, 2)
		require.Equal(t, []string{
			caching.VariantKey("pub-1", digest(acceptLanguage, deEU)),
			caching.VariantKey("pub-1", digest(langRegion, deEU)),
		}, store.lookups[1])
	})

	t.Run("when several sets hit the newest wins", func(t *testing.T) {
		store := newTestCache()
		seed(t, store, record, variant(acceptLanguage, deEU, `{"set":"lang"}`), variant(langRegion, deEU, `{"set":"lang-region"}`))

		hit, res := lookup(t, store, deEU)
		require.True(t, hit)
		require.Contains(t, string(res.out), `"set":"lang"}`)
	})

	t.Run("no set's variant is a miss", func(t *testing.T) {
		store := newTestCache()
		seed(t, store, record, variant(langRegion, sentHeaders{"Accept-Language": []string{"fr"}}, `{}`))

		hit, _ := lookup(t, store, deEU)
		require.False(t, hit)
	})

	t.Run("a header the request does not send counts as empty", func(t *testing.T) {
		store := newTestCache()
		// Stored by a request that sent no region, under the wider set.
		seed(t, store, record, variant(langRegion, lang("de"), `{"region":"none"}`))

		hit, res := lookup(t, store, lang("de"))
		require.True(t, hit, "no region again reads the same variant")
		require.Contains(t, string(res.out), `"region":"none"`)

		hit, _ = lookup(t, store, deEU)
		require.False(t, hit, "a region sent is another variant")
	})

	t.Run("a write after a miss keeps the sets already there", func(t *testing.T) {
		const body = `{"data":{"_entities":[{"__typename":"User","id":1}]}}`
		store := newTestCache()
		seed(t, store, caching.Item{Key: "pub-1", Vary: [][]string{acceptLanguage}, TTL: time.Minute})

		loader, res := varyLoader(t, store, "max-age=60", "Accept-Language, X-Region", body, "", deEU)
		prepared := &preparedFetch{res: res, responseCacheKeys: []string{"pub-1"}}
		require.False(t, loader.responseCacheLookup(prepared), "a record without its variant")
		require.NoError(t, loader.responseCacheCollect(prepared))

		items := prepared.responseCacheItems
		require.Len(t, items, 2)
		require.Equal(t, "pub-1", items[0].Key)
		require.Equal(t, [][]string{langRegion, acceptLanguage}, items[0].Vary, "own set first, the seen one kept")
		require.Equal(t, caching.VariantKey("pub-1", digest(langRegion, deEU)), items[1].Key)
	})

	t.Run("a first write holds its own set only", func(t *testing.T) {
		const body = `{"data":{"_entities":[{"__typename":"User","id":1}]}}`
		loader, res := varyLoader(t, newTestCache(), "max-age=60", "Accept-Language", body, "", lang("de"))
		prepared := &preparedFetch{res: res, responseCacheKeys: []string{"pub-1"}}
		require.False(t, loader.responseCacheLookup(prepared))
		require.NoError(t, loader.responseCacheCollect(prepared))
		require.Equal(t, [][]string{acceptLanguage}, prepared.responseCacheItems[0].Vary)
	})
}
