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
			require.Equal(t, acceptLanguage, record.Vary)
			require.Empty(t, record.Value)
			require.Empty(t, record.HeaderTags)
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
		return caching.Item{Key: key, Vary: acceptLanguage, TTL: time.Minute}
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
		seed(t, store, record("pub-1"), caching.Item{Key: caching.VariantKey("pub-1", langDigest("de")), Vary: acceptLanguage, TTL: time.Minute})
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
